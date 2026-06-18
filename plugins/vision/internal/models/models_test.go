package models

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDir_Precedence(t *testing.T) {
	t.Setenv("KAPI_VISION_MODELS_DIR", "/explicit")
	if Dir() != "/explicit" {
		t.Errorf("override = %q, want /explicit", Dir())
	}
	t.Setenv("KAPI_VISION_MODELS_DIR", "")
	t.Setenv("KAPI_VISION_CACHE", "/c")
	if got := Dir(); got != filepath.Join("/c", "models", "vision") {
		t.Errorf("cache = %q", got)
	}
	t.Setenv("KAPI_VISION_CACHE", "")
	t.Setenv("XDG_CACHE_HOME", "/xdg")
	if got := Dir(); got != filepath.Join("/xdg", "kapi", "models", "vision") {
		t.Errorf("xdg = %q", got)
	}
}

func TestEnsure_PresentOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_VISION_MODELS_DIR", dir)
	body := []byte("fake-model-bytes")
	sum := sha256.Sum256(body)
	a := Asset{Key: "det", File: "m.onnx", SHA256: hex.EncodeToString(sum[:])}
	if err := os.WriteFile(filepath.Join(dir, "m.onnx"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	// Present + matching hash → returned without a URL fetch.
	p, err := Ensure(a, nil)
	if err != nil || p != filepath.Join(dir, "m.onnx") {
		t.Fatalf("Ensure present = %q, err=%v", p, err)
	}
	// Wrong hash + no URL → error.
	bad := Asset{Key: "det", File: "m.onnx", SHA256: "deadbeef"}
	if _, err := Ensure(bad, nil); err == nil {
		t.Error("expected hash-mismatch error with no URL")
	}
}

func TestEnsure_Download(t *testing.T) {
	body := []byte("downloaded-weights")
	sum := sha256.Sum256(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("KAPI_VISION_MODELS_DIR", dir)
	a := Asset{Key: "rec", File: "r.onnx", URL: srv.URL, SHA256: hex.EncodeToString(sum[:])}
	p, err := Ensure(a, func(string, ...any) {})
	if err != nil {
		t.Fatalf("Ensure download: %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != string(body) {
		t.Errorf("downloaded content mismatch")
	}
}
