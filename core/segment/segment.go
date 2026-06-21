// Package segment provides pluggable sentence- and chunk-segmentation engines
// that produce run-anchored stand-off overlays (AD-002). Segmentation never
// rewrites the runs it describes: an engine returns ordered, non-overlapping
// [model.Span] ranges that the segment tool attaches as a [model.Overlay], so
// segmentation stays opt-in, multi-layer, and reversible.
//
// Each engine registers an [EngineDescriptor] at init under a short name (srx,
// uax29, llm, sat), mirroring the aiprovider/mtprovider pattern. A descriptor is
// self-describing: it carries the engine's selector label/description, a
// constructor for the engine's own typed configuration ([EngineConfig]), and a
// builder that takes the shared [BaseConfig] plus that config. This lets every
// engine evolve its parameters independently in its own package while the
// umbrella segmentation tool composes them through one abstraction — it holds no
// engine-specific knowledge.
//
// The framework registers the SRX engine (the default — UAX-29 base + Okapi SRX
// exceptions where ICU is linked, pure-Go SRX rules otherwise) and, where ICU is
// linked, the UAX-29 baseline; the AI tools register the LLM chunker; the CLI
// wires the out-of-process SaT model engine. An engine that is not linked into a
// given binary simply isn't registered — [Build] then reports
// [ErrEngineUnavailable] rather than failing the build.
package segment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
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

// BaseConfig carries the options every segmenter shares: how inline codes are
// flattened before boundary detection (and how breaks adjacent to codes resolve)
// and an optional locale override. Engine-specific parameters live in each
// engine's own [EngineConfig], not here.
type BaseConfig struct {
	// Mask controls how inline codes are represented to the boundary engine
	// and how breaks adjacent to codes are resolved.
	Mask MaskOptions

	// Language overrides the BCP-47 locale used to select rules (SRX language
	// map, UAX-29 break-iterator locale). Empty = use the per-call locale.
	Language string
}

// EngineDescriptor is the self-describing registration for a segmenter — the one
// abstraction every engine implements, whether in-process (srx, uax29, llm) or
// plugin-provided over a Mode-C daemon. It carries identity for the selector UI,
// the engine's own parameter schema, and a builder that takes the shared
// [BaseConfig] plus the engine's parameters (the subset of the unified config
// map the engine understands). Engines evolve their parameters independently;
// the segmentation tool composes them through this descriptor alone.
type EngineDescriptor struct {
	Name        string // short registry name (srx, uax29, llm, sat)
	Label       string // human label for the engine selector
	Description string // selector help text
	Order       int    // selector ordering; lower sorts first

	// Schema is the engine's parameter schema for the form, or nil when the
	// engine takes no parameters. Built-in engines build it from their config
	// struct (schema.FromStruct); plugin engines supply the schema loaded from
	// their manifest.
	Schema *schema.ComponentSchema

	// New builds the Segmenter from the shared base options and the engine's
	// parameters. params is the subset of the unified config map relevant to this
	// engine; built-in engines decode it into their own config, plugin engines
	// forward it to the daemon. params is never nil (callers pass an empty map).
	New func(base BaseConfig, params map[string]any) (Segmenter, error)
}

var (
	mu      sync.RWMutex
	engines = map[string]EngineDescriptor{}
)

// ErrEngineUnavailable is returned by [Build] for a name that no linked package
// registered.
var ErrEngineUnavailable = errors.New("segment: engine not available")

// Register records an engine descriptor. It panics on an empty name, a missing
// New, or a duplicate name, matching the framework's other init-time registries.
func Register(d EngineDescriptor) {
	if d.Name == "" || d.New == nil {
		panic("segment: Register requires a name and New")
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := engines[d.Name]; dup {
		panic("segment: engine already registered: " + d.Name)
	}
	engines[d.Name] = d
}

// RegisterIfAbsent records an engine descriptor only when no engine of that
// name is registered, reporting whether it was added. Unlike [Register] it never
// panics on a duplicate — it is the host's idempotent path for wiring
// plugin-provided engines, which may be re-scanned repeatedly and must not
// clobber a built-in (or earlier plugin) of the same name.
func RegisterIfAbsent(d EngineDescriptor) bool {
	if d.Name == "" || d.New == nil {
		return false
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := engines[d.Name]; dup {
		return false
	}
	engines[d.Name] = d
	return true
}

// Lookup returns the descriptor registered under name. An empty name selects
// [DefaultEngine].
func Lookup(name string) (EngineDescriptor, bool) {
	if name == "" {
		name = DefaultEngine
	}
	mu.RLock()
	defer mu.RUnlock()
	d, ok := engines[name]
	return d, ok
}

// Build constructs the named engine from base options and the engine's
// parameters (nil is treated as an empty map). An empty name selects
// [DefaultEngine]. It returns [ErrEngineUnavailable] (wrapped) when the engine
// is not linked.
func Build(name string, base BaseConfig, params map[string]any) (Segmenter, error) {
	d, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q (have: %v)", ErrEngineUnavailable, name, Engines())
	}
	if params == nil {
		params = map[string]any{}
	}
	return d.New(base, params)
}

// HasEngine reports whether an engine is registered under name.
func HasEngine(name string) bool {
	_, ok := Lookup(name)
	return ok
}

// Engines returns the registered engine names, sorted alphabetically.
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

// Descriptors returns the registered engine descriptors, ordered by
// [EngineDescriptor.Order] then name — the order the engine selector presents
// them in.
func Descriptors() []EngineDescriptor {
	mu.RLock()
	out := make([]EngineDescriptor, 0, len(engines))
	for _, d := range engines {
		out = append(out, d)
	}
	mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out
}
