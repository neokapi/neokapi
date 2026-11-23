// Package buildconfigs lists the build configurations the repository's Go
// analyzers compile under, so that every build-tagged file is analyzed.
//
// One compilation sees the files of one GOOS, one CGO_ENABLED and one tag set.
// A file behind `//go:build e2e`, `onnx`, `js && wasm` or `windows` is seen
// only by a compilation that selects it. scripts/tagmatrix (golangci-lint and
// go vet), scripts/fieldguard, scripts/editguard, scripts/projectionguard and
// scripts/keyguard each run under every configuration in All. The first covers
// each module whole; each later one covers only the packages that hold a file
// no earlier configuration compiled.
package buildconfigs

import (
	"fmt"
	"strings"
)

// Config is one build configuration.
type Config struct {
	// Name is the configuration's name on the command line and in CI.
	Name string
	// Desc says which files the configuration exists for.
	Desc string
	// Env holds the GOOS, GOARCH and CGO_ENABLED settings, if any.
	Env []string
	// Tags is the comma-separated build tag list.
	Tags string
}

// TestTags select the integration, acceptance and end-to-end suites. They
// combine with each other and with fts5.
const TestTags = "fts5,integration,acceptance,e2e"

// ModelTags are the tags whose negation gates other files (`!onnx`,
// `!storage_oneconn`, `!notelemetry`), so they cannot join TestTags in the
// first configuration.
const ModelTags = "onnx,satmodel,pdfium_experimental,storage_oneconn,notelemetry"

// All are the configurations, in order. On a Linux runner the darwin
// configuration compiles the _darwin.go files; on a Mac the linux one
// compiles the _linux.go files. The configurations with CGO_ENABLED=0
// compile the `!cgo` files.
var All = []Config{
	{Name: "host", Desc: "the host platform with the test tags", Tags: TestTags},
	{Name: "model", Desc: "the model tags", Tags: TestTags + "," + ModelTags},
	{Name: "js-wasm", Desc: "GOOS=js GOARCH=wasm", Env: []string{"GOOS=js", "GOARCH=wasm"}, Tags: TestTags},
	{Name: "windows", Desc: "GOOS=windows without cgo", Env: []string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0"}, Tags: TestTags},
	{Name: "linux-nocgo", Desc: "GOOS=linux without cgo", Env: []string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"}, Tags: TestTags},
	{Name: "darwin-nocgo", Desc: "GOOS=darwin without cgo", Env: []string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0"}, Tags: TestTags},
}

// Named returns the configurations a comma-separated list names, in the
// order of All. An empty list names them all.
func Named(list string) ([]Config, error) {
	if list == "" {
		return All, nil
	}
	want := map[string]bool{}
	for n := range strings.SplitSeq(list, ",") {
		want[strings.TrimSpace(n)] = true
	}
	var out []Config
	for _, c := range All {
		if want[c.Name] {
			out = append(out, c)
			delete(want, c.Name)
		}
	}
	for n := range want {
		return nil, fmt.Errorf("unknown build configuration %q (have %s)", n, strings.Join(names(), ", "))
	}
	return out, nil
}

// Index returns the position of the named configuration in All, or -1.
func Index(name string) int {
	for i, c := range All {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func names() []string {
	out := make([]string, len(All))
	for i, c := range All {
		out[i] = c.Name
	}
	return out
}
