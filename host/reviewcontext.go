package host

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// The review model: what a reviewer is owed about one unit besides its two
// strings.
//
// The bar: a reviewer sees at least what the model was told. A translate
// prompt carries four things about a block beyond the block itself
// (core/ai/prompt.Context): the block's key, the blocks before and after it,
// and what the block said last time. Every one of them reaches the reviewer
// here, and TestReviewContextAnswersPromptContext holds it that way.
//
// Every layer is assembled from an answer that already exists: the
// by-location resolution `kapi context` serves, the ordered blocks the file was
// read as, the content memory's version chain, the registered checkers, and the
// project state store. No store is added; the only cache is
// ReviewPointResolver, which holds a file's point for the length of one
// queue.
//
// There is no decision chain. core/state keeps one record per (Scope, Unit,
// Variant) and Put overwrites it, so Provenance carries the decision in force
// and says nothing about the ones before it.

// DefaultReviewWindow is how many blocks either side of the unit the
// neighbourhood carries. It matches the translate tool's own default, so a
// reviewer reading the neighbourhood reads the neighbourhood the model read.
const DefaultReviewWindow = 2

// ReviewTermRuleLimit caps the term rules a point renders. The rules bearing on
// this unit's own wording lead the list, so a capped list still holds every
// rule the prompt would have scoped to the text; the point reports the total
// either way.
const ReviewTermRuleLimit = 25

// ReviewContext binds one review decision to the context it is made in.
type ReviewContext struct {
	// Point is where the content sits and what governs it there.
	Point ReviewPoint `json:"point"`
	// Neighbourhood is the unit's key and the blocks around it in document
	// order.
	Neighbourhood ReviewNeighbourhood `json:"neighbourhood"`
	// History is what this unit said before, and the wording the content memory
	// already holds for it.
	History ReviewHistory `json:"history"`
	// Judgement is what the checks and the AI pre-review found.
	Judgement ReviewJudgement `json:"judgement"`
	// Provenance is where the current target came from and who decided on it.
	Provenance ReviewProvenance `json:"provenance"`
}

// ReviewPoint is the coordinate the unit's file sits at, with the governance in
// force there: the same answer `kapi context <path>` and the `context://`
// resources serve, so a reviewer and a run are never told different things
// about one file.
type ReviewPoint struct {
	// Path is the SOURCE file, project-relative and slash-separated.
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
	Voice *ContextVoice `json:"voice,omitempty"`
	// TermRules are the constraints on wording in force here, in the shape
	// every governed tool takes them (`term_rules:`).
	TermRules []coreprofile.TermRule `json:"term_rules,omitempty"`
	// TermsTotal is how many rules the point binds in all, so a capped list
	// says what it is a part of.
	TermsTotal int `json:"terms_total"`
	// Profiles are the recipe's bounded governance profiles read against now:
	// which voice is in force, and until when.
	Profiles []ContextProfileHit `json:"profiles,omitempty"`
	// Notes carries the freshness and scope caveats the resolution produced, so
	// a thin point is never ambiguous between "nothing governs here" and
	// "nothing could be read".
	Notes []string `json:"notes,omitempty"`
}

// ReviewNeighbourhood is the unit in its document: its key, and the blocks
// either side of it in the order the file holds them.
//
// The blocks travel as Run sequences rather than as text. Flattening a run
// sequence into a string reads as "concatenate the text" and behaves as
// "delete every placeholder, every paired code, every plural", so a reader
// gets a sentence with the variables silently missing from it.
type ReviewNeighbourhood struct {
	// Key is the block's key or path: `app.settings.title` rather than `Save`.
	// The cheapest disambiguation signal there is, and the one the prompt always
	// carries.
	Key string `json:"key,omitempty"`
	// Before and After are the neighbouring blocks, nearest last in Before and
	// nearest first in After, so reading the three lists in order reads the
	// document in order.
	Before []ReviewNeighbour `json:"before,omitempty"`
	After  []ReviewNeighbour `json:"after,omitempty"`
	// Window is how many blocks either side were asked for. A list shorter than
	// the window means the document ended, rather than that blocks were
	// dropped.
	Window int `json:"window"`
}

