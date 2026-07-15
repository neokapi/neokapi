package aiprovider

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// The model catalog is the single source of truth for the models kapi ships
// support for — their ceilings, their provider, and their lifecycle.
//
// It exists because that knowledge used to be scattered: a DefaultXModel constant
// per provider, a prefix→ceilings map in limits.go, a price table under
// scripts/batcheval, and nothing at all for the question a user actually asks —
// "is this model current, or has it been superseded, and when does it retire?"
// Four places to update when a vendor ships a model, and no place that answered
// the lifecycle question. Now there is one file, and the rest derive from it.
//
// It is DATA (models.json, embedded), not code, and it is CURATED, not generated:
// the vendors' APIs return what is live today as a flat list of ids, but they do
// not say when a model entered neokapi, what replaced it, or when it retires. Those
// are our facts about our support, and they have to be maintained by hand — with
// `scripts/modelcheck` as the drift alarm that flags when the curated list and the
// live one disagree. See scripts/prompts/update-model-catalog.md.

//go:embed models.json
var modelsJSON []byte

// ModelStatus is where a model sits in its neokapi life.
//
// There is no "retired" status: a model the provider has stopped serving is
// removed from the catalog outright, not kept as a tombstone. The catalog is the
// list of models kapi supports, and a dead model supports nothing. An upcoming
// retirement is recorded as a date on a still-live entry (RetirementDate), not as
// a terminal state.
type ModelStatus string

const (
	// StatusActive: current and supported; a model you would reach for today.
	StatusActive ModelStatus = "active"
	// StatusSuperseded: still callable, but a newer model is preferred. Kept in
	// the catalog because a user may still name it and kapi must know its ceilings.
	StatusSuperseded ModelStatus = "superseded"
)

// Model is one entry in the catalog: a model family kapi supports, keyed by the
// id prefix used to match a concrete (possibly dated) model id against it.
type Model struct {
	// ID is the canonical model id, and doubles as the match prefix: a dated
	// snapshot (claude-sonnet-5-20260115) resolves to the "claude-sonnet-5" entry.
	ID string `json:"id"`
	// Provider is the id of the provider that serves it.
	Provider ProviderID `json:"provider"`
	// Label is the human name ("Claude Sonnet 5").
	Label string `json:"label"`
	// Aliases are the bare names that resolve to this model — the claude-code
	// subscription aliases (sonnet, opus, haiku) resolve here.
	Aliases []string `json:"aliases,omitempty"`
	// DefaultFor lists the providers whose built-in default this model is. A model
	// can be one provider's current default while being superseded elsewhere (Azure
	// still defaults to gpt-4o), which is exactly the kind of fact worth surfacing.
	DefaultFor []ProviderID `json:"default_for,omitempty"`

	// Recommended reports whether this model is a sensible default choice for the
	// tasks kapi drives — translation, review, terminology, entity extraction. Absent
	// means yes, which is the common case, so only the exceptions carry the field.
	//
	// It is set false for a model that is fully supported but not what most users
	// should reach for: a capable-but-premium model (Opus, Gemini Pro, o3) that is
	// overkill and dear for faithful content work, or an off-task one (Fable, tuned
	// for creative writing, where fidelity is the wrong instinct). A non-recommended
	// model is not hidden or blocked — it stays catalogued, keeps its ceilings, and
	// is callable by name; it simply sits under "Advanced" rather than in the primary
	// list. It does NOT overlap with Status: a superseded model is legacy regardless,
	// and this axis only sorts the *active* ones.
	Recommended *bool `json:"recommended,omitempty"`

	// MaxOutputTokens and ContextWindow are the model's hard ceilings. These fold in
	// what used to be the limits.go table; LimitsForModel now reads them from here.
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
	ContextWindow   int `json:"context_window,omitempty"`

	// Status is the lifecycle stage.
	Status ModelStatus `json:"status"`
	// Introduced is the date this model id entered neokapi's source tree
	// (providers/), YYYY-MM-DD. It is a statement about *our* support, not the
	// vendor's launch: a model that replaced an earlier alias is dated to that
	// change. Models present when the catalog was first seeded share that date.
	Introduced string `json:"introduced,omitempty"`
	// SupersededBy is the id of the model that replaced this one, when Status is
	// superseded.
	SupersededBy string `json:"superseded_by,omitempty"`
	// RetirementDate is a *scheduled future* retirement, YYYY-MM-DD, when the
	// vendor has announced one — a warning on a model still being served. Once the
	// model is actually gone the entry is removed, not kept with a past date.
	RetirementDate string `json:"retirement_date,omitempty"`
	// Note is any additional context — a caveat, a pricing condition, a reason.
	Note string `json:"note,omitempty"`
}

type modelCatalog struct {
	// AsOf dates the catalog as a whole.
	AsOf string `json:"as_of"`
	// Note is a maintenance banner.
	Note string `json:"note"`
	// Models is the curated support matrix.
	Models []Model `json:"models"`
}

var catalog = loadCatalog()

func loadCatalog() modelCatalog {
	var c modelCatalog
	if err := json.Unmarshal(modelsJSON, &c); err != nil {
		panic(fmt.Sprintf("aiprovider: models.json: %v", err))
	}
	return c
}

// Models returns the curated catalog, in file order.
func Models() []Model { return append([]Model(nil), catalog.Models...) }

// IsRecommended reports whether the model belongs in the primary user-facing list.
// Absent (the common case) means recommended; only demoted models say otherwise.
func (m Model) IsRecommended() bool { return m.Recommended == nil || *m.Recommended }

// CatalogAsOf reports the date the catalog was last reviewed.
func CatalogAsOf() string { return catalog.AsOf }

// ModelForID resolves a concrete model id to its catalog entry by longest-prefix
// match — the same matching the limits table used, so a dated or vendor-suffixed
// id lands on its family. ok is false for a model this build has never heard of.
func ModelForID(id string) (Model, bool) {
	m := strings.ToLower(strings.TrimSpace(id))
	if m == "" {
		return Model{}, false
	}
	best := -1
	for i, entry := range catalog.Models {
		p := strings.ToLower(entry.ID)
		if strings.HasPrefix(m, p) && (best < 0 || len(p) > len(catalog.Models[best].ID)) {
			best = i
		}
	}
	if best < 0 {
		return Model{}, false
	}
	return catalog.Models[best], true
}

// ModelByAlias resolves a bare alias (or exact id) to its catalog entry — the path
// the claude-code subscription aliases (sonnet, opus, haiku) take. Exact matches
// only; unlike ModelForID it does not prefix-match, because an alias is a whole
// name, not the head of a longer id.
func ModelByAlias(name string) (Model, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return Model{}, false
	}
	for _, entry := range catalog.Models {
		if strings.EqualFold(entry.ID, n) {
			return entry, true
		}
		for _, a := range entry.Aliases {
			if strings.EqualFold(a, n) {
				return entry, true
			}
		}
	}
	return Model{}, false
}
