package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// llmSegmenter is the "llm" segmentation engine: it asks an LLM to split a
// block's masked text into coherent semantic chunks suitable for translation,
// then aligns those chunks back to rune-offset breaks for the overlay model
// (AD-002). It is registered under the name "llm" and produces the
// [segment.LayerLLMChunk] layer.
//
// All model access goes through the [aiprovider.LLMProvider] interface so tests
// drive it with the mock provider — there are no direct network calls here.
type llmSegmenter struct {
	provider      aiprovider.LLMProvider
	language      string // optional locale override from cfg.Language
	instruction   string // optional chunking instruction from cfg.Instruction
	maxChunkRunes int    // soft size hint; 0 = no hint
	mask          segment.MaskOptions
}

// init wires the LLM engine into the global segment registry, mirroring the
// aiprovider/mtprovider init-time registration pattern.
func init() {
	segment.RegisterEngine("llm", newLLMSegmenter)
}

// newLLMSegmenter builds the LLM engine from a resolved [segment.Config]. The
// provider id defaults to the same default as translate (anthropic) when
// cfg.Provider is empty. Credential resolution (keychain → cfg.APIKey) is the
// integrator's responsibility; this constructor only consumes the resolved
// APIKey / Provider / Model / BaseURL fields.
func newLLMSegmenter(cfg segment.Config) (segment.Segmenter, error) {
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("segment llm: %w", err)
	}
	if p == nil {
		return nil, fmt.Errorf("segment llm: no provider for %q", cfg.Provider)
	}
	return &llmSegmenter{
		provider:      p,
		language:      cfg.Language,
		instruction:   cfg.Instruction,
		maxChunkRunes: cfg.MaxChunkRunes,
		mask:          cfg.Mask,
	}, nil
}

// Layer reports the overlay layer this engine produces.
func (s *llmSegmenter) Layer() string { return segment.LayerLLMChunk }

// chunkSchema constrains the structured response to {"chunks": ["...", ...]}.
func chunkSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "semantic_chunks",
		Description: "Ordered, contiguous slices of the input text",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chunks": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
			},
			"required":             []string{"chunks"},
			"additionalProperties": false,
		},
	}
}

// chunkResult is the JSON structure returned by the structured chat call.
type chunkResult struct {
	Chunks []string `json:"chunks"`
}

// Segment splits runs into LLM-derived semantic chunks and returns run-anchored
// spans. On any failure — provider error, malformed JSON, or chunks that cannot
// be aligned to the source — it degrades gracefully to nil (whole-block, no
// segmentation) rather than returning an error or invalid spans.
func (s *llmSegmenter) Segment(ctx context.Context, runs []model.Run, loc model.LocaleID) ([]model.Span, error) {
	fl := segment.Flatten(runs, s.mask)
	txt := fl.Text()
	if strings.TrimSpace(txt) == "" {
		return nil, nil
	}

	lang := s.language
	if lang == "" {
		lang = string(loc)
	}

	prompt := s.buildPrompt(txt, lang)
	resp, err := s.provider.ChatStructured(ctx,
		[]aiprovider.Message{aiprovider.TextMessage("user", prompt)},
		chunkSchema(),
	)
	if err != nil || resp == nil {
		// Provider failed: leave the block whole.
		return nil, nil
	}

	var result chunkResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return nil, nil
	}
	if len(result.Chunks) < 2 {
		// Zero or one chunk means "no interior boundary" — whole block.
		return nil, nil
	}

	breaks := alignChunks([]rune(txt), result.Chunks)
	if breaks == nil {
		// Alignment failed; degrade to whole-block rather than guess.
		return nil, nil
	}
	return fl.Spans(breaks), nil
}

// buildPrompt renders the chunking instruction sent to the model. It asks for a
// JSON object of contiguous chunks that reconstruct the input, honoring an
// optional user instruction and a soft size hint.
func (s *llmSegmenter) buildPrompt(txt, lang string) string {
	var b strings.Builder
	b.WriteString("Split the following text into coherent, contiguous chunks suitable for translation. ")
	b.WriteString("Prefer sentence or clause boundaries so each chunk stands on its own. ")
	b.WriteString("Every chunk must be a verbatim, contiguous slice of the input text; ")
	b.WriteString("concatenating the chunks in order (ignoring leading/trailing whitespace) must reconstruct the input exactly. ")
	b.WriteString("Do not translate, paraphrase, reorder, add, or drop any content.")
	if lang != "" {
		fmt.Fprintf(&b, " The source language is %s.", lang)
	}
	if s.maxChunkRunes > 0 {
		fmt.Fprintf(&b, " Aim for chunks no longer than about %d characters, but never split mid-word.", s.maxChunkRunes)
	}
	if instr := strings.TrimSpace(s.instruction); instr != "" {
		b.WriteString("\n\nAdditional instruction: ")
		b.WriteString(instr)
	}
	b.WriteString("\n\nReturn a JSON object {\"chunks\": [\"...\", ...]}.\n\nText:\n")
	b.WriteString(txt)
	return b.String()
}

