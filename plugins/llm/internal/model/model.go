// Package model resolves and caches the Gemma 4 ONNX model files, tokenizer,
// and processor configs used by the kapi-llm local-LLM plugin.
//
// Gemma 4 ships as a transformers.js-style SPLIT export: four ONNX graphs
// (embed_tokens, vision_encoder, audio_encoder, decoder_model_merged), each
// with one or more external-data siblings (`*.onnx_data`), plus a tokenizer and
// a handful of JSON configs. On first use of a model this package downloads all
// of those into an XDG-style cache directory, preserving each file's basename so
// the external-data references inside the .onnx files resolve when onnxruntime
// memory-maps them.
//
// # Variant
//
// The plugin uses the q4 variant — 4-bit-quantized weights with float32 tensor
// I/O. The Go onnxruntime binding has no native float16 tensor type, so the
// fp16/q4f16 graphs (whose inputs, KV cache, and logits are float16) would
// require custom-typed tensors throughout; q4 keeps the whole pipeline in
// float32 with no precision-relevant difference for generation. This mirrors the
// kapi-sat plugin's choice of the float32 model.onnx over the fp16 graph.
//
// # Integrity verification
//
// Each downloaded file is verified before it is committed to the cache. When a
// File pins an expected SHA-256 the download fails on any mismatch, so a
// tampered upstream artifact cannot poison the cache that feeds the in-process
// onnxruntime. When a File does NOT pin a digest the download still proceeds but
// logs a clear "installed unverified" warning.
//
// This package is pure Go (net/http + os) and has no cgo dependency, so it can
// be compiled and unit-tested without the ONNX runtime present.
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
	"path"
	"path/filepath"
	"strings"
	"time"
)

// File describes one file to fetch from a model's HuggingFace repo.
type File struct {
	// RepoPath is the path within the repo on the main branch
	// (e.g. "onnx/decoder_model_merged_q4.onnx" or "tokenizer.json").
	RepoPath string
	// SHA256 is the lowercase hex SHA-256 the downloaded bytes must match. When
	// empty the file is installed unverified (with a logged warning).
	//
	// TODO(security): pin the real upstream digests, e.g.:
	//   curl -sL https://huggingface.co/onnx-community/gemma-4-E2B-it-ONNX/resolve/main/onnx/decoder_model_merged_q4.onnx | sha256sum
	SHA256 string
	// Size, when > 0, is the expected byte length, checked alongside SHA256.
	Size int64
}

// Base returns the on-disk basename the file is stored under.
func (f File) Base() string { return path.Base(f.RepoPath) }

// Spec describes one model: where to fetch it and the role each file plays. All
// files are stored flat in the model's cache directory under their basename, so
// the .onnx external-data references (which name siblings by basename) resolve.
type Spec struct {
	// Name is the model identifier the protocol uses (e.g. "gemma-4-e2b").
	Name string
	// Repo is the HuggingFace repo holding the ONNX export.
	Repo string

	// The four component graphs. Audio/Vision may be empty for a text-only
	// model; embed + decoder are always required.
	Embed   File
	Decoder File
	Vision  File
	Audio   File
	// Data holds the external-data siblings (`*.onnx_data`) that must accompany
	// the component graphs.
	Data []File

	// Tokenizer + processor/config JSON.
	Tokenizer          File
	Config             File
	GenerationConfig   File
	PreprocessorConfig File
	ProcessorConfig    File

	// Default marks the plugin's default model.
	Default bool
}

// allFiles returns every file the spec needs, skipping empty optional ones.
func (s Spec) allFiles() []File {
	out := make([]File, 0, 8+len(s.Data))
	for _, f := range []File{s.Embed, s.Decoder, s.Vision, s.Audio} {
		if f.RepoPath != "" {
			out = append(out, f)
		}
	}
	out = append(out, s.Data...)
	for _, f := range []File{s.Tokenizer, s.Config, s.GenerationConfig, s.PreprocessorConfig, s.ProcessorConfig} {
		if f.RepoPath != "" {
			out = append(out, f)
		}
	}
	return out
}

