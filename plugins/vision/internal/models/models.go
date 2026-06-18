// Package models resolves and caches the RapidOCR / PP-OCRv5 ONNX model assets
// the OCR engine loads: text detection (DBNet) and
// recognition (CRNN+CTC), plus the recognition character dictionary. It mirrors
// the SaT plugin's model cache: assets download on first use and live under an
// XDG cache, with a content-hash check; a local override dir short-circuits the
// download for development and offline/bundled use.
//
// Asset SHA-256s are pinned to the PP-OCRv5 mobile models. The dictionary
// is the PP-OCRv5 character dictionary.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Asset is one downloadable model file.
type Asset struct {
	Key    string // logical key: "det", "cls", "rec", "dict"
	File   string // filename within the cache dir
	URL    string // download URL
	SHA256 string // expected content hash (hex)
}

// Registry is the default PP-OCRv5 mobile asset set (detection, recognition, and
// the v5 character dictionary). PP-OCRv5 is the current recommended PP-OCR
// generation (better accuracy than v4 at the same mobile footprint). The assets
// are mirrored on a neokapi release for reproducible, CI-reachable downloads;
// SHAs are pinned. Angle classification is not used yet (no cls model bundled).
var Registry = []Asset{
	{
		Key:    "det",
		File:   "ppocrv5_det.onnx",
		URL:    "https://github.com/neokapi/neokapi/releases/download/vision-models-v1/ppocrv5_det.onnx",
		SHA256: "1eb7b4f7ab657ebd1c66d5f79bca7497f29768a2e3c15e52daecbba1a8e4a039",
	},
	{
		Key:    "rec",
		File:   "ppocrv5_rec.onnx",
		URL:    "https://github.com/neokapi/neokapi/releases/download/vision-models-v1/ppocrv5_rec.onnx",
		SHA256: "243a0f06d826761323e9045e9b113ab2c191c3aa50565585e628300b8eda0224",
	},
	{
		Key:    "dict",
		File:   "ppocrv5_dict.txt",
		URL:    "https://github.com/neokapi/neokapi/releases/download/vision-models-v1/ppocrv5_dict.txt",
		SHA256: "d1979e9f794c464c0d2e0b70a7fe14dd978e9dc644c0e71f14158cdf8342af1b",
	},
	{
		// PP-DocLayoutV3 layout detector (RT-DETR, 25 region classes). ~132 MB —
		// download-on-demand (NOT bundled in the tarball like the OCR models), so
		// the layout capability is acquired only when first used.
		Key:    "layout",
		File:   "ppdoclayoutv3.onnx",
		URL:    "https://github.com/neokapi/neokapi/releases/download/vision-models-v1/ppdoclayoutv3.onnx",
		SHA256: "bc307c102a52a10eedf20f36a03df384b8eb2224beb2e5e716c581901a8f0b61",
	},
}

// Get returns the asset with the given key.
func Get(key string) (Asset, bool) {
	for _, a := range Registry {
		if a.Key == key {
			return a, true
		}
	}
	return Asset{}, false
}

// Dir is the WRITABLE model directory — the download target — in precedence
// order:
//  1. $KAPI_VISION_MODELS_DIR (explicit override — local/offline);
//  2. $KAPI_VISION_CACHE/models/vision;
//  3. $XDG_CACHE_HOME/kapi/models/vision;
//  4. ~/.cache/kapi/models/vision.
//
// The read-only bundled dir (beside the executable, where the release tarball
// ships the OCR models) is NOT a download target — it is searched by Ensure but
// downloads (e.g. the on-demand layout model) always go to this writable dir.
func Dir() string {
	if d := os.Getenv("KAPI_VISION_MODELS_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("KAPI_VISION_CACHE"); d != "" {
		return filepath.Join(d, "models", "vision")
	}
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "kapi", "models", "vision")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "kapi", "models", "vision")
	}
	return filepath.Join(home, ".cache", "kapi", "models", "vision")
}

// searchDirs lists, in order, the directories Ensure looks in for an existing
// asset: the override, the read-only bundled dir beside the binary, then the
// writable cache. The first valid match is used; nothing is downloaded if found.
func searchDirs() []string {
	var ds []string
	if o := os.Getenv("KAPI_VISION_MODELS_DIR"); o != "" {
		ds = append(ds, o)
	}
	if b := bundledDir(); b != "" {
		ds = append(ds, b)
	}
	return append(ds, Dir())
}

// bundledDir returns "<executable-dir>/models" if it exists — the layout the
// release tarball installs — else "".
func bundledDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	cand := filepath.Join(filepath.Dir(exe), "models")
	if info, err := os.Stat(cand); err == nil && info.IsDir() {
		return cand
	}
	return ""
}

// Path is the on-disk path an asset resolves to (whether or not it exists yet).
func Path(a Asset) string { return filepath.Join(Dir(), a.File) }

// Ensure returns the local path to an asset. It first searches (override,
// bundled, cache) for an already-present file with a matching pinned hash and
// returns it as-is; otherwise it downloads to the writable cache dir, verifying
// the hash. Downloads are written atomically.
func Ensure(a Asset, logf func(string, ...any)) (string, error) {
	for _, d := range searchDirs() {
		p := filepath.Join(d, a.File)
		if ok, _ := verify(p, a.SHA256); ok {
			return p, nil
		}
	}
	dst := Path(a) // download to the writable dir
	if a.URL == "" {
		return "", fmt.Errorf("models: %s missing and no URL to fetch it", a.File)
	}
	if logf != nil {
		logf("downloading %s", a.File)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := download(a.URL, dst); err != nil {
		return "", fmt.Errorf("models: download %s: %w", a.File, err)
	}
	if ok, err := verify(dst, a.SHA256); !ok {
		_ = os.Remove(dst)
		return "", fmt.Errorf("models: %s failed hash check: %w", a.File, err)
	}
	return dst, nil
}

// verify reports whether path exists and (when want != "") its SHA-256 matches.
func verify(path, want string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	if want == "" {
		return true, nil // present, no pinned hash to check
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return false, fmt.Errorf("got %s want %s", got, want)
	}
	return true, nil
}

// download fetches url to dst via a temp file + atomic rename.
func download(url, dst string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url) //nolint:noctx // bounded by client.Timeout
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".dl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}
