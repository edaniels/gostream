goformat:
	go install golang.org/x/tools/cmd/goimports
	gofmt -s -w .
	`go env GOPATH`/bin/goimports -w -local=github.com/edaniels/gostream `go list -f '{{.Dir}}' ./... | grep -Ev "proto"`

format: goformat

build: buf build-web build-go

build-go:
	go list -f '{{.Dir}}' ./... | grep -v mmal | xargs go build

build-web:
	cd frontend && npm install && npx webpack

buf:
	buf lint
	buf generate
	buf generate --template ./etc/buf.web.gen.yaml buf.build/googleapis/googleapis

lint: goformat
	go install google.golang.org/protobuf/cmd/protoc-gen-go \
      google.golang.org/grpc/cmd/protoc-gen-go-grpc \
      github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway \
      github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc
	buf lint
	go install github.com/edaniels/golinters/cmd/combined
	go install github.com/golangci/golangci-lint/cmd/golangci-lint
	go install github.com/polyfloyd/go-errorlint
	go list -f '{{.Dir}}' ./... | grep -v gen | grep -v proto | grep -v mmal | xargs go vet -vettool=`go env GOPATH`/bin/combined
	go list -f '{{.Dir}}' ./... | grep -v gen | grep -v proto | grep -v mmal | xargs `go env GOPATH`/bin/go-errorlint -errorf
	go list -f '{{.Dir}}' ./... | grep -v gen | grep -v proto | grep -v mmal | xargs go run github.com/golangci/golangci-lint/cmd/golangci-lint run -v --config=./etc/.golangci.yaml

cover:
	go test -tags=no_skip -race -coverprofile=coverage.txt ./...

test:
	go test -tags=no_skip -race ./...

stream-desktop: build-web
	go run cmd/stream_desktop/main.go

stream-camera: build-web
	go run cmd/stream_desktop/main.go -camera
