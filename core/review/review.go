// Package review defines the review model: what a reviewer is owed about one
// unit besides its two strings, as one wire shape every review client reads.
//
// A review decision is made at a point. The unit sits in a document, at a
// coordinate, governed by a voice profile and a vocabulary, beside the blocks
// that precede and follow it, after a version that was approved before it.
// Context binds the decision to that context in five layers, in the order a
// reviewer reads them: Point, Neighbourhood, History, Judgement, Provenance.
//
// Two hosts assemble it. The local host (`host.AssembleReviewContext`) serves
// it to Kapi Desktop, the CLI and the MCP tools; the platform's REST server
// serves it to its review surfaces with a few platform-only rows beside it.
// Both write the same fields in the same spelling and on the same scales, so a
// reader comparing the two surfaces reads the same facts for one unit. The
// TypeScript contract both frontends read is generated from these structs
// (`make generate-contract-types`), so a field added here reaches every client
// or fails to compile in the one that ignores it.
//
// The bar the shape is held to: a reviewer sees at least what the model was
// told. A translate prompt carries a block's key, the blocks either side of it
// and the prior approved version; each is a field here, and a reflection test
// in host holds the model to the prompt's struct.
package review

import (
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/state"
)

// DefaultWindow is how many blocks either side of the unit the neighbourhood
// carries. It matches the translate tool's own default, so a reviewer reading
// the neighbourhood reads the neighbourhood the model read.
const DefaultWindow = 2

// Context binds one review decision to the context it is made in.
type Context struct {
	// Point is where the content sits and what governs it there.
	Point Point `json:"point"`
	// Neighbourhood is the unit's key and the blocks around it in document
	// order.
	Neighbourhood Neighbourhood `json:"neighbourhood"`
	// History is what this unit said before, and the wording the content memory
	// already holds for it.
	History History `json:"history"`
	// Judgement is what the checks and the AI pre-review found.
	Judgement Judgement `json:"judgement"`
	// Provenance is where the current target came from and who decided on it.
	Provenance Provenance `json:"provenance"`
}

// Point is the coordinate the unit's file sits at, with the governance in force
// there: the same answer `kapi context <path>` and the `context://` resources
// serve, so a reviewer and a run are never told different things about one
// file.
type Point struct {
	// Path is the SOURCE file, project-relative and slash-separated. On the
	// platform it is the item the block belongs to.
	Path string `json:"path,omitempty"`
	// Language is the language the unit under review belongs to, which is what
	// the governance below was resolved for: a term rule is resolved per
	// language, so a point read for one language answers for that one.
	Language string `json:"language,omitempty"`
	// IsSource marks the language as the project's source language, so a client
	// knows it is looking at the author's own wording rather than a translation
	// of it.
	IsSource bool `json:"is_source,omitempty"`
	// Profile is the governance profile in force, empty at the project's
	// default point.
	Profile string `json:"profile,omitempty"`
	// Channel is the surface the content ships on, empty when it binds none.
	Channel string `json:"channel,omitempty"`
	// Collection is the content collection claiming the path.
	Collection string `json:"collection,omitempty"`
	// Ref renders Profile and Channel as the recipe writes the binding
	// (`profile/channel`).
	Ref string `json:"ref,omitempty"`
	// Default reports that resolution fell through to the project's default
	// point, which is a real place rather than an absent one.
	Default bool `json:"default"`
	// Coordinates place the point on the declared axes: product, channel, and
	// the brand a recipe states once under `defaults:`.
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Voice is the profile in force with the guidance it renders: the same
	// prose `kapi voice guide` prints and the translate prompt carries.
	Voice *Voice `json:"voice,omitempty"`
	// TermRules are the constraints on wording in force here, in the shape
	// every governed tool takes them (`term_rules:`). The rules bearing on this
	// unit's own wording lead the list.
	TermRules []profile.TermRule `json:"term_rules,omitempty"`
	// TermsTotal is how many rules the point binds in all, so a capped list
	// says what it is a part of.
	TermsTotal int `json:"terms_total"`
	// Profiles are the recipe's bounded governance profiles read against now:
	// which voice is in force, and until when.
	Profiles []ProfileValidity `json:"profiles,omitempty"`
	// Notes carries the freshness and scope caveats the resolution produced, so
	// a thin point is never ambiguous between "nothing governs here" and
	// "nothing could be read".
	Notes []string `json:"notes,omitempty"`
}

// Voice is the voice in force at a point, with the guidance it renders. It is
// the same row the by-location context answer carries, because it is the same
// fact.
type Voice struct {
	Name string `json:"name"`
	// Source is where the profile was loaded from: a path, `pack:<name>`, or
	// `store:<id>`.
	Source string `json:"source,omitempty"`
	// Field names the recipe key that bound it (`profiles.<name>.voice` or
	// `defaults.voice`), so a caller that wants to change the answer knows
	// which line to edit. Empty where no recipe bound it.
	Field string `json:"field,omitempty"`
	// Guide is the profile rendered as prose for a model: the same rendering
	// `kapi voice guide` prints and the translation prompt carries, so what a
	// reviewer reads here is what generation was held to.
	Guide string `json:"guide,omitempty"`
}