// onnxRepo is the canonical ONNX export of Gemma 4 E2B (instruction-tuned).
const onnxRepo = "onnx-community/gemma-4-E2B-it-ONNX"

// Registry is the set of models the plugin supports. Gemma 4 E2B is a
// multimodal (text + image + audio) on-device model; the q4 variant keeps all
// tensor I/O in float32.
var Registry = []Spec{
	{
		Name:    "gemma-4-e2b",
		Repo:    onnxRepo,
		Default: true,
		Embed:   File{RepoPath: "onnx/embed_tokens_q4.onnx"},
		Decoder: File{RepoPath: "onnx/decoder_model_merged_q4.onnx"},
		Vision:  File{RepoPath: "onnx/vision_encoder_q4.onnx"},
		Audio:   File{RepoPath: "onnx/audio_encoder_q4.onnx"},
		Data: []File{
			{RepoPath: "onnx/embed_tokens_q4.onnx_data"},
			{RepoPath: "onnx/decoder_model_merged_q4.onnx_data"},
			{RepoPath: "onnx/vision_encoder_q4.onnx_data"},
			{RepoPath: "onnx/audio_encoder_q4.onnx_data"},
		},
		Tokenizer:          File{RepoPath: "tokenizer.json"},
		Config:             File{RepoPath: "config.json"},
		GenerationConfig:   File{RepoPath: "generation_config.json"},
		PreprocessorConfig: File{RepoPath: "preprocessor_config.json"},
		ProcessorConfig:    File{RepoPath: "processor_config.json"},
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

// Paths holds the resolved on-disk locations of a model's files.
type Paths struct {
	// Dir is the model's cache directory (all files live here, flat).
	Dir string
	// Component graph paths. Vision/Audio are "" for a text-only model.
	Embed   string
	Decoder string
	Vision  string
	Audio   string
	// Tokenizer + config paths.
	Tokenizer          string
	Config             string
	GenerationConfig   string
	PreprocessorConfig string
	ProcessorConfig    string
}

// CacheRoot returns the root cache directory for kapi-llm models. Precedence:
//
//  1. $KAPI_LLM_CACHE (explicit override)
//  2. $XDG_CACHE_HOME/kapi/models/llm
//  3. ~/.cache/kapi/models/llm
func CacheRoot() (string, error) {
	if v := os.Getenv("KAPI_LLM_CACHE"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "kapi", "models", "llm"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("model: resolve cache root: %w", err)
	}
	return filepath.Join(home, ".cache", "kapi", "models", "llm"), nil
}

// hfURL builds a HuggingFace resolve URL for a file in a repo on the main branch.
func hfURL(repo, repoPath string) string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, repoPath)
}

// Downloader fetches a model's files into the cache. The zero value is usable.
type Downloader struct {
	// HTTPClient is used for downloads. Defaults to a long-timeout client
	// suitable for multi-GB ONNX files.
	HTTPClient *http.Client
	// Logf, if non-nil, receives human-readable progress lines. Defaults no-op.
	Logf func(format string, args ...any)
	// LockTimeout bounds how long Ensure waits for the per-file download lock.
	// Defaults to 30 minutes (model files are large).
	LockTimeout time.Duration
}

func (d *Downloader) httpClient() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return &http.Client{Timeout: 2 * time.Hour}
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
	return 30 * time.Minute
}

