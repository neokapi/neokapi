// Package segment provides pluggable sentence- and chunk-segmentation engines
// that produce run-anchored stand-off overlays (AD-002). Segmentation never
// rewrites the runs it describes: an engine returns ordered, non-overlapping
// [model.Span] ranges that the segment tool attaches as a [model.Overlay], so
// segmentation stays opt-in, multi-layer, and reversible.
//
// Engines register at init under a short name (srx, uax29, llm, sat) into a
// global registry, mirroring the aiprovider/mtprovider pattern. The framework
// registers the pure-Go SRX engine (the default) and, where ICU is linked, the
// UAX-29 baseline; the AI tools register the LLM "llm-chunk" engine; the CLI
// wires the out-of-process SaT model engine. An engine that is not linked into
// a given binary simply isn't registered — [NewEngine] then reports
// [ErrEngineUnavailable] rather than failing the build.
package segment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// DefaultEngine is the engine used when a caller does not name one.
const DefaultEngine = "srx"

// Layer names for segmentation overlays. The empty string is the primary
// sentence segmentation that bilingual formats project to and from; named
// layers are additional interpretations that coexist over the same runs.
const (
	LayerSentence = "" // primary sentence segmentation
	LayerLLMChunk = "llm-chunk"
	LayerClause   = "clause"
)

// Segmenter computes segmentation spans over a run sequence in a given locale.
// The returned spans are run-anchored (see [model.RunRange]) and never mutate
// the runs — segmentation is a stand-off overlay. Spans must be ordered by
// position and non-overlapping; runs that no span covers are implicit
// inter-segment material ("ignorables").
type Segmenter interface {
	Segment(ctx context.Context, runs []model.Run, loc model.LocaleID) ([]model.Span, error)
	// Layer reports the natural overlay layer this engine produces (e.g.
	// [LayerSentence] for SRX/UAX-29/SaT, [LayerLLMChunk] for the LLM engine).
	// A caller may override the layer when attaching the overlay.
	Layer() string
}

// Config carries engine parameters resolved from the segment tool's config.
// Each engine reads only the fields it understands; Params is an escape hatch
// for engine-specific options not modelled here.
type Config struct {
	// Mask controls how inline codes are represented to the boundary engine
	// and how breaks adjacent to codes are resolved (shared by all engines).
	Mask MaskOptions

	// Language overrides the BCP-47 locale used to select rules (SRX language
	// map, UAX-29 break-iterator locale). Empty = use the per-call locale.
	Language string

	// SRX engine.
	SrxPath  string // path to an SRX 2.0 rules file (resolved by the caller)
	SrxRules string // inline SRX 2.0 XML (takes precedence over SrxPath)

	// LLM engine.
	Provider      string // aiprovider id (anthropic, openai, …)
	Model         string
	Credential    string
	APIKey        string
	BaseURL       string
	Instruction   string // optional chunking instruction
	MaxChunkRunes int    // soft upper bound on chunk size (0 = engine default)

	// SaT (ML) engine.
	SatModel   string  // sat-3l-sm | sat-12l-sm | …
	Threshold  float64 // boundary probability threshold (0 = model default)
	PluginPath string  // path to the kapi-sat plugin binary (resolved by CLI)

	// Params is an escape hatch for engine-specific options.
	Params map[string]any
}

// Factory builds a Segmenter from a Config.
type Factory func(cfg Config) (Segmenter, error)

var (
	mu      sync.RWMutex
	engines = map[string]Factory{}
)

// ErrEngineUnavailable is returned by [NewEngine] for a name that no linked
// package registered.
var ErrEngineUnavailable = errors.New("segment: engine not available")

// RegisterEngine registers an engine factory under a short name. It panics on
// a duplicate name, matching the framework's other init-time registries.
func RegisterEngine(name string, f Factory) {
	if name == "" || f == nil {
		panic("segment: RegisterEngine requires a name and factory")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := engines[name]; dup {
		panic("segment: engine already registered: " + name)
	}
	engines[name] = f
}

// NewEngine builds the named engine. An empty name selects [DefaultEngine].
// It returns [ErrEngineUnavailable] (wrapped) when the engine is not linked.
func NewEngine(name string, cfg Config) (Segmenter, error) {
	if name == "" {
		name = DefaultEngine
	}
	mu.RLock()
	f := engines[name]
	mu.RUnlock()
	if f == nil {
		return nil, fmt.Errorf("%w: %q (have: %v)", ErrEngineUnavailable, name, Engines())
	}
	return f(cfg)
}

// HasEngine reports whether an engine is registered under name.
func HasEngine(name string) bool {
	if name == "" {
		name = DefaultEngine
	}
	mu.RLock()
	defer mu.RUnlock()
	_, ok := engines[name]
	return ok
}

// Engines returns the registered engine names, sorted.
func Engines() []string {
	mu.RLock()
	out := make([]string, 0, len(engines))
	for name := range engines {
		out = append(out, name)
	}
	mu.RUnlock()
	sort.Strings(out)
	return out
}
