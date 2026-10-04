package main

import (
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestSelfTest runs the fixtures go run ./scripts/fieldguard -self-test runs:
// a planted use is reported, a same-named field of another type is not, a use
// in a test is counted apart, and a use in an allowed package or in core/model
// is accepted.
func TestSelfTest(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err := runSelfTest(root); err != nil {
		t.Fatal(err)
	}
}

// TestVerdict checks when the check fails: a use outside a test, core/model
// and the allowed packages fails it without -report, and nothing fails it
// with -report.
func TestVerdict(t *testing.T) {
	root := filepath.FromSlash("/repo")
	mods := []module{{Path: "root", Dir: "."}}
	at := func(file string) token.Position {
		return token.Position{Filename: filepath.Join(root, filepath.FromSlash(file)), Line: 3, Column: 2}
	}
	for _, tc := range []struct {
		name     string
		uses     map[string]string // file to kind
		noReport string            // "" when the gate passes
	}{
		{name: "no use"},
		{
			name:     "a use outside a test",
			uses:     map[string]string{"host/a.go": "Block.Editions"},
			noReport: "host/a.go:3:2: Block.Editions",
		},
		{
			name: "a use in a test",
			uses: map[string]string{"core/tools/a_test.go": "Block.Editions"},
		},
		{
			name: "a use in an allowed package",
			uses: map[string]string{"core/plugin/protoconvert/a.go": "Block.Native"},
		},
		{
			name: "a use in core/model",
			uses: map[string]string{"core/model/a.go": "Block.Editions"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := newInventory(root, mods)
			for file, kind := range tc.uses {
				inv.add(at(file), kind)
			}
			if got := inv.verdict(true); got != "" {
				t.Errorf("verdict(report) = %q, want a pass", got)
			}
			got := inv.verdict(false)
			if tc.noReport == "" && got != "" {
				t.Errorf("verdict = %q, want a pass", got)
			}
			if tc.noReport != "" && !strings.Contains(got, tc.noReport) {
				t.Errorf("verdict = %q, want a failure naming %q", got, tc.noReport)
			}
		})
	}
}

// TestOutput checks that every output format counts the uses in tests apart
// from the others, and lists them with -v.
func TestOutput(t *testing.T) {
	root := filepath.FromSlash("/repo")
	inv := newInventory(root, []module{{Path: "root", Dir: "."}})
	inv.add(token.Position{Filename: filepath.Join(root, "core", "tools", "a_test.go"), Line: 7, Column: 3}, "Block.Editions")
	inv.add(token.Position{Filename: filepath.Join(root, "host", "b.go"), Line: 9, Column: 4}, "Block.Native")

	if got := len(inv.misused()); got != 1 {
		t.Errorf("misused() = %d, want the one use outside a test", got)
	}
	var text strings.Builder
	inv.writeText(&text, true)
	for _, want := range []string{
		"  non-test: 1 uses in 1 files across 1 packages",
		"  test:     1 uses in 1 files across 1 packages",
		"  core/tools/a_test.go:7:3: Block.Editions\n",
		"  host/b.go:9:4: Block.Native\n",
	} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("text output lacks %q:\n%s", want, text.String())
		}
	}
	var md strings.Builder
	inv.writeMarkdown(&md, true)
	for _, want := range []string{
		"| `Block.Editions` | 0 | 1 |",
		"| `Block.Native` | 1 | 0 |",
		"core/tools/a_test.go:7:3: Block.Editions\n",
	} {
		if !strings.Contains(md.String(), want) {
			t.Errorf("markdown output lacks %q:\n%s", want, md.String())
		}
	}
	var js strings.Builder
	if err := inv.writeJSON(&js); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"file": "core/tools/a_test.go"`,
		`"Block.Native": 1`,
	} {
		if !strings.Contains(js.String(), want) {
			t.Errorf("json output lacks %q:\n%s", want, js.String())
		}
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
