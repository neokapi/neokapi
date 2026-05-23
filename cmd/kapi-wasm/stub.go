//go:build !(js && wasm)

// This package is only meaningful when compiled for js/wasm (see main.go,
// which is gated on `js && wasm`). The stub keeps `go build ./...`,
// `go vet ./...`, and linters happy on host platforms by giving the package
// a buildable main on every GOOS/GOARCH.
package main

func main() {}
