package main

import (
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestSelfTest runs the fixtures go run ./scripts/fieldguard -self-test runs:
// a planted use is reported, a same-named field of another type is not, a
// use in an allowed package or in core/model is accepted, and a test-only
// helper is told apart by whether a test calls it.
func TestSelfTest(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err := runSelfTest(root); err != nil {
		t.Fatal(err)
	}
}

// TestVerdict checks when the check fails: a test-only helper used outside a
// test fails it under -report as well, and every other use fails it only
// without -report.
func TestVerdict(t *testing.T) {
	root := filepath.FromSlash("/repo")
	mods := []module{{Path: "root", Dir: "."}}
	at := func(file string) token.Position {
		return token.Position{Filename: filepath.Join(root, filepath.FromSlash(file)), Line: 3, Column: 2}
	}
	const helper = "Block.FileTargetAsSpelled"
	for _, tc := range []struct {
		name     string
		uses     map[string]string // file to kind
		report   string            // "" when -report passes
		noReport string            // "" when the gate passes
	}{
		{name: "no use"},
		{
			name:     "a field use outside the allowed packages",
			uses:     map[string]string{"host/a.go": "Block.Source"},
			noReport: "1 uses of Block.Source",
		},
		{
			name: "a field use in an allowed package",
			uses: map[string]string{"core/plugin/protoconvert/a.go": "Block.Targets"},
		},
		{
			name:     "a test-only helper in a test",
			uses:     map[string]string{"core/tools/a_test.go": helper},
			noReport: "1 uses of a test-only core/model helper remain",
		},
		{
			name:     "a test-only helper outside a test",
			uses:     map[string]string{"core/tools/a.go": helper},
			report:   "core/tools/a.go:3:2: Block.FileTargetAsSpelled",
			noReport: "core/tools/a.go:3:2: Block.FileTargetAsSpelled",
		},
		{
			name:     "a test-only helper outside a test in an allowed package",
			uses:     map[string]string{"core/plugin/protoconvert/a.go": helper},
			report:   "core/plugin/protoconvert/a.go:3:2: Block.FileTargetAsSpelled",
			noReport: "core/plugin/protoconvert/a.go:3:2: Block.FileTargetAsSpelled",
		},
		{
			name: "a test-only helper in core/model",
			uses: map[string]string{"core/model/a.go": helper},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv := newInventory(root, mods)
			for file, kind := range tc.uses {
				inv.add(at(file), kind)
			}
			for _, c := range []struct {
				report bool
				want   string
			}{{true, tc.report}, {false, tc.noReport}} {
				got := inv.verdict(c.report)
				if c.want == "" && got != "" {
					t.Errorf("verdict(report=%v) = %q, want a pass", c.report, got)
				}
				if c.want != "" && !strings.Contains(got, c.want) {
					t.Errorf("verdict(report=%v) = %q, want a failure naming %q", c.report, got, c.want)
				}
			}
		})
	}
}

// TestHelperOutput checks that every output format counts the test-only
// helper uses apart from the field uses, and lists them with -v.
func TestHelperOutput(t *testing.T) {
	root := filepath.FromSlash("/repo")
	inv := newInventory(root, []module{{Path: "root", Dir: "."}})
	inv.add(token.Position{Filename: filepath.Join(root, "core", "tools", "a_test.go"), Line: 7, Column: 3}, "Block.FileTargetAsSpelled")
	inv.add(token.Position{Filename: filepath.Join(root, "host", "b.go"), Line: 9, Column: 4}, "Block.Source")

	if got := inv.remaining(); got != 1 {
		t.Errorf("remaining() = %d, want the one field use", got)
	}
	var text strings.Builder
	inv.writeText(&text, true)
	for _, want := range []string{
		"  non-test: 1 uses in 1 files across 1 packages",
		"  test:     0 uses in 0 files across 0 packages",
		"  core/tools/a_test.go:7:3: Block.FileTargetAsSpelled (test-only helper)",
		"  host/b.go:9:4: Block.Source\n",
	} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("text output lacks %q:\n%s", want, text.String())
		}
	}
	// The helper's row sits under the field columns: its name, then 0 non-test
	// and 1 test use, then what it does.
	var header, row string
	for line := range strings.Lines(text.String()) {
		if strings.HasPrefix(line, "test-only helpers:") {
			header = line
		}
		if strings.HasPrefix(line, "  Block.FileTargetAsSpelled ") {
			row = line
		}
	}
	if f := strings.Fields(row); len(f) < 3 || f[1] != "0" || f[2] != "1" {
		t.Errorf("helper row = %q, want 0 non-test and 1 test use", row)
	}
	if i, j := strings.Index(header, "test\n"), strings.Index(row, "1   files"); i < 0 || i+3 != j {
		t.Errorf("helper row %q is not aligned under the test column of %q", row, header)
	}
	var md strings.Builder
	inv.writeMarkdown(&md, true)
	for _, want := range []string{
		"| `Block.FileTargetAsSpelled` | 0 | 1 | files a target under a key",
		"core/tools/a_test.go:7:3: Block.FileTargetAsSpelled\n",
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
		`"Block.FileTargetAsSpelled": {`,
		`"file": "core/tools/a_test.go"`,
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
