PATH_WITH_GO_BIN=`pwd`/bin:${PATH}

build: build-web build-go

build-go: buf-go
	go list -f '{{.Dir}}' ./... | grep -v mmal | xargs go build

build-web: buf-web
	cd frontend && npm install && npx webpack

buf: buf-go buf-web

buf-go:
	GOBIN=`pwd`/bin go install github.com/golang/protobuf/protoc-gen-go \
		github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc \
		google.golang.org/grpc/cmd/protoc-gen-go-grpc \
		github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway \
		github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2
	buf lint
	PATH=$(PATH_WITH_GO_BIN) buf generate

buf-web:
	buf lint
	PATH=$(PATH_WITH_GO_BIN) buf generate --template ./etc/buf.web.gen.yaml
	PATH=$(PATH_WITH_GO_BIN) buf generate --template ./etc/buf.web.gen.yaml buf.build/googleapis/googleapis

lint:
	buf lint
	go install github.com/edaniels/golinters/cmd/combined
	go install github.com/golangci/golangci-lint/cmd/golangci-lint
	go list -f '{{.Dir}}' ./... | grep -v gen | grep -v proto | grep -v mmal | xargs go vet -vettool=`go env GOPATH`/bin/combined
	go list -f '{{.Dir}}' ./... | grep -v gen | grep -v proto | grep -v mmal | xargs go run github.com/golangci/golangci-lint/cmd/golangci-lint run -v --fix --config=./etc/.golangci.yaml

cover:
	go test -tags=no_skip -race -coverprofile=coverage.txt ./...

test:
	go test -tags=no_skip -race ./...

stream-desktop: build-web
	go run cmd/stream_desktop/main.go

stream-camera: build-web
	go run cmd/stream_desktop/main.go -camera
