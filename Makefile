PATH_WITH_TOOLS=`pwd`/bin:${PATH}

build: build-web build-go

build-go: buf-go
	go list -f '{{.Dir}}' ./... | grep -v mmal | xargs go build

build-web: buf-web
	cd frontend && npm install && npx webpack

tool-install:
	GOBIN=`pwd`/bin go install google.golang.org/protobuf/cmd/protoc-gen-go \
		github.com/bufbuild/buf/cmd/buf \
		github.com/bufbuild/buf/cmd/protoc-gen-buf-breaking \
		github.com/bufbuild/buf/cmd/protoc-gen-buf-lint \
		github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc \
		google.golang.org/grpc/cmd/protoc-gen-go-grpc \
		github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway \
		github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2 \
		github.com/edaniels/golinters/cmd/combined \
		github.com/golangci/golangci-lint/cmd/golangci-lint

buf: buf-go buf-web

buf-go: tool-install
	PATH=$(PATH_WITH_TOOLS) buf lint
	PATH=$(PATH_WITH_TOOLS) buf generate

buf-web: tool-install
	PATH=$(PATH_WITH_TOOLS) buf lint
	PATH=$(PATH_WITH_TOOLS) buf generate --template ./etc/buf.web.gen.yaml
	PATH=$(PATH_WITH_TOOLS) buf generate --template ./etc/buf.web.gen.yaml buf.build/googleapis/googleapis

lint: tool-install
	PATH=$(PATH_WITH_TOOLS) buf lint
	go list -f '{{.Dir}}' ./... | grep -v gen | grep -v proto | grep -v mmal | xargs go vet -vettool=bin/combined
	go list -f '{{.Dir}}' ./... | grep -v gen | grep -v proto | grep -v mmal | xargs go run github.com/golangci/golangci-lint/cmd/golangci-lint run -v --fix --config=./etc/.golangci.yaml

cover:
	go test -tags=no_skip -race -coverprofile=coverage.txt ./...

test:
	go test -tags=no_skip -race ./...

stream-desktop: buf-go build-web
	go run cmd/stream_desktop/main.go

stream-camera: buf-go build-web
	go run cmd/stream_desktop/main.go -camera
