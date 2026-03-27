package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type config struct {
	host        string
	method      string
	requestJSON string
	metadataRaw string
	protoFiles  map[string]string
}

func loadConfig() config {
	host := os.Getenv("GRPC_HOST")
	method := os.Getenv("GRPC_METHOD")
	if host == "" || method == "" {
		log.Fatal("Required env vars: GRPC_HOST, GRPC_METHOD\n" +
			"Proto source (one of):\n" +
			"  PROTO_FILES   — JSON object {\"file.proto\": \"content\", ...}\n" +
			"  PROTO_CONTENT — single proto file content\n" +
			"Optional:\n" +
			"  REQUEST_JSON  — request body (default: {})\n" +
			"  GRPC_METADATA — JSON object of metadata key-value pairs\n\n" +
			"GRPC_METHOD format: 'ServiceName/MethodName' or 'pkg.ServiceName/MethodName'")
	}

	requestJSON := os.Getenv("REQUEST_JSON")
	if requestJSON == "" {
		requestJSON = "{}"
	}

	return config{
		host:        host,
		method:      method,
		requestJSON: requestJSON,
		metadataRaw: os.Getenv("GRPC_METADATA"),
		protoFiles:  loadProtoFiles(),
	}
}

func loadProtoFiles() map[string]string {
	if raw := os.Getenv("PROTO_FILES"); raw != "" {
		var fileMap map[string]string
		if err := json.Unmarshal([]byte(raw), &fileMap); err != nil {
			log.Fatalf("PROTO_FILES is not valid JSON: %v", err)
		}
		if len(fileMap) == 0 {
			log.Fatal("PROTO_FILES must contain at least one entry")
		}
		return fileMap
	}
	if content := os.Getenv("PROTO_CONTENT"); content != "" {
		return map[string]string{"input.proto": content}
	}
	log.Fatal("Either PROTO_FILES or PROTO_CONTENT must be set")
	return nil
}

func findMethod(fileMap map[string]string, method string) *desc.MethodDescriptor {
	parts := strings.SplitN(method, "/", 2)
	if len(parts) != 2 {
		log.Fatalf("GRPC_METHOD must be in format 'ServiceName/MethodName'")
	}
	serviceName, methodName := parts[0], parts[1]

	fileNames := make([]string, 0, len(fileMap))
	for name := range fileMap {
		fileNames = append(fileNames, name)
	}

	parser := protoparse.Parser{
		Accessor: protoparse.FileContentsFromMap(fileMap),
	}
	fds, err := parser.ParseFiles(fileNames...)
	if err != nil {
		log.Fatalf("Failed to parse proto: %v", err)
	}

	for _, fd := range fds {
		for _, sd := range fd.GetServices() {
			if sd.GetFullyQualifiedName() == serviceName || sd.GetName() == serviceName {
				if md := sd.FindMethodByName(methodName); md != nil {
					return md
				}
			}
		}
	}
	log.Fatalf("Method '%s' not found in proto", method)
	return nil
}

func buildContext(metadataRaw string) context.Context {
	ctx := context.Background()
	if metadataRaw == "" {
		return ctx
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(metadataRaw), &meta); err != nil {
		log.Fatalf("GRPC_METADATA is not valid JSON: %v", err)
	}
	return metadata.NewOutgoingContext(ctx, metadata.New(meta))
}

func invoke(cfg config, methodDesc *desc.MethodDescriptor) string {
	if methodDesc.IsClientStreaming() || methodDesc.IsServerStreaming() {
		log.Fatal("Streaming methods are not supported")
	}

	reqMsg := dynamic.NewMessage(methodDesc.GetInputType())
	if err := reqMsg.UnmarshalJSON([]byte(cfg.requestJSON)); err != nil {
		log.Fatalf("Failed to unmarshal REQUEST_JSON: %v", err)
	}

	conn, err := grpc.NewClient(cfg.host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", cfg.host, err)
	}
	defer conn.Close()

	resp, err := grpcdynamic.NewStub(conn).InvokeRpc(buildContext(cfg.metadataRaw), methodDesc, reqMsg)
	if err != nil {
		log.Fatalf("RPC error: %v", err)
	}

	dynResp := dynamic.NewMessage(methodDesc.GetOutputType())
	if err := dynResp.ConvertFrom(resp); err != nil {
		log.Fatalf("Failed to convert response: %v", err)
	}

	jsonBytes, err := dynResp.MarshalJSON()
	if err != nil {
		log.Fatalf("Failed to marshal response: %v", err)
	}
	return string(jsonBytes)
}

func main() {
	cfg := loadConfig()
	methodDesc := findMethod(cfg.protoFiles, cfg.method)
	fmt.Println(invoke(cfg, methodDesc))
}
