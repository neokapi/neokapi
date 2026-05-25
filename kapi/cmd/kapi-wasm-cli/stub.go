//go:build !(js && wasm)

// Buildable placeholder for non-wasm platforms so `go build ./...` and
// linters work everywhere. The real entrypoint is in main.go (js && wasm).
package main

func main() {}
