package cli

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

	"golang.org/x/sync/errgroup"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// Model assets are large data dependencies a plugin declares in its manifest
// (manifest.ModelAsset) and the HOST fetches, verifies, and caches on the
// plugin's behalf — so the plugin binary stays a pure compute engine and asset
// downloads are uniform with the rest of kapi (one shared cache, one progress
// renderer, mandatory integrity). The model lives under
//
//	$XDG_CACHE_HOME/kapi/models/<plugin>/<id>/<version>/<file-basename>
//
// flat, so sibling references (e.g. ONNX external-data) resolve. Files stream to
// a temp sibling, are SHA-256 (and size) verified, then atomically renamed in,
// so a reader never observes a partial or unverified file. A per-file lock
// serializes concurrent installs across processes.

// ModelCacheRoot returns the root cache directory for host-managed model assets.
// Precedence: $KAPI_MODELS_CACHE, then $XDG_CACHE_HOME/kapi/models, then
// ~/.cache/kapi/models.
func ModelCacheRoot() (string, error) {
	if v := os.Getenv("KAPI_MODELS_CACHE"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "kapi", "models"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("model assets: resolve cache root: %w", err)
	}
	return filepath.Join(home, ".cache", "kapi", "models"), nil
}

// ModelDir returns the on-disk directory a (plugin, id, version) model is cached
// under. It does not create the directory.
func ModelDir(plugin, id, version string) (string, error) {
	root, err := ModelCacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, plugin, id, version), nil
}

// ModelEnsureOptions configures EnsureModel.
type ModelEnsureOptions struct {
	// Plugin namespaces the cache (the manifest's plugin name).
	Plugin string
	// HTTPClient overrides the default client (tests). A long timeout suiting
	// multi-GB files is used when nil.
	HTTPClient *http.Client
	// Logf receives progress/log lines when stderr is not a terminal. Optional.
	Logf func(format string, args ...any)
	// Concurrency bounds simultaneous file downloads. Defaults to 3.
	Concurrency int
	// LockTimeout bounds how long EnsureModel waits for a per-file lock.
	// Defaults to 30 minutes (model files are large).
	LockTimeout time.Duration
}

func (o ModelEnsureOptions) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: 2 * time.Hour}
}

func (o ModelEnsureOptions) concurrency() int {
	if o.Concurrency > 0 {
		return o.Concurrency
	}
	return 3
}

func (o ModelEnsureOptions) lockTimeout() time.Duration {
	if o.LockTimeout > 0 {
		return o.LockTimeout
	}
	return 30 * time.Minute
}

// EnsureModel makes sure every file the asset declares is present and verified
// in the cache, downloading any that are missing — concurrently, with a progress
// bar — and returns the model's cache directory. Already-present files are a
// pure cache hit (no requests, no bars). It is safe to call concurrently across
// processes; per-file locks serialize installs.
func EnsureModel(ctx context.Context, asset manifest.ModelAsset, opts ModelEnsureOptions) (string, error) {
	if opts.Plugin == "" {
		return "", errors.New("model assets: plugin name is required")
	}
	if asset.ID == "" || asset.Version == "" {
		return "", errors.New("model assets: asset id and version are required")
	}
	dir, err := ModelDir(opts.Plugin, asset.ID, asset.Version)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("model assets: create cache dir: %w", err)
	}

	// Figure out which files actually need fetching, so a cache hit shows no UI.
	// Every path is re-validated here even though the manifest contract already
	// rejects non-basenames: EnsureModel is a reusable exported entry point and
	// must not trust that its ModelAsset came through manifest.Validate.
	var todo []manifest.ModelFile
	for _, f := range asset.Files {
		if err := validateModelFilePath(f.Path); err != nil {
			return "", err
		}
		if !modelFilePresent(filepath.Join(dir, f.Path), f.Size) {
			todo = append(todo, f)
		}
	}
	if len(todo) == 0 {
		return dir, nil
	}

	prog := newDownloadProgress(opts.Logf)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.concurrency())
	for _, f := range todo {
		g.Go(func() error {
			return ensureModelFile(gctx, dir, f, prog, opts)
		})
	}
	err = g.Wait()
	prog.wait()
	if err != nil {
		return "", fmt.Errorf("model assets: %s/%s: %w", asset.ID, asset.Version, err)
	}
	return dir, nil
}

