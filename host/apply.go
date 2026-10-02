package host

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/format"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// changeKind discriminates a change-set entry. `apply` is the single write verb:
// every deliberate, reviewed change Claude proposes — a content edit or an asset
// edit (term, content memory pair, recipe field) — is one typed entry,
// so "is this change reviewed?" has one answer for everything and the backing
// stores are written by exactly one code path.
type changeKind string

const (
	kindContent changeKind = "content"
	kindTerm    changeKind = "term"
	kindMemory  changeKind = "memory"
	kindRecipe  changeKind = "recipe"
	kindReview  changeKind = "review"
	kindComment changeKind = "comment"
)

// changeEntry is one line of a `kapi apply` change-set (JSONL; one entry per
// line). Only the fields relevant to its Kind are populated. Content edits carry
// the block address (file + id + content_hash) and the new placeholder-rendered
// text; asset edits carry an op and the per-asset fields. Comment edits carry
// the file, the comment's id, the fingerprint or prose it was read with and
// its new prose.
type changeEntry struct {
	Kind changeKind `json:"kind" jsonschema:"change kind; use content for document wording and comment for a code comment"`

	// content and comment
	File        string `json:"file,omitempty"`
	ID          string `json:"id,omitempty" jsonschema:"the block id; for kind=comment the comment's id as check_file reports it, such as func/Parse"`
	ContentHash string `json:"content_hash,omitempty"`
	Text        string `json:"text,omitempty" jsonschema:"the new wording: for kind=content the block text with the inline placeholders kapi inspect shows; for kind=comment the comment's prose without comment markers"`

	// comment
	CommentSHA256 string            `json:"comment_sha256,omitempty" jsonschema:"for kind=comment: the comment_sha256 check_file reports for the comment; an edit to a comment whose bytes differ is refused as changed"`
	CurrentText   *string           `json:"current_text,omitempty" jsonschema:"for kind=comment: the comment's prose as you read it, without comment markers; guards the edit when comment_sha256 is not given"`
	Lines         *format.LineRange `json:"lines,omitempty" jsonschema:"for kind=comment: the lines check_file reported for the comment"`
	Width         int               `json:"width,omitempty" jsonschema:"for kind=comment: the column a line of prose wraps at; 0 keeps the width of the comment being rewritten, and never less than 80. The languages a comment plugin reads keep the comment's own line breaks, so width does not apply to them"`

	// asset common
	Op string `json:"op,omitempty"`

	// term
	Term     string `json:"term,omitempty"`
	Locale   string `json:"locale,omitempty"`
	Status   string `json:"status,omitempty"`
	Replaces string `json:"replaces,omitempty"`
	// DoNotTranslate, for kind=term, sets (true) or clears (false) the
	// do-not-translate flag on the term's concept; omitted leaves it.
	DoNotTranslate *bool `json:"do_not_translate,omitempty" jsonschema:"for kind=term: true keeps the term verbatim in every language, false clears that, omitted leaves it"`

	// tm
	Source       string `json:"source,omitempty"`
	Target       string `json:"target,omitempty"`
	SourceLocale string `json:"source_locale,omitempty"`
	TargetLocale string `json:"target_locale,omitempty"`

	// Replacement, for kind=term, is the wording to use instead of a
	// discouraged term; content entries use text.
	Replacement string `json:"replacement,omitempty" jsonschema:"for kind=term: the wording to use instead of a discouraged term; content entries use text"`
	// Profile scopes a new term's concept to the profile a rule's evidence was
	// seen at. Only a kept or settled rule sets it; an apply never does.
	Profile string `json:"-"`
	// AllProfiles clears the profile a term's concept is scoped to, so it
	// holds across the project. Only a rule a person widened to the project
	// sets it; an apply never does.
	AllProfiles bool `json:"-"`
	// Advisory, for a discouraged term, makes a use of it report without
	// failing a check.
	Advisory bool `json:"advisory,omitempty" jsonschema:"for kind=term with a discouraged status: a use reports without failing a check"`
	// Competitor, for kind=term, records the term as a competitor's name.
	Competitor bool `json:"competitor,omitempty" jsonschema:"for kind=term: the term is a competitor's name"`

	// recipe
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`

	// Evidence is where the wording behind an asset entry was seen, recorded on
	// the operation so the decision can be argued with later.
	Evidence []contextop.Evidence `json:"evidence,omitempty" jsonschema:"for asset entries: where the wording behind this decision was seen"`
}

// assetResult is the outcome of one asset entry, surfaced in the ApplyReport.
type assetResult struct {
	Kind   changeKind `json:"kind"`
	Op     string     `json:"op,omitempty"`
	Target string     `json:"target,omitempty"`
	Status string     `json:"status"` // applied | skipped | preview | error
	Detail string     `json:"detail,omitempty"`
}

// validateChangeSet refuses a change-set whose content or comment entries are
// malformed or contradict each other, before either surface applies any of it.
// A content entry names its file and its block, by id or, without one, by
// content_hash: an entry naming no block would match nothing and change
// nothing. Two content entries for one block that ask for different things
// would leave only one of them applied, so they are refused too; two identical
// entries are one edit.
func validateChangeSet(entries []changeEntry) error {
	type blockRef struct{ file, field, value string }
	firstFor := map[blockRef]int{}
	for i, e := range entries {
		if e.Kind == kindContent {
			switch {
			case e.Replacement != "":
				return fmt.Errorf("content entry %d for block %q: put the new wording in \"text\"; \"replacement\" belongs to term entries", i+1, e.ID)
			case e.File == "":
				return fmt.Errorf("content entry %d for block %q has no \"file\"", i+1, e.ID)
			case e.ID == "" && e.ContentHash == "":
				return fmt.Errorf("content entry %d in %s names no block: give the block's \"id\" and \"content_hash\" as kapi inspect prints them", i+1, e.File)
			}
			ref := blockRef{file: e.File, field: "id", value: e.ID}
			if e.ID == "" {
				ref = blockRef{file: e.File, field: "content_hash", value: e.ContentHash}
			}
			if j, seen := firstFor[ref]; seen {
				if prev := entries[j]; prev.Text != e.Text || prev.ContentHash != e.ContentHash {
					return fmt.Errorf("content entries %d and %d both edit the block with %s %q in %s, differently; send one entry per block", j+1, i+1, ref.field, ref.value, e.File)
				}
				continue
			}
			firstFor[ref] = i
			continue
		}
		if e.Kind != kindComment {
			continue
		}
		switch {
		case e.File == "":
			return fmt.Errorf("comment entry %d for %q has no \"file\"", i+1, e.ID)
		case e.ID == "":
			return fmt.Errorf("comment entry %d in %s has no \"id\"; use the comment's id as kapi check reports it", i+1, e.File)
		case e.Replacement != "":
			return fmt.Errorf("comment entry %d for %q: put the new prose in \"text\"; \"replacement\" belongs to term entries", i+1, e.ID)
		case e.ContentHash != "":
			return fmt.Errorf("comment entry %d for %q: a comment is guarded by the \"comment_sha256\" kapi check reports, not by \"content_hash\"", i+1, e.ID)
		case e.CommentSHA256 == "" && e.CurrentText == nil:
			return fmt.Errorf("comment entry %d for %q has no guard: pass the \"comment_sha256\" kapi check reports for the comment, or its prose as you read it in \"current_text\"", i+1, e.ID)
		}
	}
	return nil
}

// buildEditMaps splits content entries into an ID-keyed and a hash-keyed lookup
// for the apply-edits tool: entries with an ID resolve by ID, ID-less entries
// resolve by content_hash.
func buildEditMaps(entries []changeEntry) (byID, byHash map[string]coretools.Edit) {
	byID = map[string]coretools.Edit{}
	byHash = map[string]coretools.Edit{}
	for _, e := range entries {
		edit := coretools.Edit{Text: e.Text, ContentHash: e.ContentHash}
		if e.ID != "" {
			byID[e.ID] = edit
		} else if e.ContentHash != "" {
			byHash[e.ContentHash] = edit
		}
	}
	return byID, byHash
}

// retiredVoiceKind is the change kind that added a word rule to a voice
// profile. Word rules are terms, so such an entry is written as a term.
const retiredVoiceKind changeKind = "voice"

var errRetiredVoiceKind = errors.New(`apply: a "voice" entry added a word rule to a voice profile, and word rules are terms: ` +
	`write it as {"kind": "term", "term": ..., "replacement": ..., "status": "forbidden"}, with "advisory": true or "competitor": true as needed`)
