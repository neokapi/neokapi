package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/review"
	"github.com/neokapi/neokapi/terms"
)

// Context retrieval, by location. See AD-037.
//
// One question — *what applies here?* — answered for a place rather than for a
// store: the profile in force, the guidance it renders, the terms bound at that
// point, and the governance windows around them. A caller that had to ask each
// store in turn would be asked to know where the answer lives before it can ask,
// which inverts the question.
//
// This is the single implementation behind BOTH `kapi context <path>` and the
// `context://` MCP resources. The two surfaces are wrappers over it, so they
// cannot drift — the fragmentation AD-037 records came from each surface growing
// its own retrieval path.
//
// It resolves through project.ResolveGovernanceFor, the same seam a run, a check
// and a push resolve through. The voice a writer reads here is therefore the
// voice a run applies to the same file, including a content item's own
// `channel:` and a profile whose window has closed.

// ContextPointRequest is one by-location question.
type ContextPointRequest struct {
	// Path is the location to answer for: absolute, or relative — to the working
	// directory when that sits inside the project, and to the project root
	// otherwise. A location outside the project is refused rather than answered.
	// Empty with a Profile set is the by-name form.
	Path string
	// Profile names a governance profile to answer for instead of a location —
	// the ad-hoc form, for a caller with no file in hand. A name the recipe does
	// not declare falls back to a voice profile of that name (the local store,
	// then a built-in pack), which is what makes the form usable outside a
	// project at all.
	Profile string
	// Locale narrows the reported terms to one language when set.
	Locale model.LocaleID
	// Limit caps the terms rendered. The answer reports the total either way, so
	// a capped list reads as a briefing rather than as the whole vocabulary.
	Limit int
	// Declared answers for the blocks the project reads from the file at Path
	// rather than for the file's own content. The two differ for a file an item
	// declares for its comments alone: every block read from it is a comment, so
	// the answer is for the point its comments sit at.
	Declared bool
}

// DefaultContextTermsLimit caps the terms a by-location answer renders when the
// caller names no limit. Higher than the by-content limit on purpose: a search
// answers about one word, while this answers "what holds here", and a vocabulary
// truncated to a handful stops being the answer to that question.
const DefaultContextTermsLimit = 25

// ContextAnswer is what applies at one point.
type ContextAnswer struct {
	Constraints []coreprofile.ConstraintResolution `json:"constraints,omitempty"`
	// Point is the coordinate the request resolved to.
	Point ContextPoint `json:"point"`
	// Scope says how much could have been read, so a thin answer is readable:
	// a project scope that reports no rules has not consulted a concept graph,
	// because it has none.
	Scope ContextScope `json:"scope"`
	// Coverage grades how much of the project's context stands behind the
	// answer: the voice profile in force, the terms bound here and the rules
	// established here, counted, with a suggestion awaiting a decision counting
	// for less than any of them. A caller reading the JSON branches on this
	// rather than on the shape of the lists below.
	Coverage ContextCoverage `json:"coverage"`
	// Provenance says which project answered, at which workspace revision, and
	// whether the content kapi holds still matches the files on disk. nil when
	// no project stands behind the answer.
	Provenance *ContextProvenance `json:"provenance,omitempty"`
	// Voice is the profile in force and its rendered guidance, nil when no voice
	// is bound at this point.
	Voice *ContextVoice `json:"voice,omitempty"`
	// Terms are the terms bound at this point that are in force, ranked with the
	// discouraged ones first — a rule to act on before a word to reuse.
	Terms []ContextTermHit `json:"terms,omitempty"`
	// TermsTotal is how many terms the point binds in all, so a capped list says
	// what it is a part of.
	TermsTotal int `json:"terms_total,omitempty"`
	// Suggestions are what earlier sessions noticed at this point and nobody
	// has established, with the rules a disagreement contests. They are
	// reported apart from Voice and Terms because they bind nothing: a check
	// reports each one and no check fails on it. A writer reads them as the
	// project's own unfinished thinking, and an agent building on another
	// session's work reads them rather than rediscovering the same facts.
	Suggestions []ContextSuggestion `json:"suggestions,omitempty"`
	// Profiles are the governance profiles whose validity is bounded, read
	// against the answer's instant — which voice is in force, and until when.
	// The same shape the by-content answer reports, because it is the same fact.
	Profiles []ContextProfileHit `json:"profiles,omitempty"`
	// Rules is what a writer says and avoids here, as one list: the terms in
	// force, the rules established across the workspace, and the voice's
	// vocabulary, merged so each wording is stated once. Capped at the
	// request's limit; RulesTotal says how many there are in all.
	Rules      []ContextRule `json:"rules,omitempty"`
	RulesTotal int           `json:"rules_total,omitempty"`
	// VoiceBrief is the voice in force as the short brief the text answer
	// leads with: its description and the tone and style fields it sets. The
	// full guide is Voice.Guide.
	VoiceBrief string `json:"voice_brief,omitempty"`
	// Attention holds what a person or an agent must act on before relying on
	// the answer: context files nothing has read in, a voice or terms binding
	// that failed to load, a location governed by a profile of its own. The
	// text answer shows these and nothing else of Notes.
	Attention []string `json:"attention,omitempty"`
	// Notes carries every caveat, including freshness and scope, so a thin
	// answer is never ambiguous between "nothing applies here" and "nothing
	// could be consulted". The text answer shows them under --explain.
	Notes []string `json:"notes,omitempty"`
	// Notice names the context files this checkout carries whose project store
	// has never held context, and the command that reads them. An answer
	// carrying one is thin because nothing has been read in, which a caller
	// cannot otherwise tell from a project that governs nothing here.
	Notice *ContextFilesNotice `json:"notice,omitempty"`

	// explain makes the text rendering add how the answer was reached: the
	// point, the binding, the project and revision, and every note.
	explain bool
}