// ReviewNeighbour is one neighbouring block.
type ReviewNeighbour struct {
	// Key is the neighbour's stable unit key, so a reviewer can address it.
	Key string `json:"key,omitempty"`
	// Source is the neighbour's source content. The prompt carries this half.
	Source []model.Run `json:"source"`
	// Target is what the locale under review says for the neighbour, absent
	// when nothing is translated there. The prompt never carries it; a reviewer
	// reading a paragraph in sequence does.
	Target []model.Run `json:"target,omitempty"`
}

// ReviewHistory is what has already been approved for this unit.
type ReviewHistory struct {
	// Prior is the last answer approved for this block, with the source it was
	// approved for. Either half alone is worse than neither: a target with no
	// source is an anchor with no explanation.
	Prior *ReviewPriorVersion `json:"prior,omitempty"`
	// Match is the content memory's best answer for this source, with its
	// wording. A percentage alone tells a reviewer that something close exists
	// and never what it says.
	Match *ReviewMemoryMatch `json:"match,omitempty"`
	// Unseeded reports a project whose committed context sources have never
	// been compiled into the store this history reads: a fresh clone, before
	// anything ran. The store answers, and answers empty, which a reviewer
	// cannot tell from a memory that genuinely holds nothing close. `kapi up`
	// compiles the sources; until it has, an empty Match means unread.
	Unseeded bool `json:"unseeded,omitempty"`
}

// ReviewPriorVersion is one block's previous source and the target approved for
// it, read from the content memory's version chain.
type ReviewPriorVersion struct {
	Source string `json:"source"`
	Target string `json:"target"`
	// ContextFingerprint is the governing context that answer was produced
	// under. A translate prompt withholds a prior version whose fingerprint no
	// longer matches; a reviewer is shown it together with what it was approved
	// under, because judging whether the rules moved is the reviewer's job.
	ContextFingerprint string `json:"context_fingerprint,omitempty"`
	// Governed reports that the fingerprint still matches the context the
	// decision was recorded under, which is the condition under which the
	// prompt would have carried this pair.
	Governed bool `json:"governed"`
}

// ReviewMemoryMatch is the content memory's best answer for this unit's source.
type ReviewMemoryMatch struct {
	// Score is the match percent (0-100).
	Score int `json:"score"`
	// Source is the wording the corpus holds for the source locale, and Target
	// the answer approved for it.
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
}

// ReviewJudgement is what has already been said about this translation.
type ReviewJudgement struct {
	// Findings are the registered checkers' results for this unit, each with
	// the run range it applies to, so a surface can point at the text rather
	// than describe it.
	Findings []check.Finding `json:"findings,omitempty"`
	// AIScore, AIModel and AIFindings are the fresh AI pre-review annotation
	// from the state store. They repeat what ReviewUnitInfo carries flat, so a
	// client rendering judgement reads one group rather than two places.
	AIScore    *int                    `json:"ai_score,omitempty"`
	AIModel    string                  `json:"ai_model,omitempty"`
	AIFindings []state.AIReviewFinding `json:"ai_findings,omitempty"`
}

// ReviewProvenance is where the current target came from and who decided on it.
type ReviewProvenance struct {
	// Origin is the target's provenance when the state store or the format
	// records one.
	Origin *model.Origin `json:"origin,omitempty"`
	// ReviewState, By, At and Note are the decision in force. There is one per
	// (scope, unit, variant) and a new decision overwrites it, so this is the
	// current decision rather than the most recent of several.
	ReviewState string `json:"review_state,omitempty"`
	By          string `json:"by,omitempty"`
	At          string `json:"at,omitempty"`
	Note        string `json:"note,omitempty"`
	// Stale reports a decision recorded against source wording that has since
	// changed: the reviewer blessed a rendering of a sentence the project no
	// longer has.
	Stale bool `json:"stale,omitempty"`
}

// ReviewContextRequest is one unit's assembly order. The caller supplies what
// it has already read (the file's blocks, the state record, a content-memory
// handle), so assembling the model re-reads nothing.
type ReviewContextRequest struct {
	// Cmd carries the run context and the project binding the point and the
	// term rules resolve through.
	Cmd Command
	// Root is the project root.
	Root string
	// SourcePath is the unit's SOURCE file. The point is a property of where
	// the content lives, not of where one locale's rendering is written.
	SourcePath string
	// Collection is the content collection claiming the file.
	Collection string
	// Locale is the target locale under review, SourceLang the project's source
	// language.
	Locale     string
	SourceLang string
	// Blocks are the file's blocks in DOCUMENT ORDER with the locale's targets
	// already overlaid. The order is the whole point: a map keyed by unit key
	// answers "which block is this" and cannot answer "what comes next".
	Blocks []*model.Block
	// Key addresses the unit under review within Blocks.
	Key string
	// Window is how many blocks either side to carry; zero or less means
	// DefaultReviewWindow.
	Window int
	// Memory is the project's content memory when the caller holds one. Absent,
	// history reports no prior version and no match rather than opening a store
	// of its own.
	Memory memory.ContentMemory
	// Unit is the state record for this unit when the caller has already read
	// it. Absent, provenance is assembled from the block alone.
	Unit *state.UnitState
	// Points is a resolver shared across a queue, so a file's point is resolved
	// once however many of its units are reviewed. Absent, one is made for this
	// call.
	Points *ReviewPointResolver
}

