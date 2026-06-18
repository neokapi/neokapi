// Package models resolves and caches the RapidOCR / PP-OCRv4 ONNX model assets
// the OCR engine loads: text detection (DBNet), angle classification, and
// recognition (CRNN+CTC), plus the recognition character dictionary. It mirrors
// the SaT plugin's model cache: assets download on first use and live under an
// XDG cache, with a content-hash check; a local override dir short-circuits the
// download for development and offline/bundled use.
//
// Asset SHA-256s are pinned to the RapidOCR v4 *mobile* models. The dictionary
// is the standard PP-OCR keys file.
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

// Registry is the default PP-OCRv4 mobile asset set.
var Registry = []Asset{
	{
		Key:    "det",
		File:   "ch_PP-OCRv4_det_mobile.onnx",
		URL:    "https://www.modelscope.cn/models/RapidAI/RapidOCR/resolve/master/onnx/PP-OCRv4/ch_PP-OCRv4_det_mobile.onnx",
		SHA256: "d2a7720d45a54257208b1e13e36a8479894cb74155a5efe29462512d42f49da9",
	},
	{
		Key:    "cls",
		File:   "ch_ppocr_mobile_v2.0_cls_mobile.onnx",
		URL:    "https://www.modelscope.cn/models/RapidAI/RapidOCR/resolve/master/onnx/PP-OCRv4/ch_ppocr_mobile_v2.0_cls_mobile.onnx",
		SHA256: "e47acedf663230f8863ff1ab0e64dd2d82b838fceb5957146dab185a89d6215c",
	},
	{
		Key:    "rec",
		File:   "ch_PP-OCRv4_rec_mobile.onnx",
		URL:    "https://www.modelscope.cn/models/RapidAI/RapidOCR/resolve/master/onnx/PP-OCRv4/ch_PP-OCRv4_rec_mobile.onnx",
		SHA256: "48fc40f24f6d2a207a2b1091d3437eb3cc3eb6b676dc3ef9c37384005483683b",
	},
	{
		Key:    "dict",
		File:   "ppocr_keys_v1.txt",
		URL:    "https://raw.githubusercontent.com/PaddlePaddle/PaddleOCR/main/ppocr/utils/ppocr_keys_v1.txt",
		SHA256: "a1c84d9bdb9ab29043c58896224d32941783eb821629618416dcb08f12886492",
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

// Dir resolves the model directory in precedence order:
//  1. $KAPI_VISION_MODELS_DIR (explicit override — local/offline/bundled);
//  2. $KAPI_VISION_CACHE;
//  3. $XDG_CACHE_HOME/kapi/models/vision;
//  4. ~/.cache/kapi/models/vision.
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

// Path is the on-disk path an asset resolves to (whether or not it exists yet).
func Path(a Asset) string { return filepath.Join(Dir(), a.File) }

// Ensure returns the local path to an asset, downloading and verifying it if it
// is not already present. A present file with a matching pinned hash (or no
// pinned hash) is used as-is. Downloads are written atomically.
func Ensure(a Asset, logf func(string, ...any)) (string, error) {
	dst := Path(a)
	if ok, _ := verify(dst, a.SHA256); ok {
		return dst, nil
	}
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