// Explain makes the text rendering add how the answer was reached, the
// details `kapi context --explain` prints.
func (r *ContextAnswer) Explain() { r.explain = true }

// ContextPoint is the coordinate an answer is about.
type ContextPoint struct {
	// Path is the location as asked about, project-relative and
	// slash-separated. Empty for the by-name form.
	Path string `json:"path,omitempty"`
	// Profile is the governance profile in force, empty at the project's
	// default point.
	Profile string `json:"profile,omitempty"`
	// Channel is the surface the location ships on, empty when it binds none.
	Channel string `json:"channel,omitempty"`
	// Collection is the content collection claiming the path, empty when no
	// collection does.
	Collection string `json:"collection,omitempty"`
	// Ref renders Profile and Channel as the recipe writes the binding
	// (`profile/channel`). Empty at the project's default point.
	Ref         string            `json:"ref,omitempty"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Default reports that resolution fell through to the project's default
	// point — a real answer, and a different one from "no profile exists".
	Default bool `json:"default"`
}

// ContextSuggestion is one operation nobody has established, or one a
// disagreement contests, as an answer reports it: the rule or the fact it
// states, who recorded it, and where they saw it.
//
// Status is `suggested` or `contested`, in the vocabulary the operation log
// uses (contextop.StatusSuggested, contextop.StatusContested). It is carried on
// every entry so a caller reading the JSON has the standing of the entry in the
// entry, rather than in the name of the list it arrived in.
type ContextSuggestion struct {
	// Operation is the id, which is what `kapi context keep` takes.
	Operation string `json:"operation,omitempty"`
	// Kind is what the suggestion is about: `term` or `note`.
	Kind string `json:"kind"`
	// Status is what it counts as: `suggested`, or `contested` when another
	// rule disagrees with it.
	Status string `json:"status"`
	// ContestedBy names the operations on the other side of a disagreement.
	ContestedBy []string `json:"contested_by,omitempty"`
	// Term is the form a term suggestion avoids, Forms the other forms it
	// avoids, and Replacement the form to write instead. All are empty for a
	// note.
	Term        string   `json:"term,omitempty"`
	Forms       []string `json:"forms,omitempty"`
	Replacement string   `json:"replacement,omitempty"`
	// Advisory says the rule would report without failing a check once a
	// person keeps it. It decides nothing while the operation is a
	// suggestion.
	Advisory bool `json:"advisory,omitempty"`
	// Text is the prose of a note.
	Text string `json:"text,omitempty"`
	// Note is whatever the actor said about it.
	Note string `json:"note,omitempty"`
	// SuggestedBy is the actor, and Session the run it belonged to, so a person
	// can review or keep a whole session.
	SuggestedBy string `json:"suggested_by,omitempty"`
	Session     string `json:"session,omitempty"`
	// At is when it was recorded, RFC 3339.
	At string `json:"at,omitempty"`
	// Evidence is where it was seen. A suggestion with evidence can be argued
	// with; one without is a preference somebody typed.
	Evidence []contextop.Evidence `json:"evidence,omitempty"`
	// Standing counts the evidence for and against the rule: the sessions
	// that recorded it, corrections toward it, its uses in content and the
	// merges that carried it.
	Standing *contextop.Standing `json:"standing,omitempty"`
}

// ContextVoice is the voice in force at a point, with the guidance it renders.
// It is the review model's own voice row (core/review.Voice), because a
// reviewer and a run are told the same thing about one file.
type ContextVoice = review.Voice

// ContextPointSources are the materials a by-location answer is assembled from.
// A nil or empty member is something the caller could not bind; the answer
// proceeds over the rest and says so, because a project with a voice and no
// terminology is ordinary rather than broken.
type ContextPointSources struct {
	// Recipe is the project whose context space the point resolves against, nil
	// when no project is in scope.
	Recipe *project.KapiProject
	// Governance is the point's resolved governance, as resolved by
	// ResolveContextGovernance. nil when there was no recipe to resolve against.
	Governance *project.ResolvedGovernance
	// Path is the request's location made project-relative and slash-separated;
	// a location outside the project keeps the form the caller wrote, so the
	// refusal can still name what was asked. Empty for the by-name form.
	Path string
	// PathErr is set when the location could not be placed in the project's
	// coordinate space at all. It is the one assembly failure that is not
	// degraded to a note: every other gap leaves a real point with part of its
	// materials missing, while this leaves no point, and the only answer
	// available without one is a different location's.
	PathErr error
	// Collection is the content collection claiming Path, empty when none does.
	Collection string
	// Voice is the profile in force at the point, already composed with the
	// channel and locale overrides that apply there.
	Voice *coreprofile.VoiceProfile
	// VoiceSource is where that profile was loaded from.
	VoiceSource string
	// VoiceErr is set when a bound voice could not be loaded. A binding that
	// will not load is a different answer from no binding at all, and reporting
	// it as the latter tells a caller the project has no voice when in fact it
	// has one nobody can read.
	VoiceErr error
	// Rules are what the project's context operations add at this point: the
	// established rules widened to the workspace, and the suggestions awaiting a
	// decision (C-11). Read through App.ContextRulesAt, the seam a check
	// resolves them with.
	Rules contextop.Resolution
	// Suggestions are this project's undecided operations, newest first, as the
	// operation log holds them. Rules answers what holds at the point; these
	// carry the provenance a reader needs to judge one: who suggested it, in
	// which session, and where they saw it.
	Suggestions []contextop.Record
	// Concepts are the terms bound at the point, read through the same
	// resolution `kapi check` enforces with — so the terms reported here are the
	// terms a check at this location holds content to.
	Concepts []terms.Concept
	// ConceptsErr is set when that resolution failed.
	ConceptsErr error
	// Profiles are the recipe's bounded governance profiles.
	Profiles []ContextProfileHit
	// Freshness carries the staleness notes for the graph these materials come
	// from, resolved by the caller (host/freshness.go) because it is a property
	// of the reader's history rather than of any store.
	Freshness []string
	// Provenance says which project these materials came from and what state
	// it was read at. nil when no project stands behind them.
	Provenance *ContextProvenance
	// Scope names how much of the graph could have been read.
	Scope ContextScope
	// At is the instant governance is read at.
	At time.Time
	// Notes are caveats the assembly itself produced — a location outside the
	// project, a profile no recipe declares.
	Notes []string
	// Unread names the context files a checkout holds whose project store has
	// never held context, and the command that reads them.
	Unread *ContextFilesNotice
}

// ResolveContextGovernance turns a by-location request into the governance in
// force at that point. It is the one place a request becomes a coordinate, so
// the point an answer reports and the point its voice and terms were read at
// cannot be resolved twice and disagree.
//
// A named profile is resolved AS DECLARED (no instant), because "what does this
// profile hold" has an answer before it comes into force and after it lapses;
// the answer reports the window's state alongside. A path is resolved at the
// instant, the view a run takes: point is the location's point at that
// instant, or the project's default point at it for a request that names no
// location.
func ResolveContextGovernance(proj *project.KapiProject, req ContextPointRequest, point project.GovernancePoint) (*project.ResolvedGovernance, error) {
	if proj == nil {
		return nil, nil
	}
	if req.Profile != "" {
		return proj.ResolveGovernanceFor(project.GovernancePoint{Profile: req.Profile})
	}
	return proj.ResolveGovernanceFor(point)
}

// contextPathPoint is the point a by-location request resolves for the file at
// rel, relative to the project root, at the instant at. It is the point of the
// file's own content. A Declared request about a file an item declares for its
// comments alone gets the point of its comments. noReader is
// project.GovernancePoint.NoReader for the file.
func contextPathPoint(proj *project.KapiProject, req ContextPointRequest, rel string, noReader bool, at time.Time) project.GovernancePoint {
	return project.GovernancePoint{Path: rel, NoReader: noReader, Comments: req.Declared && proj.ClaimsOnlyComments(rel, noReader), At: at}
}

// ContextSourcesAt assembles what a by-location answer reads — the one path for
// both `kapi context <path>` and the `context://` MCP resources, so the two
// cannot drift in what they bind.
//
// Everything is best-effort: a project that will not load, a voice binding that
// will not open, a terms resolution that fails, each degrades to a note rather
// than an error, because the rest of the answer is still worth having to a
// caller mid-task. The returned cleanup releases anything opened here.
func (a *App) ContextSourcesAt(cmd Command, req ContextPointRequest) (ContextPointSources, func()) {
	src := ContextPointSources{Scope: ScopeProject, At: a.GovernanceInstant()}
	noop := func() {}
	ctx := CmdContext(cmd)

	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		// The location as asked about, so an answer with no project behind it
		// still names what the question was.
		src.Path = req.Path
		src.Notes = append(src.Notes, a.adHocVoice(ctx, cmd, &src, req))
		return src, noop
	}
	root := filepath.Dir(projectPath)
	proj, lerr := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		src.Path = req.Path
		src.Notes = append(src.Notes, "this project's recipe could not be read, so no point could be resolved: "+lerr.Error())
		return src, noop
	}
	src.Recipe = proj
	src.Profiles = profileHits(proj.ProfileWindows(), src.At)

	// A profile the recipe does not declare is the ad-hoc form: answer from a
	// voice profile of that name instead of failing, since that is what makes
	// the by-name address usable for a pack.
	if req.Profile != "" {
		if _, declared := proj.Profiles[req.Profile]; !declared {
			src.Notes = append(src.Notes, a.adHocVoice(ctx, cmd, &src, req))
			return src, noop
		}
	}

	// The location as the recipe's globs are matched against. A location outside
	// the project matches nothing, and answering it from the project's default
	// point would state a governance the caller never asked about — in the same
	// confident wording a real point gets — so it is refused instead.
	point := project.GovernancePoint{At: src.At}
	if req.Path != "" {
		matched, inside := projectRelative(root, req.Path)
		if !inside {
			src.Path = req.Path
			src.PathErr = fmt.Errorf(
				"%s is not inside this project (%s), so there is no point to answer for. Name a location inside it, either absolute or relative to the project root",
				req.Path, root)
			return src, noop
		}
		src.Path = matched
		// A file the project's ignore rules match is content the project does not
		// declare, so it sits at the project's default point and in no collection.
		if !ProjectIgnores(root, matched) {
			point = contextPathPoint(proj, req, matched, a.NoReaderFor(filepath.Join(root, filepath.FromSlash(matched))), src.At)
			src.Collection = proj.ContentCollectionForPath(matched, point.NoReader)
			if point.Comments {
				src.Collection = proj.CollectionForPath(matched)
			}
		}
	}

	rc, rerr := ResolveContextGovernance(proj, req, point)
	if rerr != nil {
		src.Notes = append(src.Notes, "this point could not be resolved: "+rerr.Error())
		return src, noop
	}
	src.Governance = rc

	// The voice at the point, composed with the overrides that apply there —
	// the same resolution `kapi voice guide` and a translating run take.
	store, release, serr := a.VoiceLookupStore(cmd)
	if serr != nil {
		src.Notes = append(src.Notes, "the voice store could not be opened: "+serr.Error())
	}
	defer release()
	voice, vsrc, found, verr := a.resolveVoiceForGovernance(ctx, root, store, rc, VoiceResolveOptions{
		Locale: string(req.Locale),
	})
	switch {
	case verr != nil:
		src.VoiceErr = verr
	case found:
		src.Voice, src.VoiceSource = voice, displaySource(root, vsrc)
	}

	// The terms bound at the point, through the resolution the check gate uses:
	// the profile's own store when it binds one, else the project's committed
	// terms. Reporting terms a check would not enforce is the failure AD-037
	// records in `voice_guide`, one store further along.
	point.Profile = req.Profile
	if req.Profile != "" {
		point.At = time.Time{}
	}
	concepts, cerr := a.projectConcepts(cmd, point)
	if cerr != nil {
		src.ConceptsErr = cerr
	} else {
		src.Concepts = concepts
	}

	// What the project's context operations add at this point: rules
	// established and widened, and the suggestions nobody has established yet
	// (C-11). Read through ContextRulesAt, the same seam a check resolves them
	// with, so a suggestion an answer mentions is one a check reports.
	if rules, rerr := a.ContextRulesAt(ctx, projectPath, point); rerr == nil {
		src.Rules = rules
	}
	// The operations behind those suggestions, for the provenance a reader
	// judges one by. A workspace that cannot be read leaves the answer without
	// them rather than failing it, the same as every other store here.
	if log, lerr := a.ContextOperations(ctx, ContextLogRequest{
		Project: projectPath, Subjects: true,
	}); lerr == nil {
		src.Suggestions = make([]contextop.Record, 0, len(log.Operations))
		for _, op := range log.Operations {
			if op.Status.Advises() {
				src.Suggestions = append(src.Suggestions, op.Record)
			}
		}
	}

	src.Freshness = a.governanceNotes(cmd)
	src.Provenance = a.contextProvenance(cmd, proj)
	if notice, unread := a.ContextFilesUnread(ctx, projectPath); unread {
		src.Unread = &notice
	}
	return src, noop
}