// ReviewPointResolver answers "what governs this file" once per file.
//
// A file's units share a point, and resolving one opens the recipe, the voice
// store and the terms store, so a queue resolving per unit pays that for every
// row of a file it already has the answer for.
type ReviewPointResolver struct {
	app  *App
	cmd  Command
	root string

	mu    sync.Mutex
	cache map[string]reviewPointEntry
}

// reviewPointEntry is a resolved point plus the two materials a check needs
// from it. The voice profile and the vocabulary are the objects the checkers
// take, while ReviewPoint carries the rendering a client reads.
type reviewPointEntry struct {
	point ReviewPoint
	voice *coreprofile.VoiceProfile
	terms terms.Terminology
}

// NewReviewPointResolver returns a resolver bound to one project.
func (a *App) NewReviewPointResolver(cmd Command, root string) *ReviewPointResolver {
	return &ReviewPointResolver{app: a, cmd: cmd, root: root, cache: map[string]reviewPointEntry{}}
}

// At answers what governs a source file for one target locale. Repeat calls for
// the same (collection, file, locale) return the cached answer.
func (r *ReviewPointResolver) At(ctx context.Context, collection, sourcePath, locale string) ReviewPoint {
	return r.entryAt(ctx, collection, sourcePath, locale).point
}

// entryAt is At with the checker materials attached.
func (r *ReviewPointResolver) entryAt(ctx context.Context, collection, sourcePath, locale string) reviewPointEntry {
	if r == nil {
		return reviewPointEntry{}
	}
	rel := reviewPointPath(r.root, sourcePath)
	key := collection + "\x00" + rel + "\x00" + locale
	r.mu.Lock()
	if e, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return e
	}
	r.mu.Unlock()

	e := r.resolve(ctx, collection, rel, locale)

	r.mu.Lock()
	r.cache[key] = e
	r.mu.Unlock()
	return e
}

// resolve does the work entryAt caches. Each material degrades to a note rather
// than an error: a reviewer mid-decision is better served by four layers than
// by a refusal.
func (r *ReviewPointResolver) resolve(ctx context.Context, collection, rel, locale string) reviewPointEntry {
	e := reviewPointEntry{point: ReviewPoint{Path: rel, Collection: collection}}
	if r.app == nil || r.cmd == nil {
		return e
	}

	if rel != "" {
		abs := filepath.Join(r.root, filepath.FromSlash(rel))
		req := ContextPointRequest{Path: abs, Locale: model.LocaleID(locale)}

		src, cleanup := r.app.ContextSourcesAt(r.cmd, req)
		defer cleanup()
		e.voice = src.Voice
		if answer, err := ResolveContextAt(ctx, src, req); err == nil {
			e.point.Profile = answer.Point.Profile
			e.point.Channel = answer.Point.Channel
			e.point.Ref = answer.Point.Ref
			e.point.Default = answer.Point.Default
			e.point.Voice = answer.Voice
			e.point.Profiles = answer.Profiles
			e.point.Notes = answer.Notes
			if answer.Point.Collection != "" {
				e.point.Collection = answer.Point.Collection
			}
		} else {
			e.point.Notes = append(e.point.Notes, err.Error())
		}

		if tb, terr := r.app.ProjectTermsForFile(ctx, r.cmd, abs); terr != nil {
			e.point.Notes = append(e.point.Notes, "the vocabulary bound here could not be read: "+terr.Error())
		} else {
			e.terms = tb
		}
	}

	// The coordinates the recipe declares: the project's defaults, overlaid
	// with what the resolved ref derives, overlaid with the collection's own.
	if proj := r.recipe(); proj != nil {
		ref := project.ChannelRef{Profile: e.point.Profile, Channel: e.point.Channel}
		e.point.Coordinates = project.MergeCoordinates(
			proj.Defaults.Coordinates, ref.Coordinates(), collectionCoordinates(proj, e.point.Collection))
	}

	rules, err := r.app.ResolveTermRulesFor(r.cmd, locale, r.app.GovernancePointFor(collection, rel))
	if err != nil {
		e.point.Notes = append(e.point.Notes, "the terms bound here could not be read: "+err.Error())
	} else {
		e.point.TermRules, e.point.TermsTotal = rules, len(rules)
	}
	return e
}

