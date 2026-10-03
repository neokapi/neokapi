package main

import (
	"path/filepath"
	"testing"
)

// TestSelfTest runs the fixtures go run ./scripts/fieldguard -self-test runs:
// a planted use is reported, a same-named field of another type is not, and
// a use in an allowed package or in core/model is accepted.
func TestSelfTest(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err := runSelfTest(root); err != nil {
		t.Fatal(err)
	}
}

func TestModuleOf(t *testing.T) {
	mods := []module{
		{Path: "root", Dir: "."},
		{Path: "bowrain", Dir: "bowrain"},
		{Path: "bowrain/core", Dir: "bowrain/core"},
	}
	for dir, want := range map[string]string{
		"core/model":         "root",
		"bowrain":            "bowrain",
		"bowrain/server":     "bowrain",
		"bowrain/core/store": "bowrain/core",
		"bowraincore":        "root",
	} {
		if got := moduleOf(mods, dir).Path; got != want {
			t.Errorf("moduleOf(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestPending(t *testing.T) {
	root := filepath.FromSlash("/repo")
	mods := []module{{Path: "root", Dir: "."}, {Path: "kapi", Dir: "kapi"}}
	all := []string{
		filepath.FromSlash("/repo/core/storage/wasm.go"),
		filepath.FromSlash("/repo/core/storage/native.go"),
		filepath.FromSlash("/repo/kapi/cmd/kapi-wasm-cli/main_js.go"),
		filepath.FromSlash("/repo/doc.go"),
	}
	built := map[string]bool{filepath.FromSlash("/repo/core/storage/native.go"): true}
	if got := pending(root, mods, mods[0], all, built); len(got) != 2 || got[0] != "." || got[1] != "./core/storage" {
		t.Errorf("pending(root module) = %v, want [. ./core/storage]", got)
	}
	if got := pending(root, mods, mods[1], all, built); len(got) != 1 || got[0] != "./cmd/kapi-wasm-cli" {
		t.Errorf("pending(kapi module) = %v, want [./cmd/kapi-wasm-cli]", got)
	}
}
