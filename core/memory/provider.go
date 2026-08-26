// Package memory is the contract between a producer and a content memory.
//
// The framework declares here what a tool may ask; a content memory implements
// it (memory/leverage), and the host decides which one a run gets. Nothing
// under core/ imports a store, so the arrow stays one-way.
//
// It replaces four interfaces that grew separately — MemoryProvider,
// BlockMemoryProvider and ExactMemoryProvider in core/tools, plus
// PriorVersionProvider in core/ai/tools — all of them implemented by one type
// and reached by two different injection routes. Three consequences of that
// split are worth naming, because they are what this package exists to prevent:
//
//   - Optional capability by TYPE ASSERTION. A tool asked whether its provider
//     implemented an interface and silently did less when it did not. A
//     capability that switches itself off is indistinguishable from a corpus
//     with nothing to say.
//   - Two vocabularies for one idea. LookupBlock called the location `at` and
//     took a whole block; PriorVersion called it `point` and took a unit. No
//     unified interface can be written until that is settled, which is why it
//     is settled here: Point and Unit, everywhere.
//   - Long parameter lists. Adding the point to the exact lookup, and the
//     fingerprint to the version lookup, each broke every implementation.
//     Requests are structs so a new field is additive.
package memory

import (
	"context"

	"github.com/neokapi/neokapi/core/edit"
	"github.com/neokapi/neokapi/core/model"
)

// ConfigKey is where a host puts a Provider in a tool's config map.
//
// A provider is a live handle on a store, so it cannot survive the JSON round
// trip the rest of a tool's config takes — the same reason a voice profile
// travels this way. One key, so a host grants a corpus the same way to every
// tool that accepts one, and a tool lifts it out the same way whoever built it.
const ConfigKey = "memory"

// SourceLocaleKey is where a host puts the run's source locale for a tool whose
// SourceLocale is schema-hidden.
//
// It travels beside the provider because it is useless without one: a corpus
// lookup filters on the source locale, and an empty one matches nothing. The
// target locale reaches a factory as its own argument; the source has no such
// channel, and every injection route was separately patching it onto the built
// config to compensate.
const SourceLocaleKey = "sourceLocale"

// Provider is everything a producer may ask a content memory.
//
// Two questions, not four methods. "What is approved for this content, here?"
// is one question whose answer varies by policy, and exact-versus-fuzzy is a
// policy rather than a separate question. "What did this block say before?" is
// genuinely different: it is keyed by the block's identity over time rather
// than by its text, and it is governed.
//
// Every method takes the run's context, because a lookup is I/O — a SQLite
// query, or a round trip for a provider that is not local — and cancelling a
// run has to reach it.
//
// A provider that cannot answer returns false. It does not omit the method.
// That is the whole difference from what came before: a store with no version
// chain says so on every call, rather than failing an interface check once and
// leaving the caller to guess whether the feature is off or the corpus is empty.
type Provider interface {
	// Lookup returns the approved content for a request, or false when the
	// corpus holds nothing that satisfies it.
	Lookup(ctx context.Context, req Request) (Match, bool)

	// PriorVersion returns what a block said before, or false when there is no
	// chain, no answer in it for these locales, or the answer there was
	// approved under governance that has since moved.
	//
	// The gate is inside the implementation, not in front of it. A caller that
	// must remember to compare a fingerprint is a caller that will forget, and
	// the failure is silent: a translation steered by wording approved under
	// rules that no longer apply, stamped with today's fingerprint and looking
	// fresh.
	PriorVersion(ctx context.Context, req VersionRequest) (Version, bool)
}

