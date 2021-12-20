#!/bin/bash

rm -rf src/gen

# Ours
mkdir -p src/gen/proto
cp -R ../dist/js/proto/stream src/gen/proto

# Third-Party
mkdir -p src/gen/google
cp -R ..//dist/js/google/api src/gen/google