// alignChunks converts an ordered chunk list into strictly-increasing interior
// break offsets (rune indices into txt, each the position at which a new chunk
// begins). It walks txt once and greedily matches each chunk's whitespace-
// normalized content, tolerating whitespace differences and small model drift
// by scanning forward a bounded window.
//
// It returns the interior breaks (the boundary at the end of each chunk except
// the last). On failure — a chunk that cannot be located, or boundaries that do
// not advance — it returns nil so the caller can degrade to whole-block.
func alignChunks(txt []rune, chunks []string) []int {
	n := len(txt)
	breaks := make([]int, 0, len(chunks))
	cursor := 0 // first rune of txt not yet consumed by a previous chunk

	for ci, chunk := range chunks {
		chunkRunes := normalizeWS([]rune(chunk))
		if len(chunkRunes) == 0 {
			// An empty chunk carries no boundary; skip it without advancing.
			continue
		}

		// Skip leading whitespace in txt before matching the chunk content.
		start := cursor
		for start < n && unicode.IsSpace(txt[start]) {
			start++
		}
		if start >= n {
			return nil
		}

		end, ok := matchChunk(txt, start, chunkRunes)
		if !ok {
			return nil
		}

		// The boundary sits at the end of this chunk's matched content. For the
		// last chunk there is no interior break.
		if ci < len(chunks)-1 {
			if end <= cursor || end >= n {
				return nil
			}
			breaks = append(breaks, end)
		}
		cursor = end
	}

	if len(breaks) == 0 {
		return nil
	}
	return breaks
}

// matchChunk tries to match the whitespace-normalized chunk content against
// txt starting at or shortly after `start`, returning the exclusive rune offset
// just past the matched content. It scans forward a bounded window to tolerate
// minor model drift (e.g. a chunk that begins a few runes later than expected).
func matchChunk(txt []rune, start int, chunk []rune) (int, bool) {
	const window = 8 // how many starting positions to try when drifting
	n := len(txt)
	for off := 0; off <= window && start+off < n; off++ {
		if end, ok := matchAt(txt, start+off, chunk); ok {
			return end, true
		}
		// Only drift across whitespace; never skip real content silently.
		if !unicode.IsSpace(txt[start+off]) {
			break
		}
	}
	return 0, false
}

// matchAt compares the whitespace-normalized chunk against txt from position p,
// consuming runs of whitespace flexibly (any run of whitespace in either side
// matches any run of whitespace in the other). Returns the exclusive end offset
// in txt on a full match.
func matchAt(txt []rune, p int, chunk []rune) (int, bool) {
	ti, ci := p, 0
	n := len(txt)
	for ci < len(chunk) {
		if unicode.IsSpace(chunk[ci]) {
			// Normalized chunk uses single spaces; match >=1 space in txt.
			if ti >= n || !unicode.IsSpace(txt[ti]) {
				return 0, false
			}
			for ti < n && unicode.IsSpace(txt[ti]) {
				ti++
			}
			ci++ // skip the single normalized space
			continue
		}
		if ti >= n || txt[ti] != chunk[ci] {
			return 0, false
		}
		ti++
		ci++
	}
	return ti, true
}

// normalizeWS collapses every run of whitespace to a single space and trims
// leading/trailing whitespace, so chunk content compares equal regardless of
// the model's whitespace rendering.
func normalizeWS(rs []rune) []rune {
	out := make([]rune, 0, len(rs))
	prevSpace := false
	for _, r := range rs {
		if unicode.IsSpace(r) {
			if !prevSpace {
				out = append(out, ' ')
				prevSpace = true
			}
			continue
		}
		out = append(out, r)
		prevSpace = false
	}
	// Trim leading/trailing single space.
	if len(out) > 0 && out[0] == ' ' {
		out = out[1:]
	}
	if len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}
	return out
}