// adHocVoice fills in the by-name answer when no recipe declares the profile:
// the local voice store, then a built-in pack. It returns the note explaining
// what was answered from, and "" when there was nothing to answer at all.
func (a *App) adHocVoice(ctx context.Context, cmd Command, src *ContextPointSources, req ContextPointRequest) string {
	// Whatever this branch finds, it is not a project's answer, and saying so is
	// what lets a caller tell "this project holds no answer" from "nothing was
	// consulted that could hold one".
	src.Scope = ScopeProfile

	if req.Profile == "" {
		if req.Path != "" {
			return "no kapi project is in scope, so no point could be resolved for this location. Ask from inside a project, or name a profile"
		}
		return "no kapi project is in scope and no profile was named, so there is no point to answer for"
	}

	store, release, err := a.VoiceLookupStore(cmd)
	defer release()
	if err == nil {
		if p, lerr := lookupProfileIn(ctx, store, req.Profile); lerr == nil {
			src.Voice = coreprofile.ResolveProfile(p, req.Locale, "", "")
			src.VoiceSource = "store:" + req.Profile
			return "no recipe declares this profile: answered from the local voice store, so no terms or governance window applies"
		}
	}
	if p, perr := packs.Load(req.Profile); perr == nil {
		src.Voice = coreprofile.ResolveProfile(p, req.Locale, "", "")
		src.VoiceSource = "pack:" + req.Profile
		return "no recipe declares this profile: answered from the built-in pack, so no terms or governance window applies"
	}
	return fmt.Sprintf("no profile named %q is declared by a recipe, held in the local voice store, or shipped as a pack", req.Profile)
}

