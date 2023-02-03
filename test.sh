#!/bin/bash
os="$(uname -s)"
if [[ "$os" == "Darwin" ]]; then
	args=$(go list -f '{{.Dir}}' ./... | grep -v mmal)
else
	args="./..."
fi
set -euo pipefail
go test -tags=no_skip -race $args -json -v 2>&1 | gotestfmt
