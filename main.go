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
)

func main() {
	host := os.Getenv("GRPC_HOST")
	method := os.Getenv("GRPC_METHOD")
	requestJSON := os.Getenv("REQUEST_JSON")

	if host == "" || method == "" {
		log.Fatal("Required env vars: GRPC_HOST, GRPC_METHOD\n" +
			"Proto source (one of):\n" +
			"  PROTO_FILES  — JSON object {\"file.proto\": \"content\", ...}\n" +
			"  PROTO_CONTENT — single proto file content\n" +
			"Optional: REQUEST_JSON (default: {})\n\n" +
			"GRPC_METHOD format: 'ServiceName/MethodName' or 'pkg.ServiceName/MethodName'")
	}
	if requestJSON == "" {
		requestJSON = "{}"
	}

	// Build file map from PROTO_FILES (JSON) or fall back to PROTO_CONTENT
	fileMap := map[string]string{}
	if raw := os.Getenv("PROTO_FILES"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &fileMap); err != nil {
			log.Fatalf("PROTO_FILES is not valid JSON: %v", err)
		}
		if len(fileMap) == 0 {
			log.Fatal("PROTO_FILES must contain at least one entry")
		}
	} else if content := os.Getenv("PROTO_CONTENT"); content != "" {
		fileMap["input.proto"] = content
	} else {
		log.Fatal("Either PROTO_FILES or PROTO_CONTENT must be set")
	}

	// Collect file names — parse all of them so imports resolve
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

	// Find method descriptor by "Service/Method" or "pkg.Service/Method"
	parts := strings.SplitN(method, "/", 2)
	if len(parts) != 2 {
		log.Fatalf("GRPC_METHOD must be in format 'ServiceName/MethodName'")
	}
	serviceName := parts[0]
	methodName := parts[1]

	var methodDesc *desc.MethodDescriptor
	for _, fd := range fds {
		for _, sd := range fd.GetServices() {
			if sd.GetFullyQualifiedName() == serviceName || sd.GetName() == serviceName {
				md := sd.FindMethodByName(methodName)
				if md != nil {
					methodDesc = md
					break
				}
			}
		}
		if methodDesc != nil {
			break
		}
	}

	if methodDesc == nil {
		log.Fatalf("Method '%s/%s' not found in proto", serviceName, methodName)
	}

	if methodDesc.IsClientStreaming() || methodDesc.IsServerStreaming() {
		log.Fatal("Streaming methods are not supported")
	}

	// Build request message from JSON
	reqMsg := dynamic.NewMessage(methodDesc.GetInputType())
	if err := reqMsg.UnmarshalJSON([]byte(requestJSON)); err != nil {
		log.Fatalf("Failed to unmarshal REQUEST_JSON: %v", err)
	}

	// Connect
	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", host, err)
	}
	defer conn.Close()

	// Invoke
	stub := grpcdynamic.NewStub(conn)
	resp, err := stub.InvokeRpc(context.Background(), methodDesc, reqMsg)
	if err != nil {
		log.Fatalf("RPC error: %v", err)
	}

	// Convert response to dynamic message for JSON marshaling
	dynResp := dynamic.NewMessage(methodDesc.GetOutputType())
	if err := dynResp.ConvertFrom(resp); err != nil {
		log.Fatalf("Failed to convert response: %v", err)
	}

	jsonBytes, err := dynResp.MarshalJSON()
	if err != nil {
		log.Fatalf("Failed to marshal response: %v", err)
	}

	fmt.Println(string(jsonBytes))
}
