// Package model resolves and caches the SaT ONNX model files and the
// XLM-RoBERTa tokenizer used by the kapi-sat segmenter.
//
// On first use of a model the package downloads the ONNX export and the
// tokenizer.json into an XDG-style cache directory and serializes concurrent
// downloads with a per-file lock file so two processes cannot corrupt the
// cache.
//
// # Integrity verification
//
// Each downloaded file is verified before it is committed to the cache. When a
// Spec pins an expected SHA-256 (and, optionally, a size) for a file, the
// download computes the SHA-256 of the received bytes and refuses to install
// the file (failing the download) on any mismatch — so a tampered upstream
// artifact, mirror, or CA-compromised transfer cannot poison the cache that
// feeds the in-process onnxruntime. When a Spec does NOT pin a digest the
// download still proceeds, but a clear warning is logged noting that the file
// was installed unverified; pin a hash in the Registry to close that gap.
//
// This package is pure Go (net/http + os) and has no cgo dependency, so it can
// be compiled and unit-tested without the ONNX runtime or tokenizer native
// libraries present.
package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Spec describes one SaT model: where to fetch its ONNX export and the base
// tokenizer, and what to call the files on disk.
type Spec struct {
	// Name is the model identifier the protocol uses (e.g. "sat-3l-sm").
	Name string
	// Repo is the HuggingFace repo holding the ONNX export
	// (e.g. "segment-any-text/sat-3l-sm").
	Repo string
	// ONNXFile is the file within Repo to download. SaT publishes both
	// "model.onnx" (float32 I/O) and "model_optimized.onnx" (float16 I/O).
	// We use the float32 "model.onnx": the Go onnxruntime binding's tensor
	// types are float32/float64/int only (no native float16), so feeding the
	// fp16-optimized graph would require a custom-typed tensor. The float32
	// graph is numerically equivalent for our boundary thresholding.
	ONNXFile string
	// ONNXSHA256 is the lowercase hex SHA-256 of the expected ONNXFile bytes.
	// When set, the download verifies the received bytes against it and fails
	// on mismatch. When empty, the file is installed unverified (with a logged
	// warning). See "Integrity verification" in the package doc.
	//
	// TODO(security): pin the real upstream digests here. Compute them from the
	// authoritative HuggingFace artifacts, e.g.:
	//   curl -sL https://huggingface.co/segment-any-text/sat-3l-sm/resolve/main/model.onnx | sha256sum
	// Until pinned, downloads succeed but are flagged as unverified.
	ONNXSHA256 string
	// ONNXSize, when > 0, is the expected byte length of ONNXFile and is
	// checked alongside ONNXSHA256.
	ONNXSize int64
	// TokenizerRepo is the HuggingFace repo holding tokenizer.json. The SaT
	// model repos do NOT ship a tokenizer; they reuse the xlm-roberta-base
	// tokenizer, so this points at the base model.
	TokenizerRepo string
	// TokenizerSHA256 is the lowercase hex SHA-256 of the expected
	// tokenizer.json bytes (see ONNXSHA256). Empty means install-unverified.
	//
	// TODO(security): pin the real upstream digest, e.g.:
	//   curl -sL https://huggingface.co/facebookAI/xlm-roberta-base/resolve/main/tokenizer.json | sha256sum
	TokenizerSHA256 string
	// TokenizerSize, when > 0, is the expected byte length of tokenizer.json.
	TokenizerSize int64
	// Default marks the plugin's default model.
	Default bool
}

// Registry is the set of models the plugin supports. SaT *-sm models share the
// xlm-roberta-base tokenizer (SentencePiece unigram) and the same
// SubwordXLMForTokenClassification head (single boundary logit per token).
var Registry = []Spec{
	{
		Name:          "sat-3l-sm",
		Repo:          "segment-any-text/sat-3l-sm",
		ONNXFile:      "model.onnx",
		TokenizerRepo: "facebookAI/xlm-roberta-base",
		Default:       true,
	},
	{
		Name:          "sat-12l-sm",
		Repo:          "segment-any-text/sat-12l-sm",
		ONNXFile:      "model.onnx",
		TokenizerRepo: "facebookAI/xlm-roberta-base",
	},
}

// DefaultModelName returns the registry's default model name.
func DefaultModelName() string {
	for _, s := range Registry {
		if s.Default {
			return s.Name
		}
	}
	if len(Registry) > 0 {
		return Registry[0].Name
	}
	return ""
}

// Lookup returns the Spec for the named model, or false if unknown. An empty
// name resolves to the default model.
func Lookup(name string) (Spec, bool) {
	if name == "" {
		name = DefaultModelName()
	}
	for _, s := range Registry {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

// tokenizerFile is the canonical on-disk filename for the tokenizer.
const tokenizerFile = "tokenizer.json"

// Paths holds the resolved on-disk locations of a model's files.
type Paths struct {
	// Dir is the model's cache directory.
	Dir string
	// ONNX is the absolute path to the ONNX model file.
	ONNX string
	// Tokenizer is the absolute path to tokenizer.json.
	Tokenizer string
}

// CacheRoot returns the root cache directory for SaT models. The order of
// precedence is:
//
//  1. $KAPI_SAT_CACHE (explicit override)
//  2. $XDG_CACHE_HOME/kapi/models/sat
//  3. ~/.cache/kapi/models/sat
func CacheRoot() (string, error) {
	if v := os.Getenv("KAPI_SAT_CACHE"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "kapi", "models", "sat"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("model: resolve cache root: %w", err)
	}
	return filepath.Join(home, ".cache", "kapi", "models", "sat"), nil
}

// hfURL builds a HuggingFace resolve URL for a file in a repo on the main
// branch.
func hfURL(repo, file string) string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, file)
}