// displaySource renders where a profile was loaded from as the answer shows it:
// project-relative for a file inside the project, and `pack:`/`store:` forms
// unchanged. An answer is read, quoted and sometimes committed, so a machine's
// absolute path has no business in it.
func displaySource(root, src string) string {
	if src == "" || !filepath.IsAbs(src) {
		return src
	}
	if rel, err := filepath.Rel(root, src); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return src
}

// projectRelative renders a location as the project-relative, slash-separated
// path the recipe's globs are matched against. It reports false for a location
// outside the project, whose caller refuses it rather than matching it against
// globs it can never satisfy.
func projectRelative(root, path string) (string, bool) {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(relativeBase(root), path)
	}
	return underRoot(root, abs)
}

// relativeBase is the directory a relative location is read against: the
// working directory when that directory is inside the project, and the project
// root otherwise.
//
// The caller that binds a project by `-p` or KAPI_PROJECT is precisely the one
// with no working directory in the tree — an agent runner, a CI step, an editor
// extension. Reading its path against cwd lands outside the project every time,
// where nothing can match, and it is also the caller least able to notice.
// Against the root, the path means what the recipe means by it, which is the
// same reading the `context://` resources document.
func relativeBase(root string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return root
	}
	if _, inside := underRoot(root, cwd); inside {
		return cwd
	}
	return root
}