// recipe reads the project the resolver is bound to, or nil when there is none.
func (r *ReviewPointResolver) recipe() *project.KapiProject {
	path, err := ResolveProjectPath(r.cmd)
	if err != nil || path == "" {
		return nil
	}
	proj, lerr := project.LoadWithOptions(path, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return nil
	}
	return proj
}

// collectionCoordinates reads a collection's own declared axes, nil when the
// recipe declares no such collection.
func collectionCoordinates(proj *project.KapiProject, name string) map[string]string {
	if proj == nil || name == "" {
		return nil
	}
	for i := range proj.Collections {
		if proj.Collections[i].Name == name {
			return proj.Collections[i].Coordinates
		}
	}
	return nil
}

// reviewPointPath renders a source path the way the recipe's globs are matched
// against it.
func reviewPointPath(root, sourcePath string) string {
	if sourcePath == "" {
		return ""
	}
	if !filepath.IsAbs(sourcePath) {
		return filepath.ToSlash(sourcePath)
	}
	if rel, err := filepath.Rel(root, sourcePath); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(sourcePath)
}

// AssembleReviewContext builds one unit's review model from what the caller has
// already read. It is the single assembler behind every review client, so the
// desktop, an MCP agent and the CLI cannot be shown different context for the
// same decision.
func (a *App) AssembleReviewContext(ctx context.Context, req ReviewContextRequest) *ReviewContext {
	idx := indexOfBlockKey(req.Blocks, req.Key)
	if idx < 0 {
		return nil
	}
	// Term rules resolve for a source/target pair, and the App is long-lived in
	// the desktop and the MCP server: without this, a second project reads the
	// first project's source language.
	if req.SourceLang != "" {
		defer a.scopeSourceLang()()
		a.SourceLang = ResolveSourceLocale(req.SourceLang, "")
	}
	block := req.Blocks[idx]
	loc := model.LocaleID(req.Locale)

	points := req.Points
	if points == nil {
		points = a.NewReviewPointResolver(req.Cmd, req.Root)
	}
	entry := points.entryAt(ctx, req.Collection, req.SourcePath, req.Locale)
	entry.point.TermRules = leadWithScopedRules(entry.point.TermRules, block.SourceText())
	entry.point.Language = req.Locale
	entry.point.IsSource = req.Locale != "" && req.Locale == req.SourceLang

	return &ReviewContext{
		Point:         entry.point,
		Neighbourhood: reviewNeighbourhood(req.Blocks, idx, req.Window, loc),
		History:       a.reviewHistory(ctx, req, block, loc),
		Judgement:     reviewJudgement(ctx, entry, block, req.SourceLang, loc, req.Unit),
		Provenance:    reviewProvenance(block, loc, req.Unit),
	}
}

// indexOfBlockKey finds a unit's position among the file's blocks.
func indexOfBlockKey(blocks []*model.Block, key string) int {
	for i, b := range blocks {
		if b != nil && b.Translatable && blockKey(b) == key {
			return i
		}
	}
	return -1
}

// reviewNeighbourhood collects the translatable blocks either side of idx, in
// document order.
func reviewNeighbourhood(blocks []*model.Block, idx, window int, loc model.LocaleID) ReviewNeighbourhood {
	if window <= 0 {
		window = DefaultReviewWindow
	}
	out := ReviewNeighbourhood{Key: promptKey(blocks[idx]), Window: window}
	for i := idx - 1; i >= 0 && len(out.Before) < window; i-- {
		if n, ok := reviewNeighbour(blocks[i], loc); ok {
			out.Before = append([]ReviewNeighbour{n}, out.Before...)
		}
	}
	for i := idx + 1; i < len(blocks) && len(out.After) < window; i++ {
		if n, ok := reviewNeighbour(blocks[i], loc); ok {
			out.After = append(out.After, n)
		}
	}
	return out
}

