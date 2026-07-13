package prompt

import "context"

// Prompt IDs. A rendered prompt carries its ID so a call can be identified
// without inspecting its text.
const (
	IDTranslateSingle = "translate.single"
	IDTranslateBatch  = "translate.batch"
	IDBrandInfer      = "brand.infer"
)

// CorpusDelimiter separates a prompt's instructions from a bulk corpus appended
// after it. It is a declared part of the prompt contract — the demo provider
// locates the corpus by it — rather than incidental wording, so it must not be
// changed casually.
const CorpusDelimiter = "\nCorpus:\n"

// Meta describes what a call is doing, independent of the prompt's wording.
//
// It exists so nothing has to reverse-engineer a prompt's text to recover its
// intent. The demo provider fabricates deterministic output and needs to know
// the target locale; it used to regex "to <locale>" out of the prompt string,
// which silently broke whenever the prompt was reworded. Consumers read Meta
// instead. The --explain recorder uses it to label captured calls.
type Meta struct {
	// ID identifies the prompt, e.g. IDTranslateSingle.
	ID string
	// Version is the prompt revision that rendered the call (see Version).
	Version string
	// Params carries the structured inputs a consumer may need — for translate,
	// "source_locale" and "target_locale".
	Params map[string]string
}

// Param returns a parameter, or "" when absent.
func (m Meta) Param(key string) string { return m.Params[key] }

type metaKey struct{}

// WithMeta attaches prompt metadata to ctx. Every prompt-rendering call site
// should do this, so providers and the prompt recorder see structured intent
// alongside the rendered messages.
func WithMeta(ctx context.Context, m Meta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

// MetaFrom returns the prompt metadata attached to ctx, if any.
func MetaFrom(ctx context.Context) (Meta, bool) {
	m, ok := ctx.Value(metaKey{}).(Meta)
	return m, ok
}

// Meta describes this translate call.
func (t Translate) Meta(id string) Meta {
	return Meta{
		ID:      id,
		Version: Version,
		Params: map[string]string{
			"source_locale": string(t.SourceLocale),
			"target_locale": string(t.TargetLocale),
		},
	}
}