// validateModelFilePath rejects anything that is not a clean, relative,
// single-segment basename, so a malicious or buggy ModelAsset cannot place,
// stat, or lock a file outside the model cache dir via "../", an absolute path,
// or a nested directory. Guards both the cache-hit stat and the download/lock
// paths, which all join dir + f.Path.
func validateModelFilePath(p string) error {
	if p == "" || p == "." || p == ".." || filepath.IsAbs(p) ||
		strings.ContainsAny(p, "/\\") || filepath.Clean(p) != p {
		return fmt.Errorf("model assets: invalid file path %q (must be a bare basename)", p)
	}
	return nil
}

// modelFilePresent reports whether dst already holds the file: it exists, is
// non-empty, and — when a size is pinned — matches it. A size mismatch forces a
// re-download (the cached file is stale or truncated).
func modelFilePresent(dst string, wantSize int64) bool {
	fi, err := os.Stat(dst)
	if err != nil || fi.Size() == 0 {
		return false
	}
	if wantSize > 0 && fi.Size() != wantSize {
		return false
	}
	return true
}

// ensureModelFile downloads one file under a sibling lock so two processes can't
// corrupt the cache, re-checking presence once the lock is held.
func ensureModelFile(ctx context.Context, dir string, f manifest.ModelFile, prog downloadProgress, opts ModelEnsureOptions) error {
	dst := filepath.Join(dir, f.Path)
	unlock, err := acquireModelLock(dst+".lock", opts.lockTimeout())
	if err != nil {
		return err
	}
	defer unlock()
	if modelFilePresent(dst, f.Size) {
		return nil // another process won the race
	}
	return downloadModelFile(ctx, dst, f, prog, opts)
}

// downloadModelFile streams f.URL to a temp sibling, verifies size + SHA-256,
// then atomically renames it into place. The progress bar advances as bytes
// arrive and is aborted on any failure.
func downloadModelFile(ctx context.Context, dst string, f manifest.ModelFile, prog downloadProgress, opts ModelEnsureOptions) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return fmt.Errorf("request %s: %w", f.URL, err)
	}
	resp, err := opts.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", f.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: unexpected status %s", f.URL, resp.Status)
	}

	total := f.Size
	if total <= 0 {
		total = resp.ContentLength
	}
	bar := prog.file(f.Path, total)

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".download-*")
	if err != nil {
		bar.abort()
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // no-op after a successful rename
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), bar.wrap(resp.Body))
	if err != nil {
		bar.abort()
		return fmt.Errorf("copy %s: %w", f.URL, err)
	}
	if err := tmp.Close(); err != nil {
		bar.abort()
		return fmt.Errorf("close temp: %w", err)
	}

	if f.Size > 0 && n != f.Size {
		bar.abort()
		return fmt.Errorf("get %s: integrity check failed: size %d does not match pinned %d", f.URL, n, f.Size)
	}
	gotSum := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(gotSum, f.SHA256) {
		bar.abort()
		return fmt.Errorf("get %s: integrity check failed: sha256 %s does not match pinned %s", f.URL, gotSum, f.SHA256)
	}
	bar.done(n)

	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("rename temp into place: %w", err)
	}
	return nil
}

// acquireModelLock creates lockPath exclusively, retrying until it succeeds or
// the timeout elapses. A stale lock older than the timeout is reclaimed so a
// crashed downloader cannot wedge the cache forever.
func acquireModelLock(lockPath string, timeout time.Duration) (func(), error) {
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire lock: %w", err)
		}
		if fi, statErr := os.Stat(lockPath); statErr == nil && time.Since(fi.ModTime()) > timeout {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for download lock %s", lockPath)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