// ProfileValidity is a governance profile whose validity is bounded, reported
// so an answer can say which voice is in force and until when.
type ProfileValidity struct {
	Name string `json:"name"`
	// ValidFrom and ValidTo render the profile's window as it was authored: a
	// bare date when it is midnight, otherwise the full instant. Empty means
	// the bound is open on that side.
	ValidFrom string `json:"valid_from,omitempty"`
	ValidTo   string `json:"valid_to,omitempty"`
	// State is the window read against now: "active", "upcoming" (not yet in
	// force) or "expired".
	State string `json:"state"`
}

// Neighbourhood is the unit in its document: its key, and the blocks either
// side of it in the order the file holds them.
//
// The blocks travel as Run sequences rather than as text. Flattening a run
// sequence into a string reads as "concatenate the text" and behaves as
// "delete every placeholder, every paired code, every plural", so a reader
// gets a sentence with the variables silently missing from it.
type Neighbourhood struct {
	// Key is the block's key or path: `app.settings.title` rather than `Save`.
	// The cheapest disambiguation signal there is, and the one the prompt
	// always carries.
	Key string `json:"key,omitempty"`
	// Before and After are the neighbouring blocks, nearest last in Before and
	// nearest first in After, so reading the three lists in order reads the
	// document in order.
	Before []Neighbour `json:"before,omitempty"`
	After  []Neighbour `json:"after,omitempty"`
	// Window is how many blocks either side were asked for. A list shorter
	// than the window means the document ended, rather than that blocks were
	// dropped.
	Window int `json:"window"`
}

// Neighbour is one neighbouring block.
type Neighbour struct {
	// Key is the neighbour's stable unit key, so a reviewer can address it.
	Key string `json:"key,omitempty"`
	// Source is the neighbour's source content. The prompt carries this half.
	Source []model.Run `json:"source"`
	// Target is what the locale under review says for the neighbour, absent
	// when nothing is translated there. The prompt never carries it; a
	// reviewer reading a paragraph in sequence does.
	Target []model.Run `json:"target,omitempty"`
	// Status is the neighbour's rung on the target ladder in this locale,
	// empty when it has no target.
	Status string `json:"status,omitempty"`
}

// History is what has already been approved for this unit.
type History struct {
	// Prior is the last answer approved for this block, with the source it was
	// approved for. Either half alone is worse than neither: a target with no
	// source is an anchor with no explanation.
	Prior *PriorVersion `json:"prior,omitempty"`
	// Match is the content memory's best answer for this source, with its
	// wording. A percentage alone tells a reviewer that something close exists
	// and never what it says.
	Match *MemoryMatch `json:"match,omitempty"`
	// Unseeded reports a project whose committed context sources have never
	// been compiled into the store this history reads: a fresh clone, before
	// anything ran. The store answers, and answers empty, which a reviewer
	// cannot tell from a memory that genuinely holds nothing close. `kapi up`
	// compiles the sources; until it has, an empty Match means unread.
	Unseeded bool `json:"unseeded,omitempty"`
}

// PriorVersion is one block's previous source and the target approved for it,
// read from the content memory's version chain.
type PriorVersion struct {
	Source string `json:"source"`
	Target string `json:"target"`
	// ContextFingerprint is the governing context that answer was produced
	// under. A translate prompt withholds a prior version whose fingerprint no
	// longer matches; a reviewer is shown it together with what it was
	// approved under, because judging whether the rules moved is the
	// reviewer's job.
	ContextFingerprint string `json:"context_fingerprint,omitempty"`
	// Governed reports that the fingerprint still matches the context the
	// decision was recorded under, which is the condition under which the
	// prompt would have carried this pair.
	Governed bool `json:"governed"`
}

// MemoryMatch is the content memory's best answer for this unit's source.
type MemoryMatch struct {
	// Score is the match percent (0-100).
	Score int `json:"score"`
	// Kind is how the memory matched it: "exact", "fuzzy", or one of the
	// generalized and structural variants the memory reports.
	Kind string `json:"kind,omitempty"`
	// Source is the wording the corpus holds for the source locale, and Target
	// the answer approved for it.
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
}

// Judgement is what has already been said about this translation.
type Judgement struct {
	// Findings are the checkers' results for this unit, each with the run range
	// it applies to, so a surface can point at the text rather than describe
	// it.
	Findings []check.Finding `json:"findings,omitempty"`
	// AIScore, AIModel and AIFindings are the fresh AI pre-review annotation
	// from the state store, where the host keeps one.
	AIScore    *int                    `json:"ai_score,omitempty"`
	AIModel    string                  `json:"ai_model,omitempty"`
	AIFindings []state.AIReviewFinding `json:"ai_findings,omitempty"`
}

// Provenance is where the current target came from and who decided on it.
type Provenance struct {
	// Origin is the target's provenance when the state store or the format
	// records one.
	Origin *model.Origin `json:"origin,omitempty"`
	// ReviewState, By, At and Note are the decision in force. There is one per
	// (scope, unit, variant) and a new decision overwrites it, so this is the
	// current decision rather than the most recent of several.
	ReviewState string `json:"review_state,omitempty"`
	// Status is the ladder rung the decision in force landed the unit on: a
	// target rung for a translation, an authoring rung for source wording.
	Status string `json:"status,omitempty"`
	By     string `json:"by,omitempty"`
	At     string `json:"at,omitempty"`
	Note   string `json:"note,omitempty"`
	// Stale reports a decision recorded against source wording that has since
	// changed: the reviewer blessed a rendering of a sentence the project no
	// longer has.
	Stale bool `json:"stale,omitempty"`
}