// Request asks what the corpus has approved for some content, at a point.
//
// Block and Text are the two ways to name the content, and they are not
// interchangeable. A block carries its inline codes, so it can match on
// structure and the answer's codes survive the fill. Text is the flattened
// form, which is what a legacy entry stored without codes can be found by; it
// is the fallback, and a caller uses it after the block form finds nothing.
type Request struct {
	// Block is the content to match, with its inline codes. Preferred.
	Block *model.Block
	// Text is the flattened content, for the entries a block form cannot
	// reach. Ignored when Block is set.
	Text string

	// Source and Target are the locales the answer must span.
	Source, Target model.LocaleID

	// Point is where the content sits: the product, channel and collection it
	// belongs to. A source the corpus answers more than one way resolves to the
	// approval nearest here, because an answer approved somewhere else is still
	// an answer from somewhere else.
	//
	// Empty means the caller has no location in hand, and the corpus may answer
	// from anywhere. That is a weaker request, not an error.
	Point string

	// MinScore is the floor, 0-100. 100 admits only an exact match.
	MinScore int

	// Verbatim restricts the answer to the same text, matched plainly: no
	// entity adaptation and no structural tier. It is what a fill asks for when
	// it wants a guarantee that one string does not render two ways, rather
	// than a best effort.
	Verbatim bool
}

// Match is what the corpus answered.
//
// The target is carried as runs rather than a string so inline codes — icons,
// paired markup, placeholders — survive into the fill instead of being
// flattened into literal token text.
type Match struct {
	// TargetRuns is the approved target, with any entity adaptations already
	// applied to the current content's values.
	TargetRuns []model.Run

	// Score is 0-100. 100 is a structurally exact match; a plain-text exact
	// whose inline codes differ from the request's is capped below it.
	//
	// It is reported because a surface shows it and a legacy path still gates
	// on it. It is not what a fill decision should read — see Edit.
	Score int

	// Exact reports whether the answer came from an exact tier rather than a
	// fuzzy one.
	Exact bool

	// Ambiguous marks an exact match the corpus could not resolve: several
	// entries matched at full score with different targets. An ambiguous match
	// is recorded as a candidate and never filled unattended, because picking
	// one is a decision and the corpus has no basis for making it.
	Ambiguous bool

	// Edit is how the request's content differs from the content this answer
	// was approved for.
	//
	// This is what Score was always standing in for, and what a fill decision
	// should read instead. A percentage softens with length: the same added
	// word is 95% in a sentence and 78% in a label, so no threshold ranks them
	// together. An edit kind does not have that problem.
	//
	// Empty when the answer carries no source to compare against, which is the
	// flattened-text path. A caller falls back to Score there, which is the
	// behaviour that existed before classification.
	Edit edit.Kind
}

// VersionRequest asks what a block said before.
type VersionRequest struct {
	// Unit is the block's identity across edits: what links its successive
	// approved answers into one chain. See model.Block.ChainUnit — never the
	// block ID, which is assigned per read.
	//
	// Required. An empty unit would select every entry approved before the
	// chain existed, which is never a useful answer.
	Unit string

	// Point is where the content sits, so a wording approved for one surface
	// does not steer another.
	Point string

	// Source and Target are the locales the answer must span. An answer missing
	// either is not returned: a target with no source it was approved for is an
	// anchor with no explanation.
	Source, Target model.LocaleID

	// GovernedBy is the governing context in force now — the fingerprint over
	// the voice guidance and term rules that are about to reach the producer.
	// An answer approved under anything else is withheld.
	//
	// Required, and deliberately so. An empty fingerprint cannot be asserted to
	// match, so an ungoverned run gets no reference rather than an unjudged one.
	GovernedBy string
}

// Version is one previously approved answer and the content it was approved for.
//
// Both halves, always. The target alone is an anchor a model can only copy;
// the pair is a diff it can reason about — what the content said, what was
// approved for it, and what it says now.
type Version struct {
	Source string
	Target string
}

// NullProvider answers nothing.
//
// It is the honest provider for a run with no content memory, and the default
// wherever one is optional. Every question gets a real answer — "no" — so a
// tool holding one behaves identically to a tool holding a corpus that happens
// to be empty, and neither has to be special-cased.
type NullProvider struct{}

// Lookup always returns no match.
func (NullProvider) Lookup(context.Context, Request) (Match, bool) { return Match{}, false }

// PriorVersion always returns no prior version.
func (NullProvider) PriorVersion(context.Context, VersionRequest) (Version, bool) {
	return Version{}, false
}

var _ Provider = NullProvider{}
