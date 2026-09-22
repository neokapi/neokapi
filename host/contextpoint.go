package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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
	// confirmed here, counted, with a candidate awaiting a decision counting
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
	// Candidates are what earlier sessions proposed and noticed at this point
	// and nobody has decided on. They are reported apart from Voice and Terms
	// because they bind nothing: a check reports each one and no check fails on
	// it. A writer reads them as the project's own unfinished thinking, and an
	// agent building on another session's work reads them rather than
	// rediscovering the same facts.
	Candidates []ContextCandidate `json:"candidates,omitempty"`
	// Profiles are the governance profiles whose validity is bounded, read
	// against the answer's instant — which voice is in force, and until when.
	// The same shape the by-content answer reports, because it is the same fact.
	Profiles []ContextProfileHit `json:"profiles,omitempty"`
	// Notes carries freshness and scope-shaped caveats. Present so a thin answer
	// is never ambiguous between "nothing applies here" and "nothing could be
	// consulted".
	Notes []string `json:"notes,omitempty"`
	// Notice names the context files this checkout carries whose project store
	// has never held context, and the command that reads them. An answer
	// carrying one is thin because nothing has been read in, which a caller
	// cannot otherwise tell from a project that governs nothing here.
	Notice *ContextFilesNotice `json:"notice,omitempty"`
}

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

// ContextCandidate is one operation nobody has decided on, as an answer reports
// it: the rule or the fact it states, who recorded it, and where they saw it.
//
// Status is always `candidate`, in the vocabulary the operation log uses
// (contextop.StatusCandidate). It is carried on every entry so a caller reading
// the JSON has the standing of the entry in the entry, rather than in the name
// of the list it arrived in.
type ContextCandidate struct {
	// Operation is the id, which is what `kapi context confirm` takes.
	Operation string `json:"operation,omitempty"`
	// Kind is what the candidate is about: `term`, `voice` or `note`.
	Kind string `json:"kind"`
	// Status is what it counts as, and is always `candidate`.
	Status string `json:"status"`
	// Term is the word a term or voice candidate is about, and Replacement what
	// it proposes writing instead. Both are empty for a note.
	Term        string `json:"term,omitempty"`
	Replacement string `json:"replacement,omitempty"`
	// List is the voice-profile vocabulary list a voice candidate sits in.
	List string `json:"list,omitempty"`
	// Severity is how hard the rule would bite once a person confirms it. It
	// decides nothing while the operation is a candidate.
	Severity string `json:"severity,omitempty"`
	// Text is the prose of a note.
	Text string `json:"text,omitempty"`
	// Note is whatever the actor said about it.
	Note string `json:"note,omitempty"`
	// ProposedBy is the actor, and Session the run it belonged to, so a person
	// can review or revert a whole session.
	ProposedBy string `json:"proposed_by,omitempty"`
	Session    string `json:"session,omitempty"`
	// At is when it was recorded, RFC 3339.
	At string `json:"at,omitempty"`
	// Evidence is where it was seen. A candidate with evidence can be argued
	// with; one without is a preference somebody typed.
	Evidence []contextop.Evidence `json:"evidence,omitempty"`
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
	// confirmed rules widened to the workspace, and the candidates awaiting a
	// decision (C-11). Read through App.ContextRulesAt, the seam a check
	// resolves them with.
	Rules contextop.Resolution
	// Candidates are this project's undecided operations, newest first, as the
	// operation log holds them. Rules answers what holds at the point; these
	// carry the provenance a reader needs to judge one: who proposed it, in
	// which session, and where they saw it.
	Candidates []contextop.Record
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

	// What the project's context operations add at this point: rules confirmed
	// and widened, and the candidates nobody has decided on yet
	// (C-11). Read through ContextRulesAt, the same seam a check resolves them
	// with, so a candidate an answer mentions is a candidate a check reports.
	if rules, rerr := a.ContextRulesAt(ctx, projectPath, point); rerr == nil {
		src.Rules = rules
	}
	// The operations behind those candidates, for the provenance a reader
	// judges one by. A workspace that cannot be read leaves the answer without
	// them rather than failing it, the same as every other store here.
	if log, lerr := a.ContextOperations(ctx, ContextLogRequest{
		Project: projectPath, Status: contextop.StatusCandidate, Subjects: true,
	}); lerr == nil {
		src.Candidates = make([]contextop.Record, 0, len(log.Operations))
		for _, op := range log.Operations {
			src.Candidates = append(src.Candidates, op.Record)
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
		hits  []ContextTermHit
		total int
	)
	if src.ConceptsErr == nil {
		hits, total = termsInForce(src.Concepts, req.Locale, src.At, limit)
	}

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

	// The candidates at this point, apart from the rules in force. The rules
	// come from the resolution a check reads, so an answer can never mention a
	// rule candidate a check would not report; the operation log supplies who
	// proposed each one and where they saw it.
	res.Candidates = contextCandidates(src.Rules.Advisory, src.Candidates, res.Point.Coordinates)

	// Freshness leads the notes: it is the only note that says the rest of the
	// answer may already describe a graph that has moved.
	res.Notes = append(res.Notes, src.Freshness...)
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
	}
	for _, n := range src.Notes {
		if n != "" {
			res.Notes = append(res.Notes, n)
		}
	}
	// A profile that stopped governing on a date has to be visible; a reader is
	// never told a rule is in force by an answer that just watched it lapse.
	if src.Governance != nil && src.Governance.Fallback != nil {
		res.Notes = append(res.Notes, src.Governance.Fallback.String())
	}

	switch {
	case src.VoiceErr != nil:
		res.Notes = append(res.Notes, "the voice bound here could not be loaded: "+src.VoiceErr.Error())
	case src.Voice != nil:
		res.Constraints = coreprofile.ConstraintResolutions(src.Voice)
		res.Voice = &ContextVoice{
			Name:   src.Voice.Name,
			Source: src.VoiceSource,
			Guide:  coreprofile.RenderVoiceGuide(src.Voice),
		}
		if src.Governance != nil {
			res.Voice.Field = src.Governance.VoiceField
		}
	default:
		res.Notes = append(res.Notes, "no voice profile is bound at this point, so no tone or style guidance applies")
	}

	switch {
	case src.ConceptsErr != nil:
		res.Notes = append(res.Notes, "the terms bound here could not be read: "+src.ConceptsErr.Error())
	case len(src.Concepts) > 0:
		// A capped list is stated by Terms against TermsTotal, so a caller
		// that draws the list draws the count beside it; the text rendering
		// below says where the rest are.
		res.Terms, res.TermsTotal = hits, total
	case scope == ScopeProject:
		res.Notes = append(res.Notes, "no terms are bound at this point, so terminology was not consulted")
	}

	res.Profiles = src.Profiles

	if scope == ScopeProject {
		res.Notes = append(res.Notes,
			"project scope: concept relations, revisions and market scoping live in a connected workspace")
	}

	return res, nil
}

