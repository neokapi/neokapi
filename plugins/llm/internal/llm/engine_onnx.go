//go:build onnx

// This file is the cgo-backed Gemma 4 generation engine. It is compiled only
// with `-tags onnx`, when the onnxruntime and tokenizer native libraries must be
// present and linkable. The default build uses engine_stub.go instead, so the
// module (and the chat/sample tests) build with no native dependency.
//
// # Pipeline
//
// Gemma 4 ships as a split ONNX export. Generation threads four graphs:
//
//	input_ids ─▶ embed_tokens ─▶ inputs_embeds + per_layer_inputs
//	                                  │
//	      (media positions replaced)  ▼
//	  image ─▶ vision_encoder ─▶ image_features ─┐
//	  audio ─▶ audio_encoder  ─▶ audio_features ─┴▶ decoder_model_merged ─▶ logits
//	                                                  ▲   (+ KV cache loop)
//
// The decoder graph contains the entire transformer (RoPE, attention, the
// alternating full/sliding-window mask, norms); this Go code never implements
// attention — it only marshals tensors and threads the KV cache from one step's
// `present.*` outputs into the next step's `past_key_values.*` inputs.
//
// # Validation status
//
// The text path (tokenize → embed → KV-cache decode → sample → detokenize) is
// structurally complete. The multimodal splice (vision/audio encoder outputs
// replacing placeholder rows in inputs_embeds) is wired in media_onnx.go. Exact
// numerics — tensor dtypes, the num_logits_to_keep rank, image/audio
// preprocessing constants — are validated on-device against the real q4 weights;
// session I/O names and dtypes are introspected at load (not hardcoded) so the
// engine adapts to the export rather than guessing.
package llm

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"

	"github.com/daulet/tokenizers"
	ort "github.com/yalue/onnxruntime_go"

	"github.com/neokapi/neokapi/plugins/llm/internal/model"
)

// readFile is a thin wrapper kept so the rest of the file needs no direct os
// import beyond this.
func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

// onnxEngine is the real Engine. It owns the onnxruntime environment and a cache
// of loaded models keyed by name. It is safe for concurrent use.
type onnxEngine struct {
	dl   *model.Downloader
	logf func(string, ...any)

	mu     sync.Mutex
	models map[string]*loadedModel
}

// loadedModel bundles a model's tokenizer, config, and ONNX sessions plus the
// introspected decoder I/O plan.
type loadedModel struct {
	tk  *tokenizers.Tokenizer
	cfg modelConfig

	embed   *ort.DynamicAdvancedSession
	decoder *ort.DynamicAdvancedSession
	vision  *ort.DynamicAdvancedSession // nil if the model has no vision encoder
	audio   *ort.DynamicAdvancedSession // nil if the model has no audio encoder

	// embed I/O: one input (input_ids), two outputs (inputs_embeds + per_layer_inputs).
	embedIn     string
	embedOut    []string // ordered output names
	embedEmbeds string   // name of the inputs_embeds output
	embedPLE    string   // name of the per_layer_inputs output (may be "")

	// decoder I/O plan (ordered names + role classification).
	decIn     []string
	decOut    []string
	decInRole map[string]inputRole
	logitsOut string
	// pastToPresent maps a past_key_values.* input name to its present.* output.
	presentToPast map[string]string
	// pastDims holds each past_key_values.* input's declared shape. Gemma 4's
	// hybrid attention gives sliding-window layers a wider KV head_dim than
	// full-attention layers (e.g. 512 vs 256), so the empty cache must be built
	// per-input from the model, not from a single config head_dim.
	pastDims map[string][]int64

	// vision/audio I/O names.
	visionIn, visionOut []string
	audioIn, audioOut   []string

	// runMu serializes ORT Run calls (sessions are not concurrency-safe).
	runMu *sync.Mutex
}

// inputRole classifies a decoder input so the per-step builder knows what to
// supply for it.
type inputRole int

const (
	roleEmbeds inputRole = iota
	rolePerLayer
	roleAttnMask
	rolePositionIDs
	roleNumLogits
	rolePastKV
	roleUnknown
)

