BIN_OUTPUT_PATH = bin/$(shell uname -s)-$(shell uname -m)

TOOL_BIN = bin/gotools/$(shell uname -s)-$(shell uname -m)

PATH_WITH_TOOLS="`pwd`/$(TOOL_BIN):`pwd`/node_modules/.bin:${PATH}"

build: build-web build-go

build-go: buf-go
	go list -f '{{.Dir}}' ./... | grep -v mmal | xargs go build

build-web: buf-web
	cd frontend && npm install && npx webpack

tool-install:
	GOBIN=`pwd`/$(TOOL_BIN)  go install \
		`go list -f '{{ range $$import := .Imports }} {{ $$import }} {{ end }}' ./tools/tools.go`

buf: buf-go buf-web

buf-go: tool-install
	PATH=$(PATH_WITH_TOOLS) buf lint
	PATH=$(PATH_WITH_TOOLS) buf generate

buf-web: tool-install
	PATH=$(PATH_WITH_TOOLS) buf lint
	PATH=$(PATH_WITH_TOOLS) buf generate --template ./etc/buf.web.gen.yaml

lint: tool-install
	PATH=$(PATH_WITH_TOOLS) buf lint
	export pkgs=`go list -f '{{.Dir}}' ./... | grep -v gen | grep -v proto | grep -v mmal` && echo "$$pkgs" | xargs go vet -vettool=bin/combined
	export pkgs=`go list -f '{{.Dir}}' ./... | grep -v gen | grep -v proto | grep -v mmal` && echo "$$pkgs" | xargs go run github.com/golangci/golangci-lint/cmd/golangci-lint run -v --fix --config=./etc/.golangci.yaml

cover:
	go test -tags=no_skip -race -coverprofile=coverage.txt ./...

test:
	go test -tags=no_skip -race ./...

stream-desktop: buf-go build-web
	go run cmd/stream_desktop/main.go

stream-camera: buf-go build-web
	go run cmd/stream_desktop/main.go -camera

stream-microphone: buf-go build-web
	go run cmd/stream_microphone/main.go

stream-av: buf-go build-web
	go run cmd/stream_av/main.go -camera

