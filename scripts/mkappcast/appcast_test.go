package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSignDigest_VerifiesLikeWailsUpdater is the load-bearing test: it
// reproduces EXACTLY what github.com/wailsapp/wails/v3/pkg/updater does —
// compute sha256(artifact), then ed25519.Verify(pub, that digest, sig). If this
// passes, the signatures mkappcast emits are accepted by the native updater.
func TestSignDigest_VerifiesLikeWailsUpdater(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("pretend this is Kapi.app.zip bytes")

	sigB64 := signDigest(priv, artifact)
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatalf("signature is not base64: %v", err)
	}

	// Exactly the Wails verify path (verify.go ed25519Verifier + sha256 digest).
	sum := sha256.Sum256(artifact)
	if !ed25519.Verify(pub, sum[:], sig) {
		t.Fatal("signature did not verify under ed25519(pub, sha256(file), sig) — Wails updater would reject it")
	}

	// Tamper with the artifact → must fail.
	tampered := sha256.Sum256(append(artifact, '!'))
	if ed25519.Verify(pub, tampered[:], sig) {
		t.Fatal("signature verified against a tampered digest — must not happen")
	}
}

func TestNewItem_And_Render(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	art := filepath.Join(dir, "Kapi.app.zip")
	if err := os.WriteFile(art, []byte("zip-bytes-here"), 0o644); err != nil {
		t.Fatal(err)
	}

	item, err := newItem(priv, "1.2.0", "beta", "https://x/download/v1.2.0/Kapi.app.zip", art, nowRFC1123Z(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatal(err)
	}
	if item.Length != int64(len("zip-bytes-here")) {
		t.Errorf("Length = %d, want %d", item.Length, len("zip-bytes-here"))
	}
	if item.OS != "macos" {
		t.Errorf("OS = %q, want macos", item.OS)
	}

	xmlDoc := renderAppcast("Kapi", []Item{item})
	for _, want := range []string{
		`xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle"`,
		`<sparkle:shortVersionString>1.2.0</sparkle:shortVersionString>`,
		`<sparkle:channel>beta</sparkle:channel>`,
		`sparkle:os="macos"`,
		`sparkle:edSignature="` + item.EdSignature + `"`,
		`url="https://x/download/v1.2.0/Kapi.app.zip"`,
	} {
		if !strings.Contains(xmlDoc, want) {
			t.Errorf("appcast XML missing %q\n---\n%s", want, xmlDoc)
		}
	}
}

func TestRender_StableHasNoChannelTag(t *testing.T) {
	item := Item{ShortVersion: "1.2.0", URL: "https://x/a.zip", OS: "macos", EdSignature: "AAAA"}
	xmlDoc := renderAppcast("Kapi", []Item{item})
	if strings.Contains(xmlDoc, "<sparkle:channel>") {
		t.Errorf("stable item must not emit a <sparkle:channel> tag:\n%s", xmlDoc)
	}
}

func TestDownloadURL(t *testing.T) {
	cases := []struct{ prefix, path, want string }{
		{"https://x/download/v1", "/tmp/Kapi.app.zip", "https://x/download/v1/Kapi.app.zip"},
		{"https://x/download/v1/", "Kapi.app.zip", "https://x/download/v1/Kapi.app.zip"},
		{"", "/tmp/Kapi.app.zip", "Kapi.app.zip"},
	}
	for _, c := range cases {
		if got := downloadURL(c.prefix, c.path); got != c.want {
			t.Errorf("downloadURL(%q,%q) = %q, want %q", c.prefix, c.path, got, c.want)
		}
	}
}