// underRoot reports whether an absolute location sits at or under root, and
// renders it project-relative and slash-separated when it does.
func underRoot(root, abs string) (string, bool) {
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// ResolveContextAt answers what applies at one point.
//
// A missing store degrades to a note rather than failing the call: half an
// answer plus a statement of what was unreachable is more useful to a caller
// mid-task than an error, and dropping the failure silently would make an
// incomplete answer look complete.
func ResolveContextAt(_ context.Context, src ContextPointSources, req ContextPointRequest) (*ContextAnswer, error) {
	if req.Path == "" && req.Profile == "" {
		return nil, errors.New("context: name a location or a profile")
	}
	// A location that resolved to no point is refused rather than answered.
	// Falling through to the project's default point would hand back another
	// location's voice, terms and guidance in the wording a resolved point gets,
	// and the caller has nothing in the answer to tell the two apart.
	if src.PathErr != nil {
		return nil, src.PathErr
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultContextTermsLimit
	}
	scope := src.Scope
	if scope == "" {
		scope = ScopeProject
	}

	// The terms are resolved before the notes are written, because whether
	// there are any is half of the answer's coverage and the coverage note
	// leads the thin ones.
	var (
		all  []ContextTermHit
		hits []ContextTermHit
	)
	if src.ConceptsErr == nil {
		all, _ = termsInForce(src.Concepts, req.Locale, src.At, 0)
		hits = all
		if len(hits) > limit {
			hits = hits[:limit]
		}
	}
	total := len(all)

	res := &ContextAnswer{
		Scope: scope,
		Coverage: coverageOf(
			countKinds(src.Voice != nil, len(hits) > 0, len(src.Rules.Binding) > 0),
			len(src.Rules.Advisory) > 0),
		Provenance: src.Provenance,
		Point:      ContextPoint{Path: src.Path, Collection: src.Collection},
	}
	switch {
	case src.Governance != nil:
		res.Point.Profile = src.Governance.Profile
		res.Point.Channel = src.Governance.Channel
		res.Point.Ref = src.Governance.Ref().String()
		if src.Recipe != nil {
			res.Point.Coordinates = project.MergeCoordinates(
				src.Recipe.Defaults.Coordinates, src.Governance.Ref().Coordinates(),
				collectionCoordinates(src.Recipe, src.Collection))
		}
		// Default is a resolution outcome, so it is only meaningful when a
		// recipe was resolved at all.
		res.Point.Default = res.Point.Ref == ""
	case req.Profile != "":
		// Nothing resolved, but the answer still names what was addressed.
		res.Point.Profile = req.Profile
	}

	// The suggestions at this point, apart from the rules in force. The rules
	// come from the resolution a check reads, so an answer can never mention a
	// suggested rule a check would not report; the operation log supplies who
	// suggested each one and where they saw it.
	res.Suggestions = contextSuggestions(src.Rules.Advisory, src.Suggestions, res.Point.Coordinates)

	// Freshness leads the notes: it is the only note that says the rest of the
	// answer may already describe a graph that has moved, and a reader acts on
	// it by reading again.
	res.Notes = append(res.Notes, src.Freshness...)
	res.Attention = append(res.Attention, src.Freshness...)
	// Then the coverage, for a thin answer. A caller that reads no further has
	// still been told the two things that matter here: how much stands behind
	// this, and what to watch for while it works.
	if note := contextPointCoverageNote(res.Coverage, len(src.Rules.Advisory)); note != "" {
		res.Notes = append(res.Notes, note)
	}
	if src.Provenance != nil && src.Provenance.StaleReason != "" {
		res.Notes = append(res.Notes, src.Provenance.StaleReason)
	}
	if src.Unread != nil {
		res.Notice = src.Unread
		res.Notes = append(res.Notes, src.Unread.Message())
		res.Attention = append(res.Attention, src.Unread.Message()+". Ask the person to run it")
	}
	// The assembly's own notes say why part of the answer could not be
	// reached, or that no project stood behind it, which a caller has to know
	// before relying on the rest.
	for _, n := range src.Notes {
		if n != "" {
			res.Notes = append(res.Notes, n)
			res.Attention = append(res.Attention, n)
		}
	}
	// A location a profile claims is governed apart from the rest of the
	// project, so what holds in the next file over can differ.
	if src.Governance != nil && !res.Point.Default && req.Path != "" && res.Point.Profile != "" {
		res.Attention = append(res.Attention, fmt.Sprintf(
			"this file is governed by the `%s` profile, so what applies here can differ from the rest of the project", res.Point.Profile))
	}
	// A profile that stopped governing on a date has to be visible; a reader is
	// never told a rule is in force by an answer that just watched it lapse.
	if src.Governance != nil && src.Governance.Fallback != nil {
		res.Notes = append(res.Notes, src.Governance.Fallback.String())
	}

	switch {
	case src.VoiceErr != nil:
		note := "the voice bound here could not be loaded: " + src.VoiceErr.Error()
		res.Notes = append(res.Notes, note)
		res.Attention = append(res.Attention, note)
	case src.Voice != nil:
		res.Constraints = coreprofile.ConstraintResolutions(src.Voice)
		res.Voice = &ContextVoice{
			Name:   src.Voice.Name,
			Source: src.VoiceSource,
			Guide:  coreprofile.RenderVoiceGuide(src.Voice),
		}
		res.VoiceBrief = coreprofile.RenderVoiceBrief(src.Voice)
		if src.Governance != nil {
			res.Voice.Field = src.Governance.VoiceField
		}
	default:
		res.Notes = append(res.Notes, "no voice profile is bound at this point, so no tone or style guidance applies")
	}

	switch {
	case src.ConceptsErr != nil:
		note := "the terms bound here could not be read: " + src.ConceptsErr.Error()
		res.Notes = append(res.Notes, note)
		res.Attention = append(res.Attention, note)
	case len(src.Concepts) > 0:
		// A capped list is stated by Terms against TermsTotal, so a caller
		// that draws the list draws the count beside it; the text rendering
		// below says where the rest are.
		res.Terms, res.TermsTotal = hits, total
	case scope == ScopeProject:
		res.Notes = append(res.Notes, "no terms are bound at this point, so terminology was not consulted")
	}

	res.Profiles = src.Profiles
	res.Rules, res.RulesTotal = sayThisNotThat(all, src.Rules.Binding, src.Voice, limit)

	if scope == ScopeProject {
		res.Notes = append(res.Notes,
			"project scope: concept relations, revisions and market scoping live in a connected workspace")
	}

	return res, nil
}

// contextSuggestions renders the undecided operations an answer reports.
//
// Two sources, joined here. The advisory rules are what the resolution says
// holds at this point, already scoped, deduplicated and ordered, and they are
// the list a check reports; the log supplies the operation behind each one. The
// notes are facts somebody recorded that state no rule, so no resolution
// carries them and they are scoped here against the point's own coordinates.
//
// A rule widened out of another project has no operation in this project's log,
// so it is reported with its rule and no provenance rather than dropped.
func contextSuggestions(advisory []coreprofile.TermRule, records []contextop.Record, coordinates map[string]string) []ContextSuggestion {
	if len(advisory) == 0 && len(records) == 0 {
		return nil
	}
	// Records arrive newest first, so the first one about a term is the latest
	// statement of it, which is the one the resolution kept.
	byTerm := make(map[string]contextop.Record, len(records))
	for _, r := range records {
		rule, ok := r.Rule()
		if !ok {
			continue
		}
		key := suggestionKey(rule.Term)
		if key == "" {
			continue
		}
		if _, held := byTerm[key]; !held {
			byTerm[key] = r
		}
	}

	out := make([]ContextSuggestion, 0, len(advisory)+len(records))
	for _, rule := range advisory {
		entry := ContextSuggestion{
			Kind:        string(contextop.SubjectTerm),
			Status:      string(contextop.StatusSuggested),
			Term:        rule.Term,
			Forms:       rule.Forms,
			Replacement: rule.Replacement,
			Advisory:    rule.Advisory,
			Note:        rule.Note,
		}
		if r, held := byTerm[suggestionKey(rule.Term)]; held {
			entry.Operation = r.ID
			entry.Kind = string(r.Subject.Kind)
			entry.Status = string(r.Status)
			entry.ContestedBy = r.ContestedBy
			if entry.Note == "" {
				entry.Note = r.Subject.Text
			}
			entry.SuggestedBy = r.Actor.Name
			if entry.SuggestedBy == "" {
				entry.SuggestedBy = string(r.Actor.Kind)
			}
			entry.Session = r.Actor.Session
			entry.Evidence = r.Evidence
			entry.Standing = r.Standing
			if !r.At.IsZero() {
				entry.At = r.At.UTC().Format(time.RFC3339)
			}
		}
		out = append(out, entry)
	}

	for _, r := range records {
		if r.Subject.Kind != contextop.SubjectNote || r.Subject.Text == "" {
			continue
		}
		if !r.Scope.Covers(coordinates) {
			continue
		}
		entry := ContextSuggestion{
			Operation:   r.ID,
			Kind:        string(contextop.SubjectNote),
			Status:      string(r.Status),
			Text:        r.Subject.Text,
			Note:        r.Note,
			SuggestedBy: r.Actor.Name,
			Session:     r.Actor.Session,
			Evidence:    r.Evidence,
		}
		if entry.SuggestedBy == "" {
			entry.SuggestedBy = string(r.Actor.Kind)
		}
		if !r.At.IsZero() {
			entry.At = r.At.UTC().Format(time.RFC3339)
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// suggestionKey folds a term for comparison the way the resolution folds it, so
// a rule and the operation that suggested it are matched on the same reading of
// the word.
func suggestionKey(term string) string { return strings.ToLower(strings.TrimSpace(term)) }

// termsInForce projects the concepts bound at a point into the answer's hits:
// the terms whose own window covers the instant, discouraged ones first, capped
// at limit. It returns the total in force as well, so a capped list says what it
// is a part of.
//
// The discouraged-first order is the useful one when the list is capped. A rule
// a writer must not break outranks a word it may reuse, and truncating
// alphabetically would drop the rules about as often as it kept them.
func termsInForce(concepts []terms.Concept, locale model.LocaleID, at time.Time, limit int) ([]ContextTermHit, int) {
	var all []ContextTermHit
	for _, c := range concepts {
		for _, t := range c.Terms {
			if locale != "" && t.Locale != locale {
				continue
			}
			if !at.IsZero() && !t.Validity.Matches(graph.ScopeAt(at)) {
				continue
			}
			hit := ContextTermHit{
				ConceptID:   c.ID,
				Term:        t.Text,
				Locale:      string(t.Locale),
				Status:      string(t.Status),
				Definition:  c.Definition,
				Domain:      c.Domain,
				Discouraged: t.Status.Discouraged(),
			}
			if hit.Discouraged {
				hit.Replacement = preferredTerm(c, t.Locale)
			}
			if v := t.Validity; v != nil {
				if v.ValidFrom != nil {
					hit.ValidFrom = v.ValidFrom.UTC().Format(time.RFC3339)
				}
				if v.ValidTo != nil {
					hit.ValidTo = v.ValidTo.UTC().Format(time.RFC3339)
				}
			}
			all = append(all, hit)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if ri, rj := termRank(all[i]), termRank(all[j]); ri != rj {
			return ri < rj
		}
		if a, b := strings.ToLower(all[i].Term), strings.ToLower(all[j].Term); a != b {
			return a < b
		}
		return all[i].Locale < all[j].Locale
	})
	total := len(all)
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, total
}

// termRank orders a term within the answer: a rule to act on, then a word to
// reuse, then the rest.
func termRank(h ContextTermHit) int {
	switch {
	case h.Discouraged:
		return 0
	case h.Status == string(model.TermPreferred):
		return 1
	default:
		return 2
	}
}

// FormatText renders the answer as the brief a writer reads before changing a
// file: the voice, what to say and what not, what has been suggested and not
// yet established, and what to record while working. It lives on the shared
// type rather than in the CLI so the CLI render and the `context://` resource
// body are the same bytes, defined once.
//
// How the answer was reached (the point, the binding, the project and its
// revision, the scope and every note) is left to the JSON form and to
// Explain, because a writer acts on none of it.
func (r *ContextAnswer) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "# %s\n", r.heading())

	for _, a := range r.Attention {
		fmt.Fprintf(w, "\n%s\n", capitalSentence(a))
	}

	if r.VoiceBrief != "" {
		fmt.Fprintf(w, "\n%s", r.VoiceBrief)
	}

	if len(r.Rules) > 0 {
		fmt.Fprintln(w, "\nSay this, not that:")
		showLocale := rulesSpanLocales(r.Rules)
		for _, rule := range r.Rules {
			fmt.Fprintln(w, ruleLine(rule, showLocale))
		}
		if r.RulesTotal > len(r.Rules) {
			fmt.Fprintf(w, "Showing %d of %d. context_search (or `kapi context search <word>`) finds one by name.\n",
				len(r.Rules), r.RulesTotal)
		}
	}

	if suggested := r.suggestedLines(); len(suggested) > 0 {
		fmt.Fprintln(w, "\nSuggested, not yet established:")
		for _, line := range suggested {
			fmt.Fprintln(w, line)
		}
	}

	// Recording is what grows the answer, and it needs a project to record
	// into. An answer with no project behind it stops at what it found.
	recordable := r.Scope != ScopeProfile
	subject := r.subject()
	switch {
	case r.VoiceBrief == "" && len(r.Rules) == 0 && len(r.Suggestions) == 0:
		fmt.Fprintf(w, "\nNothing is recorded for %s yet. Write as the surrounding files do.\n", subject)
		if recordable {
			fmt.Fprintln(w, "\nWhile you read, record the names and spellings this project keeps to, such as a product "+
				"or feature name, with context_observe (or `kapi context observe`).")
		}
	case recordable:
		fmt.Fprintf(w, "\nNothing else is recorded for %s. If you notice a name or spelling the project keeps to, "+
			"record it with context_observe (or `kapi context observe`).\n", subject)
	}
	if r.Provenance != nil && r.Provenance.Sync != nil {
		fmt.Fprintf(w, "\n%s\n", r.Provenance.Sync.Line)
	}

	if r.explain {
		r.formatExplain(w)
	}
	return nil
}

// formatExplain renders how the answer was reached: where the location sits,
// which recipe line bound the voice, what was consulted, which project answered
// at which revision, the governance windows, and every note.
func (r *ContextAnswer) formatExplain(w io.Writer) {
	fmt.Fprintln(w, "\n## How this was answered")
	fmt.Fprintf(w, "\n%s\n", r.where())
	if r.Voice != nil {
		fmt.Fprintf(w, "%s\n", r.voiceLine())
	}
	fmt.Fprintf(w, "%s\n", r.scopeLine())
	if line := provenanceLine(r.Provenance); line != "" {
		fmt.Fprintf(w, "%s\n", line)
	}
	if len(r.Profiles) > 0 {
		fmt.Fprintln(w, "\nGovernance windows:")
		for _, p := range r.Profiles {
			window := strings.TrimSpace(strings.TrimSpace("from "+p.ValidFrom) + " " + strings.TrimSpace("until "+p.ValidTo))
			if p.ValidFrom == "" {
				window = strings.TrimSpace("until " + p.ValidTo)
			}
			if p.ValidTo == "" {
				window = strings.TrimSpace("from " + p.ValidFrom)
			}
			fmt.Fprintf(w, "- `%s`: %s (%s)\n", p.Name, window, p.State)
		}
	}
	if len(r.Notes) > 0 {
		fmt.Fprintln(w, "\nNotes:")
		for _, n := range r.Notes {
			fmt.Fprintf(w, "- %s\n", n)
		}
	}
}

// heading names what the answer is for.
func (r *ContextAnswer) heading() string {
	if r.Point.Path != "" {
		return "Writing " + r.Point.Path
	}
	if r.Point.Profile != "" {
		return "Writing for the " + r.Point.Profile + " profile"
	}
	return "Writing"
}

// subject is how the text refers to what it answers for.
func (r *ContextAnswer) subject() string {
	switch {
	case r.Point.Path != "":
		return "this file"
	case r.Point.Profile != "":
		return "this profile"
	default:
		return "this location"
	}
}

// suggestedLines renders the suggestions a writer has not already been told as
// a rule: what each suggests, then who recorded it and where they saw it.
func (r *ContextAnswer) suggestedLines() []string {
	say := map[string]bool{}
	avoid := map[string]bool{}
	for _, rule := range r.Rules {
		say[fold(rule.Say)] = true
		for _, n := range rule.Not {
			avoid[fold(n)] = true
		}
	}
	var lines []string
	for _, c := range r.Suggestions {
		if c.Kind != string(contextop.SubjectNote) && avoid[fold(c.Term)] && (c.Replacement == "" || say[fold(c.Replacement)]) {
			continue
		}
		lines = append(lines, suggestionLine(c))
	}
	return lines
}

// where states the coordinate in one sentence: the answer's own address, so a
// caller can tell which of several points it just read.
func (r *ContextAnswer) where() string {
	if r.Scope == ScopeProfile {
		if r.Point.Profile != "" {
			return fmt.Sprintf("Profile `%s`, addressed by name: no recipe point stands behind it.", r.Point.Profile)
		}
		return "No point resolved."
	}
	if r.Point.Default {
		if r.Point.Path == "" {
			return "No point resolved."
		}
		return "This location sits at the project's default point: no profile claims it."
	}
	if r.Point.Ref == "" {
		// No recipe resolved a point, so there is no coordinate to state.
		return "No point resolved."
	}
	var parts []string
	if r.Point.Channel != "" {
		parts = append(parts, fmt.Sprintf("channel `%s`", r.Point.Channel))
	}
	if r.Point.Collection != "" {
		parts = append(parts, fmt.Sprintf("collection `%s`", r.Point.Collection))
	}
	if len(r.Point.Coordinates) > 0 {
		axes := make([]string, 0, len(r.Point.Coordinates))
		for axis, value := range r.Point.Coordinates {
			axes = append(axes, fmt.Sprintf("%s=`%s`", axis, value))
		}
		sort.Strings(axes)
		parts = append(parts, "coordinates "+strings.Join(axes, ", "))
	}
	if len(parts) == 0 {
		// A profile with nothing under it: repeating the name as its own gloss
		// would say the same thing twice.
		return fmt.Sprintf("Point `%s`: this profile's whole product, no channel narrowing it.", r.Point.Ref)
	}
	return fmt.Sprintf("Point `%s`: profile `%s`, %s.", r.Point.Ref, r.Point.Profile, strings.Join(parts, ", "))
}

// voiceLine names the voice in force and the line that bound it, so a caller
// that wants a different answer knows what to edit.
func (r *ContextAnswer) voiceLine() string {
	line := fmt.Sprintf("Voice `%s`", r.Voice.Name)
	if r.Voice.Field != "" {
		line += fmt.Sprintf(", bound by `%s`", r.Voice.Field)
	}
	if r.Voice.Source != "" {
		line += fmt.Sprintf(" (%s)", r.Voice.Source)
	}
	return line + "."
}

// scopeLine states what was answered from, so a caller can tell "this project
// holds no answer" from "this scope cannot hold one".
func (r *ContextAnswer) scopeLine() string {
	switch r.Scope {
	case ScopeWorkspace:
		return "Answered from a connected workspace: the full concept graph, with relations, revisions and market scoping."
	case ScopeProfile:
		return "Answered from one voice profile alone: no project was consulted, so there is no terminology and no governance window behind this."
	default:
		return "Answered from this project alone: its terms, its content memory and its voice profile."
	}
}

// suggestionLine renders one suggestion: what it suggests, then who recorded
// it and where they saw it, and the other side when it is contested.
func suggestionLine(c ContextSuggestion) string {
	var b strings.Builder
	switch {
	case c.Kind == string(contextop.SubjectNote):
		fmt.Fprintf(&b, "- %s", c.Text)
	case c.Replacement != "" && c.Replacement != c.Term:
		avoid := make([]string, 0, 1+len(c.Forms))
		for _, form := range append([]string{c.Term}, c.Forms...) {
			avoid = append(avoid, strconv.Quote(form))
		}
		fmt.Fprintf(&b, "- %s, not %s", c.Replacement, strings.Join(avoid, ", "))
	default:
		fmt.Fprintf(&b, "- %s", c.Term)
	}
	if c.Note != "" && c.Note != c.Text {
		fmt.Fprintf(&b, ": %s", c.Note)
	}
	var by []string
	if c.SuggestedBy != "" {
		by = append(by, c.SuggestedBy)
	}
	var seen []string
	for _, e := range c.Evidence {
		if e.Path != "" && !slices.Contains(seen, e.Path) {
			seen = append(seen, e.Path)
		}
	}
	if len(seen) > 0 {
		by = append(by, "seen in "+strings.Join(seen, ", "))
	}
	if len(by) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(by, ", "))
	}
	if standing := c.Standing.Describe(); standing != "" {
		fmt.Fprintf(&b, " · %s", standing)
	}
	if c.Status == string(contextop.StatusContested) && len(c.ContestedBy) > 0 {
		others := make([]string, len(c.ContestedBy))
		for i, id := range c.ContestedBy {
			others[i] = "#" + id
		}
		fmt.Fprintf(&b, " [contested by %s]", strings.Join(others, ", "))
	}
	return b.String()
}

// capitalSentence renders a note as a sentence: capitalised, with a full stop.
func capitalSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	s = string(unicode.ToUpper(r)) + s[size:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
