//go:build !(js && wasm)

package main

import (
	"fmt"
	"os"
)

// main exists so the package builds (and its tests run) on every platform; the
// real entry point is main_js.go.
func main() {
	fmt.Fprintln(os.Stderr, "cmd/web runs in a browser: build it with scripts/build-web.sh (GOOS=js GOARCH=wasm)")
	os.Exit(2)
}
