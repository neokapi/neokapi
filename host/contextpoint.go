package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
}

// DefaultContextTermsLimit caps the terms a by-location answer renders when the
// caller names no limit. Higher than the by-content limit on purpose: a search
// answers about one word, while this answers "what holds here", and a vocabulary
// truncated to a handful stops being the answer to that question.
const DefaultContextTermsLimit = 25

// ContextAnswer is what applies at one point.
type ContextAnswer struct {
	// Point is the coordinate the request resolved to.
	Point ContextPoint `json:"point"`
	// Scope says how much could have been read, so a thin answer is readable:
	// a project scope that reports no rules has not consulted a concept graph,
	// because it has none.
	Scope ContextScope `json:"scope"`
	// Voice is the profile in force and its rendered guidance, nil when no voice
	// is bound at this point.
	Voice *ContextVoice `json:"voice,omitempty"`
	// Terms are the terms bound at this point that are in force, ranked with the
	// discouraged ones first — a rule to act on before a word to reuse.
	Terms []ContextTermHit `json:"terms,omitempty"`
	// TermsTotal is how many terms the point binds in all, so a capped list says
	// what it is a part of.
	TermsTotal int `json:"terms_total,omitempty"`
	// Profiles are the governance profiles whose validity is bounded, read
	// against the answer's instant — which voice is in force, and until when.
	// The same shape the by-content answer reports, because it is the same fact.
	Profiles []ContextProfileHit `json:"profiles,omitempty"`
	// Notes carries freshness and scope-shaped caveats. Present so a thin answer
	// is never ambiguous between "nothing applies here" and "nothing could be
	// consulted".
	Notes []string `json:"notes,omitempty"`
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
	Ref string `json:"ref,omitempty"`
	// Default reports that resolution fell through to the project's default
	// point — a real answer, and a different one from "no profile exists".
	Default bool `json:"default"`
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
	// Scope names how much of the graph could have been read.
	Scope ContextScope
	// At is the instant governance is read at.
	At time.Time
	// Notes are caveats the assembly itself produced — a location outside the
	// project, a profile no recipe declares.
	Notes []string
}

// ResolveContextGovernance turns a by-location request into the governance in
// force at that point. It is the one place a request becomes a coordinate, so
// the point an answer reports and the point its voice and terms were read at
// cannot be resolved twice and disagree.
//
// A named profile is resolved AS DECLARED (no instant), because "what does this
// profile hold" has an answer before it comes into force and after it lapses;
// the answer reports the window's state alongside. A path is resolved at the
// instant, the view a run takes.
func ResolveContextGovernance(proj *project.KapiProject, req ContextPointRequest, relPath string, at time.Time) (*project.ResolvedGovernance, error) {
	if proj == nil {
		return nil, nil
	}
	if req.Profile != "" {
		return proj.ResolveGovernanceFor(project.GovernancePoint{Profile: req.Profile})
	}
	return proj.ResolveGovernanceFor(project.GovernancePoint{Path: relPath, At: at})
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
	var rel string
	if req.Path != "" {
		matched, inside := projectRelative(root, req.Path)
		if !inside {
			src.Path = req.Path
			src.PathErr = fmt.Errorf(
				"%s is not inside this project (%s), so there is no point to answer for. Name a location inside it, either absolute or relative to the project root",
				req.Path, root)
			return src, noop
		}
		rel = matched
		src.Path, src.Collection = matched, proj.CollectionForPath(matched)
	}

	rc, rerr := ResolveContextGovernance(proj, req, rel, src.At)
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
	point := project.GovernancePoint{Profile: req.Profile, Path: src.Path, At: src.At}
	if req.Profile != "" {
		point.At = time.Time{}
	}
	concepts, cerr := a.projectConcepts(cmd, point)
	if cerr != nil {
		src.ConceptsErr = cerr
	} else {
		src.Concepts = concepts
	}

	src.Freshness = a.governanceNotes(cmd)
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

	res := &ContextAnswer{Scope: scope, Point: ContextPoint{Path: src.Path, Collection: src.Collection}}
	switch {
	case src.Governance != nil:
		res.Point.Profile = src.Governance.Profile
		res.Point.Channel = src.Governance.Channel
		res.Point.Ref = src.Governance.Ref().String()
		// Default is a resolution outcome, so it is only meaningful when a
		// recipe was resolved at all.
		res.Point.Default = res.Point.Ref == ""
	case req.Profile != "":
		// Nothing resolved, but the answer still names what was addressed.
		res.Point.Profile = req.Profile
	}

	// Freshness leads the notes: it is the only note that says the rest of the
	// answer may already describe a graph that has moved.
	res.Notes = append(res.Notes, src.Freshness...)
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
		res.Terms, res.TermsTotal = termsInForce(src.Concepts, req.Locale, src.At, limit)
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
