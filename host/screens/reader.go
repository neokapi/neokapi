package screens

import "github.com/neokapi/neokapi/core/model"

// ReaderFileDTO is one content file as the Reader draws it.
type ReaderFileDTO struct {
	// Path is the file, project-relative and slash-separated; Abs is the same
	// file on disk.
	Path string `json:"path"`
	Abs  string `json:"abs"`
	// Format is the format the recipe reads the file as.
	Format string `json:"format,omitempty"`
	// Blocks are the file's source blocks in file order, each with the
	// findings on it.
	Blocks []ReaderBlockDTO `json:"blocks"`
	// Document holds the findings that name no block of the file.
	Document []ReaderFindingDTO `json:"document"`
	// Findings counts every finding the check reports for the file, and
	// Fixable those with a fix.
	Findings int `json:"findings"`
	Fixable  int `json:"fixable"`
	// Verdict is the checks run's: "passed", "failed" or "did_not_run". A run
	// that did not run found nothing because it checked nothing.
	Verdict string `json:"verdict"`
	// Rules are what the context answer for the file holds, in the order
	// `kapi context <path>` prints them. RulesTotal counts every rule the
	// answer holds when the list is capped.
	Rules      []ReaderRuleDTO `json:"rules"`
	RulesTotal int             `json:"rules_total"`
	// Attention is what the context answer says to act on before relying on
	// it, such as a binding that failed to load.
	Attention []string `json:"attention"`
	// Refused names the fixes the change service refused in the last fix,
	// each with the service's reason. A refused fix changed nothing.
	Refused []ReaderRefusalDTO `json:"refused"`
}

// ReaderBlockDTO is one block of the file.
type ReaderBlockDTO struct {
	// ID is the block's id as the check names it.
	ID string `json:"id"`
	// Runs are the block's source runs, which the Reader projects for display.
	Runs     []model.Run        `json:"runs"`
	Findings []ReaderFindingDTO `json:"findings"`
}

// ReaderFindingDTO is one finding, placed on its block.
type ReaderFindingDTO struct {
	// ID names the finding for FixFinding. It is the same in every read of
	// the file while the finding stands.
	ID      string `json:"id"`
	BlockID string `json:"block_id,omitempty"`
	// Position is the run range the finding is about, over the block's
	// source runs; Span is that range in code points of the block's own text,
	// in which an inline code has no width. Span is absent for a finding the
	// check located at the block alone or inside a plural.
	Position model.Anchor `json:"position,omitzero"`
	Span     *ReaderSpan  `json:"span,omitempty"`
	// Kind is the mark the finding carries: "name" for a term rule about a
	// spelling, "word" for any other term rule, "tone" for a voice rule, and
	// otherwise the check that raised it (placeholder, dnt, …).
	Kind string `json:"kind"`
	// Label is the line under the block: the wording a term rule asks for and
	// the word it avoids, or the check's message.
	Label string `json:"label"`
	// Fails says whether the finding fails the check; Suggested marks one a
	// rule nobody has established raised, which never fails.
	Fails     bool `json:"fails"`
	Suggested bool `json:"suggested,omitempty"`
	// Fixable says the finding carries a fix.
	Fixable bool `json:"fixable"`
	// Locale is the language of the translation a target-side finding is
	// about; empty for a finding on the source.
	Locale string `json:"locale,omitempty"`
}

// ReaderSpan is a half-open code-point range [Start, End).
type ReaderSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ReaderRuleDTO is one rule that holds at the file.
type ReaderRuleDTO struct {
	// Kind is the mark: "tone" for the voice, "name" or "word" for a term
	// rule, "keep" for a wording a rule elsewhere leaves correct here, "note"
	// for a note.
	Kind string `json:"kind"`
	// Rule is the rule as one line.
	Rule string `json:"rule"`
	// Status is "established", "suggested" or "contested".
	Status string `json:"status"`
	// From is where the rule is held: terms, a pack, the workspace, or the
	// voice profile.
	From string `json:"from,omitempty"`
	// By is who recorded the rule and At when, RFC 3339, when the answer
	// carries them.
	By string `json:"by,omitempty"`
	At string `json:"at,omitempty"`
}

// ReaderRefusalDTO is one fix the change service refused.
type ReaderRefusalDTO struct {
	FindingID string `json:"finding_id"`
	Reason    string `json:"reason"`
}
