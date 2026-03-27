.PHONY: build run example

build:
	go build -o grpc-client .

# Single proto file example
example:
	PROTO_CONTENT='syntax = "proto3"; \
		package helloworld; \
		service Greeter { rpc SayHello (HelloRequest) returns (HelloReply); } \
		message HelloRequest { string name = 1; } \
		message HelloReply { string message = 1; }' \
	GRPC_HOST="localhost:50051" \
	GRPC_METHOD="helloworld.Greeter/SayHello" \
	REQUEST_JSON='{"name": "World"}' \
	go run main.go

# Multiple proto files example (common types + service)
example-multi:
	PROTO_FILES='{ \
		"common.proto": "syntax = \"proto3\"; package common; message Status { int32 code = 1; string message = 2; }", \
		"service.proto": "syntax = \"proto3\"; package svc; import \"common.proto\"; service MyService { rpc GetStatus (GetStatusRequest) returns (common.Status); } message GetStatusRequest { string id = 1; }" \
	}' \
	GRPC_HOST="localhost:50051" \
	GRPC_METHOD="svc.MyService/GetStatus" \
	REQUEST_JSON='{"id": "abc"}' \
	go run main.go

# Example with metadata (authorization token + custom header)
example-meta:
	PROTO_CONTENT='syntax = "proto3"; \
		package helloworld; \
		service Greeter { rpc SayHello (HelloRequest) returns (HelloReply); } \
		message HelloRequest { string name = 1; } \
		message HelloReply { string message = 1; }' \
	GRPC_HOST="localhost:50051" \
	GRPC_METHOD="helloworld.Greeter/SayHello" \
	REQUEST_JSON='{"name": "World"}' \
	GRPC_METADATA='{"authorization": "Bearer token123", "x-request-id": "abc-456"}' \
	go run main.go

# Run with your own envs:
# make run GRPC_HOST=localhost:50051 GRPC_METHOD=pkg.Service/Method REQUEST_JSON='{}' PROTO_CONTENT='...' GRPC_METADATA='{}'
run:
	GRPC_HOST=$(GRPC_HOST) \
	GRPC_METHOD=$(GRPC_METHOD) \
	REQUEST_JSON='$(REQUEST_JSON)' \
	PROTO_CONTENT='$(PROTO_CONTENT)' \
	PROTO_FILES='$(PROTO_FILES)' \
	GRPC_METADATA='$(GRPC_METADATA)' \
	go run main.go
