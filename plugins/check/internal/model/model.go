// Package model resolves and caches the ONNX checker models and their
// tokenizer. On first explicit pull it downloads the int8 ONNX export and
// tokenizer.json into an XDG-style cache, serializing concurrent downloads with
// a lock file. Pure Go (net/http + os): it builds and unit-tests without the
// ONNX runtime or tokenizer native libraries present.
//
// Acquisition is explicit (the host calls Ensure from `kapi-check pull` or a
// plugin install step), never a silent download mid-check.
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

// Spec describes one checker model: the int8 ONNX export and the tokenizer.
type Spec struct {
	// Name is the model identifier the protocol uses.
	Name string
	// Repo is the HuggingFace repo holding the ONNX export.
	Repo string
	// ONNXFile is the file within Repo to download. We default to the int8
	// export: it is ~4x smaller and ~30x lighter in memory than fp32 with no
	// meaningful loss for similarity scoring (see the ML-benchmark dashboard).
	ONNXFile string
	// ONNXSHA256 is the lowercase hex SHA-256 of the expected ONNXFile bytes.
	// When set, the download verifies the received bytes against it and fails
	// on mismatch. When empty, the file is installed unverified (with a logged
	// warning). See "Integrity verification" in the package doc.
	//
	// TODO(security): pin the real upstream digest, e.g.:
	//   curl -sL https://huggingface.co/intfloat/multilingual-e5-small/resolve/main/onnx/model_qint8_avx512_vnni.onnx | sha256sum
	// Until pinned, downloads succeed but are flagged as unverified.
	ONNXSHA256 string
	// ONNXSize, when > 0, is the expected byte length of ONNXFile.
	ONNXSize int64
	// TokenizerRepo / TokenizerFile locate tokenizer.json.
	TokenizerRepo string
	TokenizerFile string
	// TokenizerSHA256 is the lowercase hex SHA-256 of the expected
	// tokenizer.json bytes (see ONNXSHA256). Empty means install-unverified.
	//
	// TODO(security): pin the real upstream digest, e.g.:
	//   curl -sL https://huggingface.co/intfloat/multilingual-e5-small/resolve/main/tokenizer.json | sha256sum
	TokenizerSHA256 string
	// TokenizerSize, when > 0, is the expected byte length of tokenizer.json.
	TokenizerSize int64
	// Dim is the embedding dimension (for sanity checks/info).
	Dim int
	// Default marks the plugin's default model.
	Default bool
}

// Registry is the set of models the plugin supports.
var Registry = []Spec{
	{
		Name:          "e5-small-int8",
		Repo:          "intfloat/multilingual-e5-small",
		ONNXFile:      "onnx/model_qint8_avx512_vnni.onnx",
		TokenizerRepo: "intfloat/multilingual-e5-small",
		TokenizerFile: "tokenizer.json",
		Dim:           384,
		Default:       true,
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

// Lookup returns the Spec for the named model (empty name → default).
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

// Paths holds the resolved on-disk locations of a model's files.
type Paths struct {
	Dir       string
	ONNX      string
	Tokenizer string
}

// CacheRoot returns the root cache directory for checker models:
//  1. $KAPI_CHECK_CACHE
//  2. $XDG_CACHE_HOME/kapi/models/check
//  3. ~/.cache/kapi/models/check
func CacheRoot() (string, error) {
	if v := os.Getenv("KAPI_CHECK_CACHE"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "kapi", "models", "check"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("model: resolve cache root: %w", err)
	}
	return filepath.Join(home, ".cache", "kapi", "models", "check"), nil
}

// Resolve returns where a model's files live in the cache (without downloading).
func Resolve(name string) (Paths, Spec, error) {
	spec, ok := Lookup(name)
	if !ok {
		return Paths{}, Spec{}, fmt.Errorf("model: unknown model %q", name)
	}
	root, err := CacheRoot()
	if err != nil {
		return Paths{}, Spec{}, err
	}
	dir := filepath.Join(root, spec.Name)
	return Paths{
		Dir:       dir,
		ONNX:      filepath.Join(dir, "model.onnx"),
		Tokenizer: filepath.Join(dir, "tokenizer.json"),
	}, spec, nil
}

// Present reports whether a model's files are already cached.
func Present(name string) bool {
	p, _, err := Resolve(name)
	if err != nil {
		return false
	}
	return fileNonEmpty(p.ONNX) && fileNonEmpty(p.Tokenizer)
}

func hfURL(repo, file string) string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, file)
}

// Downloader fetches a model's files into the cache.
type Downloader struct {
	HTTPClient  *http.Client
	Logf        func(format string, args ...any)
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

// Ensure downloads the named model's files if missing and returns their paths.
// This is the explicit acquisition step.
func (d *Downloader) Ensure(ctx context.Context, name string) (Paths, error) {
	p, spec, err := Resolve(name)
	if err != nil {
		return Paths{}, err
	}
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return Paths{}, fmt.Errorf("model: create cache dir: %w", err)
	}
	if err := d.ensureFile(ctx, p.ONNX, hfURL(spec.Repo, spec.ONNXFile), expected{sha256: spec.ONNXSHA256, size: spec.ONNXSize}); err != nil {
		return Paths{}, fmt.Errorf("model: onnx: %w", err)
	}
	if err := d.ensureFile(ctx, p.Tokenizer, hfURL(spec.TokenizerRepo, spec.TokenizerFile), expected{sha256: spec.TokenizerSHA256, size: spec.TokenizerSize}); err != nil {
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

func (d *Downloader) ensureFile(ctx context.Context, dst, url string, want expected) error {
	if fileNonEmpty(dst) {
		return nil
	}
	unlock, err := d.acquireLock(ctx, dst+".lock")
	if err != nil {
		return err
	}
	defer unlock()
	if fileNonEmpty(dst) {
		return nil
	}
	return d.download(ctx, url, dst, want)
}

func fileNonEmpty(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 0
}

func (d *Downloader) acquireLock(ctx context.Context, lockPath string) (func(), error) {
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
		if fi, statErr := os.Stat(lockPath); statErr == nil && time.Since(fi.ModTime()) > d.lockTimeout() {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("model: timed out waiting for download lock %s", lockPath)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("model: waiting for download lock %s: %w", lockPath, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// download streams url to a temp file, verifies its integrity, then atomically
// renames it to dst so a reader never sees a half-written or unverified model.
// The SHA-256 of the received bytes is always computed; when want pins a digest
// (or size) the download fails on mismatch before the rename, otherwise a
// warning is logged that the file was installed unverified.
func (d *Downloader) download(ctx context.Context, url, dst string, want expected) error {
	d.logf("downloading %s -> %s", url, dst)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // fixed registry of HuggingFace repos
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
