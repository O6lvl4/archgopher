#!/bin/sh
# Builds the engine to WebAssembly and copies Go's JavaScript bridge next to it.
set -eu
cd "$(dirname "$0")/../.."
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o web/public/archgopher.wasm ./cmd/wasm
bridge="$(go env GOROOT)/lib/wasm/wasm_exec.js"
[ -f "$bridge" ] || bridge="$(go env GOROOT)/misc/wasm/wasm_exec.js"
cp -f "$bridge" web/public/wasm_exec.js
