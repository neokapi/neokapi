//go:build parity

package parity

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDiscoverSandbox verifies the upward-walk lookup finds a
// `.parity/bin/kapi` ancestor and returns the absolute `.parity/`
// path. Builds a tiny fake sandbox in a temp dir and chdir's into a
// nested subdir so the walk has to traverse at least one parent.
func TestDiscoverSandbox(t *testing.T) {
	tmp := t.TempDir()
	parityDir := filepath.Join(tmp, ".parity")
	binDir := filepath.Join(parityDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "kapi"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake kapi: %v", err)
	}
	nested := filepath.Join(tmp, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("chdir nested: %v", err)
	}

	got, err := discoverSandbox()
	if err != nil {
		t.Fatalf("discoverSandbox: %v", err)
	}
	wantSuffix := filepath.Base(parityDir)
	if filepath.Base(got) != wantSuffix {
		t.Fatalf("got %q, want a path ending in %q", got, wantSuffix)
	}
	// macOS resolves /private/var via /var so EvalSymlinks both sides.
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(parityDir)
	if gotResolved != wantResolved {
		t.Fatalf("got %q, want %q", gotResolved, wantResolved)
	}
}

// TestDiscoverSandboxNotFound asserts the walk returns an error when
// no `.parity/bin/kapi` exists in any ancestor.
func TestDiscoverSandboxNotFound(t *testing.T) {
	tmp := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	if _, err := discoverSandbox(); err == nil {
		t.Fatalf("discoverSandbox: expected error in empty tmp")
	}
}