// reviewNeighbour projects one block into the neighbourhood, or reports that it
// carries nothing a reader would see.
func reviewNeighbour(b *model.Block, loc model.LocaleID) (ReviewNeighbour, bool) {
	if b == nil || !b.Translatable {
		return ReviewNeighbour{}, false
	}
	src := b.SourceRuns()
	if len(src) == 0 {
		return ReviewNeighbour{}, false
	}
	n := ReviewNeighbour{Key: blockKey(b), Source: src}
	if t := b.Target(loc); t != nil {
		n.Target = t.Runs
	}
	return n, true
}

// promptKey is the key a translate prompt sends for a block: the reader's name
// for it, which is what makes a bare "Save" mean something.
func promptKey(b *model.Block) string {
	if b == nil {
		return ""
	}
	if name := strings.TrimSpace(b.Name); name != "" {
		return name
	}
	return blockKey(b)
}

// leadWithScopedRules puts the rules bearing on this unit's own wording at the
// front and caps the list, so a truncated list still holds every rule the
// prompt would have scoped to the text.
func leadWithScopedRules(rules []coreprofile.TermRule, sourceText string) []coreprofile.TermRule {
	if len(rules) <= ReviewTermRuleLimit {
		return rules
	}
	scoped := coreprofile.ScopeTermRules(rules, sourceText)
	lead := make(map[string]bool, len(scoped))
	for _, r := range scoped {
		lead[r.Term] = true
	}
	out := make([]coreprofile.TermRule, 0, ReviewTermRuleLimit)
	out = append(out, scoped...)
	for _, r := range rules {
		if len(out) >= ReviewTermRuleLimit {
			break
		}
		if !lead[r.Term] {
			out = append(out, r)
		}
	}
	if len(out) > ReviewTermRuleLimit {
		out = out[:ReviewTermRuleLimit]
	}
	return out
}

// reviewHistory reads what has already been approved for this unit: its prior
// version from the content memory's version chain, and the corpus's best match
// with the wording it holds.
func (a *App) reviewHistory(ctx context.Context, req ReviewContextRequest, b *model.Block, loc model.LocaleID) ReviewHistory {
	var h ReviewHistory
	if req.Root != "" {
		h.Unseeded = a.ContextSourcesUnseeded(ctx, req.Root)
	}
	if req.Memory == nil {
		return h
	}
	source := model.LocaleID(ResolveSourceLocale(req.SourceLang, ""))

	if vr, versioned := req.Memory.(memory.VersionReader); versioned {
		h.Prior = reviewPriorVersion(ctx, vr, b, source, loc, req.Unit)
	}

	lookup := &model.Block{ID: "review-lookup", Translatable: true, Source: b.SourceRuns()}
	matches, err := req.Memory.Lookup(ctx, lookup, source, loc, memory.LookupOptions{MinScore: 0.5, MaxResults: 1})
	if err == nil && len(matches) > 0 {
		if target := matches[0].Entry.VariantText(loc); target != "" {
			h.Match = &ReviewMemoryMatch{
				Score:  int(matches[0].Score*100 + 0.5),
				Source: matches[0].Entry.VariantText(source),
				Target: target,
			}
		}
	}
	return h
}

// reviewPriorVersion reads the answer immediately before this one from the
// version chain, keyed on the block's chain identity (never its key: a chain
// links successive answers across edits, and an edit moves the key).
//
// Ungated, unlike the prompt's. A prompt withholds a prior version approved
// under superseded rules because it would anchor the model to wording those
// rules now reject, while a reviewer reading history is the party who judges
// whether the rules moved. Governed says which it is.
func reviewPriorVersion(
	ctx context.Context,
	vr memory.VersionReader,
	b *model.Block,
	source, target model.LocaleID,
	unit *state.UnitState,
) *ReviewPriorVersion {
	chain := b.ChainUnit()
	if chain == "" {
		return nil
	}
	versions, err := vr.Versions(ctx, memory.VersionQuery{Unit: chain, Limit: 1}, "")
	if err != nil || len(versions) == 0 {
		return nil
	}
	v := versions[0]
	prior := &ReviewPriorVersion{
		Source:             v.Entry.VariantText(source),
		Target:             v.Entry.VariantText(target),
		ContextFingerprint: v.ContextFingerprint,
	}
	if prior.Source == "" || prior.Target == "" {
		return nil
	}
	if unit != nil {
		prior.Governed = v.GovernedBy(unit.ContextHash)
	}
	return prior
}