// Downloader fetches a model's files into the cache. The zero value is usable;
// HTTPClient and Logf may be set to customize behavior.
type Downloader struct {
	// HTTPClient is used for downloads. Defaults to a client with a long
	// timeout suitable for multi-hundred-MB ONNX files.
	HTTPClient *http.Client
	// Logf, if non-nil, receives human-readable progress lines (e.g. for
	// printing to stderr). Defaults to a no-op.
	Logf func(format string, args ...any)
	// LockTimeout bounds how long Ensure waits to acquire the per-file
	// download lock before giving up. Defaults to 10 minutes.
	LockTimeout time.Duration
}

func (d *Downloader) httpClient() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Minute}
}

func (d *Downloader) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}

func (d *Downloader) lockTimeout() time.Duration {
	if d.LockTimeout > 0 {
		return d.LockTimeout
	}
	return 10 * time.Minute
}

// Ensure makes sure the named model's ONNX file and tokenizer.json are present
// and valid in the cache, downloading any that are missing, and returns their
// paths. Concurrent calls (across goroutines or processes) for the same files
// are serialized by a lock file so a partially written file is never observed.
func (d *Downloader) Ensure(name string) (Paths, error) {
	spec, ok := Lookup(name)
	if !ok {
		return Paths{}, fmt.Errorf("model: unknown model %q", name)
	}
	root, err := CacheRoot()
	if err != nil {
		return Paths{}, err
	}
	dir := filepath.Join(root, spec.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Paths{}, fmt.Errorf("model: create cache dir: %w", err)
	}
	p := Paths{
		Dir:       dir,
		ONNX:      filepath.Join(dir, "model.onnx"),
		Tokenizer: filepath.Join(dir, tokenizerFile),
	}

	if err := d.ensureFile(p.ONNX, hfURL(spec.Repo, spec.ONNXFile), expected{sha256: spec.ONNXSHA256, size: spec.ONNXSize}); err != nil {
		return Paths{}, fmt.Errorf("model: onnx: %w", err)
	}
	if err := d.ensureFile(p.Tokenizer, hfURL(spec.TokenizerRepo, tokenizerFile), expected{sha256: spec.TokenizerSHA256, size: spec.TokenizerSize}); err != nil {
		return Paths{}, fmt.Errorf("model: tokenizer: %w", err)
	}
	return p, nil
}

// expected carries the optional pinned integrity values for a downloaded file.
// A zero value means "no pin" — the file is installed unverified (with a logged
// warning).
type expected struct {
	// sha256 is the lowercase hex SHA-256 the downloaded bytes must match.
	sha256 string
	// size, when > 0, is the expected byte length.
	size int64
}

// ensureFile downloads url to dst if dst is missing or empty. It acquires a
// sibling lock file first; if another process already produced the file while
// we waited, we use it.
func (d *Downloader) ensureFile(dst, url string, want expected) error {
	if fileNonEmpty(dst) {
		return nil
	}
	unlock, err := d.acquireLock(dst + ".lock")
	if err != nil {
		return err
	}
	defer unlock()

	// Re-check under the lock: another process may have finished while we
	// waited.
	if fileNonEmpty(dst) {
		return nil
	}
	return d.download(url, dst, want)
}

func fileNonEmpty(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 0
}

// acquireLock creates lockPath exclusively, retrying until it succeeds or the
// timeout elapses. The returned func removes the lock file. A stale lock older
// than the timeout is reclaimed so a crashed downloader cannot wedge the cache
// forever.
func (d *Downloader) acquireLock(lockPath string) (func(), error) {
	deadline := time.Now().Add(d.lockTimeout())
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("model: acquire lock: %w", err)
		}
		// Reclaim a stale lock.
		if fi, statErr := os.Stat(lockPath); statErr == nil && time.Since(fi.ModTime()) > d.lockTimeout() {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("model: timed out waiting for download lock %s", lockPath)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// download streams url to a temp file, verifies its integrity, then atomically
// renames it to dst so a reader never sees a half-written or unverified model.
// The SHA-256 of the received bytes is always computed; when want pins a digest
// (or size) the download fails on mismatch before the rename, otherwise a
// warning is logged that the file was installed unverified.
func (d *Downloader) download(url, dst string, want expected) error {
	d.logf("downloading %s -> %s", url, dst)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil) //nolint:gosec // URL built from a fixed registry of HuggingFace repos
	if err != nil {
		return fmt.Errorf("request %s: %w", url, err)
	}
	resp, err := d.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: unexpected status %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".download-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // no-op after a successful rename
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if err != nil {
		return fmt.Errorf("copy body: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("get %s: empty response body", url)
	}
	// If the server advertised a length, verify it.
	if resp.ContentLength > 0 && n != resp.ContentLength {
		return fmt.Errorf("get %s: size mismatch: got %d want %d", url, n, resp.ContentLength)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	// Integrity verification: refuse to install bytes that do not match a
	// pinned digest/size. This runs BEFORE the rename, so a mismatching file is
	// removed (by the deferred cleanup) and never enters the cache.
	gotSum := hex.EncodeToString(h.Sum(nil))
	if want.size > 0 && n != want.size {
		return fmt.Errorf("get %s: integrity check failed: size %d does not match pinned %d", url, n, want.size)
	}
	if want.sha256 != "" {
		if !strings.EqualFold(gotSum, want.sha256) {
			return fmt.Errorf("get %s: integrity check failed: sha256 %s does not match pinned %s", url, gotSum, want.sha256)
		}
		d.logf("verified %d bytes, sha256=%s (matches pinned digest)", n, gotSum)
	} else {
		d.logf("WARNING: installed %d bytes UNVERIFIED (no pinned sha256 for %s); sha256=%s", n, url, gotSum)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("rename temp into place: %w", err)
	}
	return nil
}