// contextCandidates renders the undecided operations an answer reports.
//
// Two sources, joined here. The advisory rules are what the resolution says
// holds at this point, already scoped, deduplicated and ordered, and they are
// the list a check reports; the log supplies the operation behind each one. The
// notes are facts somebody recorded that state no rule, so no resolution
// carries them and they are scoped here against the point's own coordinates.
//
// A rule widened out of another project has no operation in this project's log,
// so it is reported with its rule and no provenance rather than dropped.
func contextCandidates(advisory []coreprofile.TermRule, records []contextop.Record, coordinates map[string]string) []ContextCandidate {
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
		key := candidateKey(rule.Term)
		if key == "" {
			continue
		}
		if _, held := byTerm[key]; !held {
			byTerm[key] = r
		}
	}

	out := make([]ContextCandidate, 0, len(advisory)+len(records))
	for _, rule := range advisory {
		entry := ContextCandidate{
			Kind:        string(contextop.SubjectTerm),
			Status:      string(contextop.StatusCandidate),
			Term:        rule.Term,
			Replacement: rule.Replacement,
			Severity:    rule.Severity,
			Note:        rule.Note,
		}
		if r, held := byTerm[candidateKey(rule.Term)]; held {
			entry.Operation = r.ID
			entry.Kind = string(r.Subject.Kind)
			if r.Subject.Kind == contextop.SubjectVoice && r.Subject.Voice != nil {
				entry.List = r.Subject.Voice.List
			}
			entry.ProposedBy = r.Actor.Name
			if entry.ProposedBy == "" {
				entry.ProposedBy = string(r.Actor.Kind)
			}
			entry.Session = r.Actor.Session
			entry.Evidence = r.Evidence
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
		entry := ContextCandidate{
			Operation:  r.ID,
			Kind:       string(contextop.SubjectNote),
			Status:     string(contextop.StatusCandidate),
			Text:       r.Subject.Text,
			Note:       r.Note,
			ProposedBy: r.Actor.Name,
			Session:    r.Actor.Session,
			Evidence:   r.Evidence,
		}
		if entry.ProposedBy == "" {
			entry.ProposedBy = string(r.Actor.Kind)
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

// candidateKey folds a term for comparison the way the resolution folds it, so
// a rule and the operation that proposed it are matched on the same reading of
// the word.
func candidateKey(term string) string { return strings.ToLower(strings.TrimSpace(term)) }

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
	if len(all) > limit {
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

// FormatText renders the answer as one markdown document — prose for a model,
// which is what a by-location answer is for. It lives on the shared type rather
// than in the CLI so the CLI render and the `context://` resource body are the
// same bytes, defined once.
func (r *ContextAnswer) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "# %s\n\n", r.heading())
	fmt.Fprintf(w, "%s\n", r.where())
	if r.Voice != nil {
		fmt.Fprintf(w, "%s\n", r.voiceLine())
	}
	fmt.Fprintf(w, "%s\n", r.scopeLine())
	if line := provenanceLine(r.Provenance); line != "" {
		fmt.Fprintf(w, "%s\n", line)
	}

	if r.Voice != nil && r.Voice.Guide != "" {
		// The guide renders its own headings from `# Voice Guide: …` down; one
		// level of demotion nests it under this document's title instead of
		// competing with it.
		fmt.Fprintf(w, "\n%s\n", demoteHeadings(strings.TrimRight(r.Voice.Guide, "\n")))
	}

	if len(r.Terms) > 0 {
		fmt.Fprintln(w, "\n## Terms in force")
		for _, t := range r.Terms {
			fmt.Fprintf(w, "%s\n", termLine(t))
		}
		if r.TermsTotal > len(r.Terms) {
			fmt.Fprintf(w, "\nShowing %d of %d terms bound here. `kapi context search <word>` finds one by name.\n",
				len(r.Terms), r.TermsTotal)
		}
	}

	if len(r.Candidates) > 0 {
		fmt.Fprintln(w, "\n## Candidates, not yet decided")
		fmt.Fprintln(w, "\nProposed here and awaiting a person's decision. A check reports each of these and"+
			" none of them can fail one. Build on them; do not write them up as rules in force.")
		for _, c := range r.Candidates {
			fmt.Fprintf(w, "%s\n", candidateLine(c))
		}
		fmt.Fprintln(w, "\n`kapi context log --status candidate` lists them, `kapi context confirm <id>` makes one binding.")
	}

	if len(r.Profiles) > 0 {
		fmt.Fprintln(w, "\n## Governance windows")
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

	// Notes last and always: they are what makes a thin answer readable rather
	// than ambiguous.
	if len(r.Notes) > 0 {
		fmt.Fprintln(w, "\n## Notes")
		for _, n := range r.Notes {
			fmt.Fprintf(w, "- %s\n", n)
		}
	}
	return nil
}

// heading names what the answer is about.
func (r *ContextAnswer) heading() string {
	if r.Point.Path != "" {
		return "Context at " + r.Point.Path
	}
	if r.Point.Profile != "" {
		return "Context of profile " + r.Point.Profile
	}
	return "Context"
}

// where states the coordinate in one sentence — the answer's own address, so a
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

// scopeLine states what was answered from. AD-037's reach-not-capability rule:
// a caller must be able to tell "this project holds no answer" from "this scope
// cannot hold one", which it cannot do unless the scope is on the answer.
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

// termLine renders one term as the answer shows it: the verdict first, because
// the caller is asking whether it may use the word.
func termLine(t ContextTermHit) string {
	var b strings.Builder
	if t.Discouraged {
		fmt.Fprintf(&b, "- ~~%s~~", t.Term)
		if t.Replacement != "" {
			fmt.Fprintf(&b, " → say **%s**", t.Replacement)
		}
	} else {
		fmt.Fprintf(&b, "- **%s**", t.Term)
	}
	meta := t.Locale
	if t.Status != "" {
		if meta != "" {
			meta += ", "
		}
		meta += t.Status
	}
	if meta != "" {
		fmt.Fprintf(&b, " (%s)", meta)
	}
	if t.ValidTo != "" {
		fmt.Fprintf(&b, " until %s", validityText(t.ValidTo))
	}
	if t.Domain != "" {
		fmt.Fprintf(&b, " [%s]", t.Domain)
	}
	if t.Definition != "" {
		fmt.Fprintf(&b, ": %s", t.Definition)
	}
	return b.String()
}

// candidateLine renders one candidate as the answer shows it: what it says
// first, then who recorded it and where they saw it, then the id a person acts
// on it by.
func candidateLine(c ContextCandidate) string {
	var b strings.Builder
	switch {
	case c.Kind == string(contextop.SubjectNote):
		fmt.Fprintf(&b, "- %s", c.Text)
	case c.Replacement != "":
		fmt.Fprintf(&b, "- ~~%s~~ → say **%s**", c.Term, c.Replacement)
	default:
		fmt.Fprintf(&b, "- **%s**", c.Term)
	}
	if c.List != "" {
		fmt.Fprintf(&b, " (voice `%s`)", c.List)
	}
	if c.Note != "" && c.Note != c.Text {
		fmt.Fprintf(&b, ": %s", c.Note)
	}
	var seen []string
	for _, e := range c.Evidence {
		switch {
		case e.Path != "" && e.Quote != "":
			seen = append(seen, fmt.Sprintf("%s (%q)", e.Path, e.Quote))
		case e.Path != "":
			seen = append(seen, e.Path)
		case e.Quote != "":
			seen = append(seen, strconv.Quote(e.Quote))
		}
	}
	if len(seen) > 0 {
		fmt.Fprintf(&b, ", seen in %s", strings.Join(seen, ", "))
	}
	var by []string
	if c.ProposedBy != "" {
		by = append(by, c.ProposedBy)
	}
	if c.Operation != "" {
		by = append(by, "#"+c.Operation)
	}
	if len(by) > 0 {
		fmt.Fprintf(&b, " [%s]", strings.Join(by, " "))
	}
	return b.String()
}

// demoteHeadings pushes every ATX heading in a markdown fragment down one level
// so it can be nested inside a larger document. Content inside fenced code
// blocks is left alone: a `#` there is a comment, not a heading.
func demoteHeadings(md string) string {
	var out []string
	fenced := false
	for line := range strings.SplitSeq(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "######") {
			line = "#" + line
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