// NewEngine creates the ONNX-backed engine.
func NewEngine(cacheLogf func(string, ...any)) (Engine, error) {
	if cacheLogf == nil {
		cacheLogf = func(string, ...any) {}
	}
	if err := initORT(); err != nil {
		return nil, err
	}
	return &onnxEngine{
		dl:     &model.Downloader{Logf: cacheLogf},
		logf:   cacheLogf,
		models: map[string]*loadedModel{},
	}, nil
}

func (e *onnxEngine) Loaded(name string) bool {
	if name == "" {
		name = model.DefaultModelName()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.models[name]
	return ok
}

// Modalities advertises only text for v0.1.0. Gemma 4's vision and audio
// encoders are wired (engine sessions load and the embed-splice works), but
// their preprocessing — Gemma's native-resolution patchified pixel_values
// ([N,768] 16×16×3 patches + [N,2] position ids) and the log-mel audio features
// ([B,frames,128] + bool mask) — is not yet numerically validated against
// reference outputs, so image/audio input is gated off (see encodeMedia). The
// text generation path is validated on-device. Tracked for a follow-up.
func (e *onnxEngine) Modalities() []string { return nil }

func (e *onnxEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, m := range e.models {
		m.destroy()
	}
	e.models = map[string]*loadedModel{}
	// Do NOT DestroyEnvironment here: it is process-global and initialized once
	// via initORT's sync.Once; tearing it down would break any later engine. The
	// OS reclaims it at process exit (mirrors the vision plugin).
	return nil
}

func (m *loadedModel) destroy() {
	for _, s := range []*ort.DynamicAdvancedSession{m.embed, m.decoder, m.vision, m.audio} {
		if s != nil {
			s.Destroy()
		}
	}
	if m.tk != nil {
		m.tk.Close()
	}
}

// get returns the loaded model for name, loading (and caching) it on first use.
func (e *onnxEngine) get(name string) (*loadedModel, error) {
	if name == "" {
		name = model.DefaultModelName()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if m, ok := e.models[name]; ok {
		return m, nil
	}
	paths, err := e.dl.Ensure(name)
	if err != nil {
		return nil, err
	}

	cfg, err := loadConfig(paths.Config, paths.GenerationConfig)
	if err != nil {
		return nil, err
	}

	tkBytes, err := readFile(paths.Tokenizer)
	if err != nil {
		return nil, fmt.Errorf("llm: read tokenizer: %w", err)
	}
	tk, err := tokenizers.FromBytes(tkBytes)
	if err != nil {
		return nil, fmt.Errorf("llm: load tokenizer: %w", err)
	}

	m := &loadedModel{tk: tk, cfg: cfg, runMu: &sync.Mutex{}}
	if err := m.openEmbed(paths.Embed); err != nil {
		m.destroy()
		return nil, err
	}
	if err := m.openDecoder(paths.Decoder); err != nil {
		m.destroy()
		return nil, err
	}
	if paths.Vision != "" {
		if err := m.openVision(paths.Vision); err != nil {
			m.destroy()
			return nil, err
		}
	}
	if paths.Audio != "" {
		if err := m.openAudio(paths.Audio); err != nil {
			m.destroy()
			return nil, err
		}
	}

	e.models[name] = m
	e.logf("loaded model %s (%d layers, %d kv-heads, head_dim %d)", name, cfg.NumLayers, cfg.NumKVHeads, cfg.HeadDim)
	return m, nil
}

// openEmbed opens embed_tokens and records its input + the inputs_embeds /
// per_layer_inputs output names.
func (m *loadedModel) openEmbed(path string) error {
	ins, outs, err := ort.GetInputOutputInfo(path)
	if err != nil {
		return fmt.Errorf("llm: inspect embed_tokens: %w", err)
	}
	if len(ins) == 0 || len(outs) == 0 {
		return fmt.Errorf("llm: embed_tokens has no inputs/outputs")
	}
	m.embedIn = ins[0].Name
	for _, o := range outs {
		m.embedOut = append(m.embedOut, o.Name)
		switch {
		case strings.Contains(o.Name, "per_layer"):
			m.embedPLE = o.Name
		case strings.Contains(o.Name, "embed"):
			m.embedEmbeds = o.Name
		}
	}
	if m.embedEmbeds == "" {
		m.embedEmbeds = outs[0].Name // fall back to first output
	}
	m.embed, err = ort.NewDynamicAdvancedSession(path, []string{m.embedIn}, m.embedOut, nil)
	if err != nil {
		return fmt.Errorf("llm: embed_tokens session: %w", err)
	}
	return nil
}

// openDecoder opens decoder_model_merged and classifies every input by role so
// the per-step builder can supply the right tensor, and maps present→past KV.
func (m *loadedModel) openDecoder(path string) error {
	ins, outs, err := ort.GetInputOutputInfo(path)
	if err != nil {
		return fmt.Errorf("llm: inspect decoder: %w", err)
	}
	m.decInRole = map[string]inputRole{}
	m.pastDims = map[string][]int64{}
	for _, in := range ins {
		role := classifyDecoderInput(in.Name)
		m.decIn = append(m.decIn, in.Name)
		m.decInRole[in.Name] = role
		if role == rolePastKV {
			m.pastDims[in.Name] = append([]int64(nil), in.Dimensions...)
		}
	}
	m.presentToPast = map[string]string{}
	for _, out := range outs {
		m.decOut = append(m.decOut, out.Name)
		switch {
		case strings.Contains(out.Name, "logits"):
			m.logitsOut = out.Name
		case strings.HasPrefix(out.Name, "present"):
			// present.<i>.<key|value> → past_key_values.<i>.<key|value>
			past := strings.Replace(out.Name, "present", "past_key_values", 1)
			m.presentToPast[out.Name] = past
		}
	}
	if m.logitsOut == "" && len(outs) > 0 {
		m.logitsOut = outs[0].Name
	}
	m.decoder, err = ort.NewDynamicAdvancedSession(path, m.decIn, m.decOut, nil)
	if err != nil {
		return fmt.Errorf("llm: decoder session: %w", err)
	}
	return nil
}

// classifyDecoderInput maps a decoder input name to its role.
func classifyDecoderInput(name string) inputRole {
	switch {
	case strings.HasPrefix(name, "past_key_values"):
		return rolePastKV
	case strings.Contains(name, "per_layer"):
		return rolePerLayer
	case strings.Contains(name, "inputs_embeds"):
		return roleEmbeds
	case strings.Contains(name, "attention_mask"):
		return roleAttnMask
	case strings.Contains(name, "position_ids"):
		return rolePositionIDs
	case strings.Contains(name, "logits_to_keep") || strings.Contains(name, "num_logits"):
		return roleNumLogits
	default:
		return roleUnknown
	}
}

// Generate runs the full generation pipeline.
func (e *onnxEngine) Generate(params GenerateParams) (Result, error) {
	m, err := e.get(params.Model)
	if err != nil {
		return Result{}, err
	}

	maxTokens := params.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	var rng *rand.Rand
	if params.Temperature > 0 {
		// Deterministic-enough seed derived from the prompt length; generation
		// reproducibility is not a correctness requirement here.
		rng = rand.New(rand.NewSource(int64(len(params.Messages)*1000 + maxTokens)))
	}

	// Serialize ORT calls per engine (sessions are not concurrency-safe). The
	// media encoders run under the same lock as the decode loop.
	m.mu().Lock()
	defer m.mu().Unlock()

	// Build the prompt token stream. When messages carry media, this runs the
	// vision/audio encoders and splices placeholder tokens whose embeddings are
	// overwritten with the encoder outputs (see media_onnx.go).
	ids, slots, err := m.buildPromptIDs(params.Messages)
	if err != nil {
		return Result{}, err
	}
	if len(ids) == 0 {
		return Result{}, nil
	}

	gen := &genState{m: m, params: params, rng: rng, maxTokens: maxTokens, slots: slots}
	text, out, err := gen.run(ids)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: cleanOutput(text), InputTokens: len(ids), OutputTokens: out}, nil
}

// mu returns the per-model run mutex (initialized in get). Sessions are shared
// across calls and not concurrency-safe, so every ORT Run is serialized on it.
func (m *loadedModel) mu() *sync.Mutex { return m.runMu }