// reviewJudgement runs the checkers a translated unit is held to and folds in
// the AI pre-review annotation the state store already holds: the voice
// vocabulary on the source where a voice is bound, placeholder integrity
// between the two sides, and do-not-translate over the terms the point marks.
//
// A checker that cannot RUN becomes a major "checks did not complete" finding
// rather than being dropped. A review surface showing a clean unit because a
// checker errored is wrong in the one direction that matters.
func reviewJudgement(
	ctx context.Context,
	entry reviewPointEntry,
	b *model.Block,
	sourceLang string,
	loc model.LocaleID,
	unit *state.UnitState,
) ReviewJudgement {
	j := ReviewJudgement{}
	fail := func(what string, err error) {
		j.Findings = append(j.Findings, check.Finding{
			Category: "check",
			Severity: check.SeverityMajor,
			Message:  fmt.Sprintf("checks did not complete: %s: %v", what, err),
			Check:    what,
		})
	}

	if entry.voice != nil {
		// InSourceLocale, like the Checks panel and the desktop's own review
		// checkset: the vocabulary lookup asks in the source language for a
		// block carrying no locale of its own, which is most of them. The terms
		// store is keyed by language, so without it a rule resolved from the
		// store matches here and not there.
		vocab := coretools.NewVoiceVocabCheckTool(entry.voice, entry.terms).
			InSourceLocale(model.LocaleID(ResolveSourceLocale(sourceLang, "")))
		if err := RunCheckTool(ctx, vocab, b); err != nil {
			fail("voice-vocab-check", err)
		} else if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](b, "voice"); ok {
			j.Findings = append(j.Findings, ann.Findings...)
			b.DelAnno("voice")
		}
	}

	placeholder := coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig(loc))
	if err := RunCheckTool(ctx, placeholder, b); err != nil {
		fail("placeholder-check", err)
	} else {
		j.Findings = append(j.Findings, FindingsFromBlock(b, true)...)
	}

	if dnt := doNotTranslateFromRules(entry.point.TermRules); len(dnt) > 0 {
		cfg := coretools.NewDNTCheckConfig(loc)
		cfg.Terms = dnt
		if err := RunCheckTool(ctx, coretools.NewDNTCheckTool(cfg), b); err != nil {
			fail("dnt-check", err)
		} else {
			j.Findings = append(j.Findings, FindingsFromBlock(b, true)...)
		}
	}

	if unit != nil && unit.AIReview != nil {
		score := unit.AIReview.Score
		j.AIScore = &score
		j.AIModel = unit.AIReview.Model
		j.AIFindings = unit.AIReview.Findings
	}
	return j
}

// doNotTranslateFromRules names the terms the point says must survive verbatim:
// a rule the terms store marks do-not-translate, and a rule whose required
// rendering is the term itself.
func doNotTranslateFromRules(rules []coreprofile.TermRule) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range rules {
		if r.Term == "" || seen[r.Term] {
			continue
		}
		if r.DoNotTranslate || r.Term == r.Replacement {
			seen[r.Term] = true
			out = append(out, r.Term)
		}
	}
	return out
}

// reviewProvenance reads where the current target came from and who decided on
// it.
func reviewProvenance(b *model.Block, loc model.LocaleID, unit *state.UnitState) ReviewProvenance {
	var p ReviewProvenance
	if unit != nil {
		if unit.Origin.Kind != "" {
			o := unit.Origin
			p.Origin = &o
		}
		p.ReviewState = unit.Decision.ReviewState
		p.By = unit.Decision.By
		p.At = unit.Decision.At
		p.Note = unit.Decision.Note
	}
	// The format's own provenance wins: it describes the bytes on disk, while
	// the state record describes what was last written through kapi.
	if t := b.Target(loc); t != nil && t.Origin.Kind != "" {
		o := t.Origin
		p.Origin = &o
	}
	return p
}

// ReviewMemory opens the project's content memory for a review read, or returns
// nil when the project holds none. The handle belongs to the project's shared
// pool, whose schemas also carry the terms store and the block cache, so a
// caller must not close it.
func (a *App) ReviewMemory(ctx context.Context, root string) memory.ContentMemory {
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return nil
	}
	tm := db.Memory()
	if tm == nil {
		return nil
	}
	return tm
}
