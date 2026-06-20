package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost/registry"
)

func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractKapiBinary(t *testing.T) {
	want := []byte("#!/bin/sh\necho new kapi\n")
	bn := binaryName()

	gz := makeTarGz(t, bn, want)
	got, err := extractKapiBinary(gz, "https://x/kapi-cli_1.0_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("extractKapiBinary(tar.gz) error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("tar.gz extract = %q, want %q", got, want)
	}

	zp := makeZip(t, bn, want)
	got, err = extractKapiBinary(zp, "https://x/kapi-cli_1.0_windows_amd64.zip")
	if err != nil {
		t.Fatalf("extractKapiBinary(zip) error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("zip extract = %q, want %q", got, want)
	}
}

func TestExtractKapiBinary_Missing(t *testing.T) {
	gz := makeTarGz(t, "not-kapi", []byte("x"))
	if _, err := extractKapiBinary(gz, "https://x/a.tar.gz"); err == nil {
		t.Error("extractKapiBinary with no kapi member: want error, got nil")
	}
}

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, binaryName())
	if err := os.WriteFile(target, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}

	newBin := []byte("NEW BINARY BYTES")
	if err := replaceBinary(target, newBin); err != nil {
		t.Fatalf("replaceBinary() error: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBin) {
		t.Errorf("after replace, target = %q, want %q", got, newBin)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("replaced binary mode = %v, want executable", info.Mode().Perm())
		}
	}
	// No stray temp files left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if name := e.Name(); name != binaryName() && name != binaryName()+".old" {
			t.Errorf("unexpected leftover file %q", name)
		}
	}
}

func TestVerifySignature_TrustPinning(t *testing.T) {
	ctx := context.Background()

	// No signature → refuse.
	err := verifySignature(ctx, registry.PlatformEntry{SHA256: "ab"})
	if err == nil {
		t.Error("verifySignature with empty signature: want error")
	}

	// Untrusted issuer → refuse (before any network call).
	err = verifySignature(ctx, registry.PlatformEntry{
		SHA256:         "ab",
		Signature:      "https://x/sig.json",
		CertIdentity:   trustedCertIdentityPrefix + "release.yml@refs/tags/v1.0.0",
		CertOIDCIssuer: "https://evil.example.com",
	})
	if err == nil {
		t.Error("verifySignature with untrusted issuer: want error")
	}

	// Untrusted identity prefix → refuse (before any network call).
	err = verifySignature(ctx, registry.PlatformEntry{
		SHA256:         "ab",
		Signature:      "https://x/sig.json",
		CertIdentity:   "https://github.com/attacker/repo/.github/workflows/release.yml@refs/tags/v1.0.0",
		CertOIDCIssuer: trustedCertIssuer,
	})
	if err == nil {
		t.Error("verifySignature with untrusted identity: want error")
	}
}
