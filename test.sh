#!/bin/bash
os="$(uname -s)"
if [[ "$os" == "Darwin" ]]; then
	args=$(go list -f '{{.Dir}}' ./... | grep -v mmal)
else
	args="./..."
fi
go test -tags=no_skip -race $args -v
