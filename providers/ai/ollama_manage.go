package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/neokapi/neokapi/core/httputil"
)

// OllamaManager drives an Ollama server's model lifecycle — detecting that the
// server is up, listing installed models, and pulling new ones — so kapi can
// install the model a translation needs without the user dropping to a separate
// `ollama` shell. It speaks the same HTTP API as OllamaProvider but covers the
// management endpoints (/api/version, /api/tags, /api/pull) rather than
// inference.
type OllamaManager struct {
	baseURL string
	client  *http.Client
}

// DefaultOllamaBaseURL is the address Ollama listens on out of the box.
const DefaultOllamaBaseURL = "http://localhost:11434"

// NewOllamaManager creates a manager for the Ollama server at baseURL (empty →
// the default localhost endpoint).
func NewOllamaManager(baseURL string) *OllamaManager {
	if baseURL == "" {
		baseURL = DefaultOllamaBaseURL
	}
	return &OllamaManager{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  httputil.NewResilientClient(),
	}
}

// RecommendedOllamaModel is a curated Ollama model kapi suggests for local
// translation, with a one-line rationale. Surfaced by `kapi models list` so a
// user sees sensible choices before pulling anything.
type RecommendedOllamaModel struct {
	Name string
	Note string
}

// RecommendedOllamaModels are the vetted local-translation picks (from kapi's
// own benchmarking), ordered best-default first. DefaultOllamaModel is the head.
// gemma4:e2b is the quality tier (best multilingual grammar, e.g. German) but is
// larger and needs a recent Ollama; gemma4:e4b is intentionally absent — it drops
// inline placeholder tags, which is disqualifying for localization.
var RecommendedOllamaModels = []RecommendedOllamaModel{
	{Name: DefaultOllamaModel, Note: "default · smallest, exact inline-tag fidelity"},
	{Name: "gemma4:e2b", Note: "best multilingual quality · ~7 GB, recent Ollama"},
	{Name: "qwen3:1.7b", Note: "fastest · smallest viable"},
	{Name: "aya-expanse:8b", Note: "high quality · slower"},
}

// OllamaModelInfo describes one model already installed on the server.
type OllamaModelInfo struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
	Family     string `json:"family,omitempty"`
	Parameters string `json:"parameters,omitempty"`
	Quant      string `json:"quantization,omitempty"`
}

// PullProgress is one progress report from a streaming model pull. Total and
// Completed are byte counts for the current layer (0 until a layer download
// begins); Status is Ollama's human phase string (e.g. "pulling manifest",
// "downloading", "verifying sha256 digest", "success").
type PullProgress struct {
	Status    string
	Digest    string
	Total     int64
	Completed int64
}

// Version returns the running server's version, or an actionable error when no
// server is reachable at the configured address.
func (m *OllamaManager) Version(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/api/version", nil)
	if err != nil {
		return "", err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", ollamaUnreachableError(m.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama: version check failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", fmt.Errorf("ollama: decode version: %w", err)
	}
	return v.Version, nil
}

// Reachable reports whether an Ollama server is responding at the configured
// address.
func (m *OllamaManager) Reachable(ctx context.Context) bool {
	_, err := m.Version(ctx)
	return err == nil
}

// List returns the models already installed on the server.
func (m *OllamaManager) List(ctx context.Context) ([]OllamaModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, ollamaUnreachableError(m.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama: list models failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Models []OllamaModelInfo `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ollama: decode model list: %w", err)
	}
	return out.Models, nil
}

// Has reports whether a model is already installed. An unqualified name (no
// ":tag") also matches the model's ":latest" tag, mirroring Ollama's own
// shorthand.
func (m *OllamaManager) Has(ctx context.Context, name string) (bool, error) {
	models, err := m.List(ctx)
	if err != nil {
		return false, err
	}
	want := name
	if !strings.Contains(want, ":") {
		want += ":latest"
	}
	for _, mi := range models {
		if mi.Name == name || mi.Name == want {
			return true, nil
		}
	}
	return false, nil
}

// Pull installs a model, invoking onProgress (if non-nil) for each progress
// frame Ollama streams. It returns when the pull completes or fails. onProgress
// must not block.
func (m *OllamaManager) Pull(ctx context.Context, name string, onProgress func(PullProgress)) error {
	body, err := json.Marshal(map[string]any{"name": name, "stream": true})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return ollamaUnreachableError(m.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama: pull %q failed (%d): %s", name, resp.StatusCode, strings.TrimSpace(string(b)))
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var frame struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if err := dec.Decode(&frame); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("ollama: decode pull progress: %w", err)
		}
		if frame.Error != "" {
			return fmt.Errorf("ollama: pull %q: %s", name, frame.Error)
		}
		if onProgress != nil {
			onProgress(PullProgress{
				Status:    frame.Status,
				Digest:    frame.Digest,
				Total:     frame.Total,
				Completed: frame.Completed,
			})
		}
	}
}

// Delete removes an installed model from the server (DELETE /api/delete).
func (m *OllamaManager) Delete(ctx context.Context, name string) error {
	body, err := json.Marshal(map[string]any{"name": name})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, m.baseURL+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return ollamaUnreachableError(m.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(b))
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("ollama: model %q is not installed", name)
		}
		return fmt.Errorf("ollama: delete %q failed (%d): %s", name, resp.StatusCode, msg)
	}
	return nil
}

// EnsureModel pulls a model only if it is not already installed. It returns
// (pulled, error): pulled is true when a download was performed.
func (m *OllamaManager) EnsureModel(ctx context.Context, name string, onProgress func(PullProgress)) (bool, error) {
	has, err := m.Has(ctx, name)
	if err != nil {
		return false, err
	}
	if has {
		return false, nil
	}
	if err := m.Pull(ctx, name, onProgress); err != nil {
		return false, err
	}
	return true, nil
}

// ollamaUnreachableError turns a connection-level failure into guidance: the
// usual cause is that no Ollama server is running at baseURL. Shared by the
// provider and the manager so the message is identical everywhere.
func ollamaUnreachableError(baseURL string, err error) error {
	return fmt.Errorf("ollama: cannot reach Ollama at %s — is it running? Start it with `ollama serve`, or install it from https://ollama.com (underlying error: %w)", baseURL, err)
}
