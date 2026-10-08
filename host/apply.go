package host

import (
	"encoding/json"
	"errors"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/format"
)

// changeKind discriminates a changeEntry: an asset change (a term, a content
// memory pair, a recipe field) or a code comment's rewrite.
type changeKind string

const (
	kindTerm    changeKind = "term"
	kindMemory  changeKind = "memory"
	kindRecipe  changeKind = "recipe"
	kindComment changeKind = "comment"
)

// changeEntry is one change on its way to the store or the file it changes:
// an asset operation of a change set (assetEntry), a rule a context operation
// lands (contextOpsSession.assetEntries), or a code comment a comment change
// set rewrites (applyCommentSet). Only the fields relevant to its Kind are
// set.
type changeEntry struct {
	Kind changeKind

	// comment: the file, the comment's id as check_file reports it (such as
	// func/Parse), and its new prose without comment markers.
	File string
	ID   string
	Text string

	// comment: the fingerprint of the comment's bytes when the change set was
	// checked; a comment whose bytes differ when it is rewritten is refused as
	// changed. Lines locates the comment.
	CommentSHA256 string
	Lines         *format.LineRange

	// Op is the asset operation: upsert for a term, add for a pair, set for a
	// recipe field.
	Op string

	// term
	Term     string
	Locale   string
	Status   string
	Replaces string
	// DoNotTranslate sets (true) or clears (false) the do-not-translate flag
	// on the term's concept; nil leaves it.
	DoNotTranslate *bool
	// Replacement is the wording to use instead of a discouraged term.
	Replacement string
	// Profile scopes a new term's concept to the profile a rule's evidence was
	// seen at. Only a kept or settled rule sets it.
	Profile string
	// AllProfiles clears the profile a term's concept is scoped to, so it
	// holds across the project. Only a rule a person widened to the project
	// sets it.
	AllProfiles bool
	// Coordinates scope a new term's concept to the point a rule's evidence
	// was seen at (terms.PropCoordinates), beside its profile. Only a kept or
	// settled rule sets them.
	Coordinates map[string]string
	// Rescope moves the concept a term joins to Profile and Coordinates, for a
	// rule a person widened: its concept was scoped to the narrower point the
	// rule held at, and now holds where the rule does.
	Rescope bool
	// Advisory, for a discouraged term, makes a use of it report without
	// failing a check.
	Advisory bool
	// Competitor records the term as a competitor's name.
	Competitor bool

	// memory
	Source       string
	Target       string
	SourceLocale string
	TargetLocale string

	// recipe
	Path  string
	Value json.RawMessage

	// Evidence is where the wording behind an asset change was seen, recorded
	// on the operation so the decision can be argued with later.
	Evidence []contextop.Evidence
}

// assetResult is the outcome of one asset entry.
type assetResult struct {
	Kind   changeKind `json:"kind"`
	Op     string     `json:"op,omitempty"`
	Target string     `json:"target,omitempty"`
	Status string     `json:"status"` // applied | skipped | preview | error
	Detail string     `json:"detail,omitempty"`
}

// retiredVoiceKind is the change kind that added a word rule to a voice
// profile. Word rules are terms, so such an entry is written as a term.
const retiredVoiceKind changeKind = "voice"

var errRetiredVoiceKind = errors.New(`apply: a "voice" entry added a word rule to a voice profile, and word rules are terms: ` +
	`write it as {"kind": "term", "term": ..., "replacement": ..., "status": "forbidden"}, with "advisory": true or "competitor": true as needed`)