// Ensure makes sure every file the named model needs is present and valid in the
// cache, downloading any that are missing, and returns their paths. Concurrent
// calls (across goroutines or processes) for the same files are serialized by a
// lock file so a partially written file is never observed.
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

	// Gemma's ONNX files total several GB, so report progress per file and across
	// the whole set ("[2/13] file: 45%"). The other ML plugins log a single line
	// per (much smaller) file; this is the same Logf→stderr→host channel, just
	// progress-aware to match the larger multi-file download.
	files := spec.allFiles()
	for i, f := range files {
		dst := filepath.Join(dir, f.Base())
		label := fmt.Sprintf("[%d/%d] %s", i+1, len(files), f.Base())
		if err := d.ensureFile(dst, hfURL(spec.Repo, f.RepoPath), label, expected{sha256: f.SHA256, size: f.Size}); err != nil {
			return Paths{}, fmt.Errorf("model: %s: %w", f.RepoPath, err)
		}
	}

	at := func(f File) string {
		if f.RepoPath == "" {
			return ""
		}
		return filepath.Join(dir, f.Base())
	}
	return Paths{
		Dir:                dir,
		Embed:              at(spec.Embed),
		Decoder:            at(spec.Decoder),
		Vision:             at(spec.Vision),
		Audio:              at(spec.Audio),
		Tokenizer:          at(spec.Tokenizer),
		Config:             at(spec.Config),
		GenerationConfig:   at(spec.GenerationConfig),
		PreprocessorConfig: at(spec.PreprocessorConfig),
		ProcessorConfig:    at(spec.ProcessorConfig),
	}, nil
}

// expected carries the optional pinned integrity values for a downloaded file.
type expected struct {
	sha256 string
	size   int64
}

// ensureFile downloads url to dst if dst is missing or empty, serialized by a
// sibling lock file so two processes cannot corrupt the cache. label is a
// human-readable prefix (e.g. "[2/13] decoder_model_merged_q4.onnx") used in
// progress logs.
func (d *Downloader) ensureFile(dst, url, label string, want expected) error {
	if fileNonEmpty(dst) {
		return nil
	}
	unlock, err := d.acquireLock(dst + ".lock")
	if err != nil {
		return err
	}
	defer unlock()

	if fileNonEmpty(dst) {
		return nil
	}
	return d.download(url, dst, label, want)
}

func fileNonEmpty(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Size() > 0
}

// acquireLock creates lockPath exclusively, retrying until it succeeds or the
// timeout elapses. A stale lock older than the timeout is reclaimed so a crashed
// downloader cannot wedge the cache forever.
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
// renames it to dst so a reader never sees a half-written or unverified file.
// label prefixes progress logs (e.g. "[2/13] decoder_model_merged_q4.onnx").
func (d *Downloader) download(url, dst, label string, want expected) error {
	if label == "" {
		label = path.Base(dst)
	}
	d.logf("downloading %s", label)
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
	// Stream through a progress reader so the host sees percentage updates for
	// the large (multi-hundred-MB) model files, throttled to avoid log spam.
	pr := &progressReader{r: resp.Body, total: resp.ContentLength, label: label, logf: d.logf}
	n, err := io.Copy(io.MultiWriter(tmp, h), pr)
	if err != nil {
		return fmt.Errorf("copy body: %w", err)
	}
	pr.done(n)
	if n == 0 {
		return fmt.Errorf("get %s: empty response body", url)
	}
	if resp.ContentLength > 0 && n != resp.ContentLength {
		return fmt.Errorf("get %s: size mismatch: got %d want %d", url, n, resp.ContentLength)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

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

// progressReader wraps a download body and emits throttled percent/byte progress
// through logf, so the host (and ultimately the user) sees movement during the
// multi-GB Gemma download instead of a silent stall. Updates are rate-limited to
// at most once per second and only when the integer percent changes.
type progressReader struct {
	r       io.Reader
	total   int64
	read    int64
	label   string
	logf    func(string, ...any)
	lastLog time.Time
	lastPct int
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	p.maybeLog(false)
	return n, err
}

func (p *progressReader) maybeLog(force bool) {
	now := time.Now()
	if !force && now.Sub(p.lastLog) < time.Second {
		return
	}
	p.lastLog = now
	if p.total > 0 {
		pct := int(p.read * 100 / p.total)
		if !force && pct == p.lastPct {
			return
		}
		p.lastPct = pct
		p.logf("  %s: %d%% (%s / %s)", p.label, pct, humanBytes(p.read), humanBytes(p.total))
		return
	}
	p.logf("  %s: %s", p.label, humanBytes(p.read))
}

// done emits a final 100% line.
func (p *progressReader) done(n int64) {
	p.read = n
	if p.total <= 0 {
		p.total = n
	}
	p.maybeLog(true)
}

// humanBytes formats a byte count as a short human-readable string.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
