package host

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/state"
)

// The staleness gate: content produced under a context that has since been
// superseded.
//
// Every translation producer stamps model.Origin.ContextFingerprint — a hash of
// the governing context as it actually reached it, the rendered voice guidance
// and the terminology it was given. Until now nothing compared it, so a project
// could change the voice its content is written in and keep shipping every
// target written in the old one, with no surface saying so. Coverage would read
// 100%: the units are there, they are approved, and they are governed by a
// context that no longer exists.
//
// The gate compares the stamp against the context in force AT THE POINT THAT
// GOVERNS EACH FILE, so a recipe that binds two voices holds each file to its
// own. It is a ship-gate concern: superseded governance blocks the release bar,
// never an ordinary build, because catching targets up is exactly the work
// `kapi up` exists to absorb.
//
// Unstamped targets are reported and never failed. Content that predates the
// stamp, and anything a person wrote by hand, carries no fingerprint at all —
// failing it would punish content for the mechanism arriving after it, and the
// gate would be unusable on the day it shipped, in every project that has any
// history.

// gateStaleness is the gate's id. There is no flag to tune it: a coverage bar
// is a judgement about how much is enough, and this is not one — a stamp either
// matches the context in force or it does not.
const gateStaleness = "staleness"

// stalenessScope is one (file, locale) pair's standing against the context
// governing it.
type stalenessScope struct {
	unit VerifyUnit
	// stale is how many of the scope's targets carry a superseded stamp, of
	// produced targets in total.
	stale    int
	produced int
	// moved names the ref component the divergence belongs to, and detail is
	// the human half of it.
	moved  ref.Component
	detail string
}

// verifyStaleness evaluates every produced target against the context governing
// its source file, and returns the gate result plus whether there was anything
// to judge at all.
//
// A project whose state store holds no produced target contributes no gate row.
// "0 findings" about content that does not exist reads as a gate that ran and
// passed, which is a claim about provenance nobody made.
func (a *App) verifyStaleness(cmd Command, proj *project.KapiProject, root string, units []VerifyUnit) (verifyGateResult, bool, error) {
	ctx := CmdContext(cmd)
	gate := verifyGateResult{Gate: gateStaleness, Pass: true, Findings: []verifyFinding{}}

	st, err := a.OpenProjectState(ctx, root)
	if err != nil {
		return gate, false, err
	}
	recorded, err := st.All(ctx)
	if err != nil {
		return gate, false, err
	}
	produced := producedTargets(recorded)
	if len(produced) == 0 {
		return gate, false, nil
	}

	current, err := newContextFingerprints(a, cmd, proj, root)
	if err != nil {
		return gate, false, err
	}
	defer current.close()

	docs := a.documentIndexOrEmpty(ctx, root)
	blocks := newSourceBlockCache(a, ctx)
	unstamped := 0
	var scopes []stalenessScope

	for _, u := range units {
		bl, berr := blocks.forPath(u.SourcePath)
		if berr != nil {
			return gate, false, berr
		}
		point := a.unitGovernancePoint(root, u)
		want, ferr := current.at(point, u.Locale)
		if ferr != nil {
			return gate, false, ferr
		}

		scope := stalenessScope{unit: u}
		document := docs.Scope(root, u.SourcePath)
		for _, b := range bl {
			if !b.Translatable {
				continue
			}
			origin, ok := produced[reviewUnitKey(document, blockKey(b), u.Locale)]
			if !ok {
				// Nothing was produced here. Whether that is a gap is the ship
				// gate's question, and answering it twice in two vocabularies
				// is how two gates come to disagree.
				continue
			}
			scope.produced++
			switch {
			case origin.ContextFingerprint == "":
				unstamped++
			case origin.ContextFingerprint != want.fingerprint:
				scope.stale++
				scope.moved, scope.detail = movedComponent(origin, want)
			}
		}
		if scope.stale > 0 {
			scopes = append(scopes, scope)
		}
	}

	for _, s := range scopes {
		gate.Pass = false
		gate.Findings = append(gate.Findings, verifyFinding{
			Gate:     gateStaleness,
			File:     s.unit.DisplayPath,
			Locale:   s.unit.Locale,
			Severity: "error",
			Message: fmt.Sprintf("%d of %d targets were produced under a superseded context — %s moved (%s)",
				s.stale, s.produced, s.moved, s.detail),
			Suggestion: "run `kapi up` to reproduce them under the context now in force",
		})
	}

	// The informational half, once for the run rather than once per file: it is
	// a statement about the project's history, and repeating it per scope would
	// bury the findings that are actually actionable.
	if unstamped > 0 {
		gate.Findings = append(gate.Findings, verifyFinding{
			Gate:       gateStaleness,
			Severity:   "info",
			Message:    fmt.Sprintf("%d targets carry no governing-context stamp — produced before the stamp existed, or written by hand", unstamped),
			Suggestion: "they are reported, never failed; the next `kapi up` stamps what it reproduces",
		})
	}

	return gate, true, nil
}

// producedTargets indexes the state store by document, unit and locale, keeping
// only the records that carry PROVENANCE: a stamp saying what governed the
// target, or a decision saying a person judged it.
//
// A unit with neither is one the project knows about and has not judged. It has
// no provenance to be stale, and counting it would report every untranslated
// string in the project as unstamped. The basis a convergence pass records for a
// target it wrote is not provenance either: it says which source the target
// translates, which is the drift question, and answering the governance question
// with it would report the whole project as produced under no context at all.
func producedTargets(recorded []state.UnitState) map[string]model.Origin {
	out := make(map[string]model.Origin, len(recorded))
	for _, u := range recorded {
		if u.Origin == (model.Origin{}) && u.Decision.ReviewState == "" {
			continue
		}
		out[reviewUnitKey(u.Scope, u.Unit, string(u.Variant.Locale))] = u.Origin
	}
	return out
}

// governingContext is the context in force at one point, for one locale: the
// fingerprint a producer running there now would stamp, plus the profile stamp
// it would carry alongside it.
type governingContext struct {
	fingerprint    string
	profileID      string
	profileVersion string
}

// movedComponent attributes a divergence to a ref component, from the stamps a
// producer left.
//
// The fingerprint alone cannot say what moved — it folds the voice guidance and
// the terminology into one hash on purpose, so that targets written under one
// context fall stale together whichever half changed. What separates them is
// the profile stamp beside it: a changed profile id or revision is the CONTEXT
// component, and an unchanged one with a moved fingerprint leaves the
// terminology as what moved.
func movedComponent(stamped model.Origin, want governingContext) (ref.Component, string) {
	switch {
	case stamped.Profile != want.profileID:
		return ref.ComponentContext, fmt.Sprintf("the governing voice profile is now %s, was %s",
			orNone(want.profileID), orNone(stamped.Profile))
	case stamped.ProfileVersion != want.profileVersion:
		return ref.ComponentContext, fmt.Sprintf("voice profile %s is now revision %s, was %s",
			orNone(want.profileID), orNone(want.profileVersion), orNone(stamped.ProfileVersion))
	default:
		return ref.ComponentTerms, "the terminology in force has changed since they were written"
	}
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// contextFingerprints resolves the context in force at a point, for a locale,
// once per pair.
//
// It resolves through exactly the ladder resolveProjectBindings uses, and folds
// through exactly the function every producer stamps with. That is the whole
// correctness condition of this gate: reproduce the inputs, not an
// approximation of them. A resolution that differed by one entry would report
// the entire project stale against a context nobody had touched.
type contextFingerprints struct {
	app     *App
	cmd     Command
	proj    *project.KapiProject
	root    string
	store   coreprofile.Store
	release func()
	cache   map[string]governingContext
}

func newContextFingerprints(a *App, cmd Command, proj *project.KapiProject, root string) (*contextFingerprints, error) {
	store, release, err := a.VoiceLookupStore(cmd)
	if err != nil {
		return nil, err
	}
	return &contextFingerprints{
		app: a, cmd: cmd, proj: proj, root: root, store: store, release: release,
		cache: map[string]governingContext{},
	}, nil
}

// close releases the voice store, if this run opened one of its own. Inside a
// project the store is the shared pool's and the release is a no-op.
func (c *contextFingerprints) close() {
	if c != nil && c.release != nil {
		c.release()
	}
}

func (c *contextFingerprints) at(point project.GovernancePoint, locale string) (governingContext, error) {
	key := point.Collection + "\x00" + point.Path + "\x00" + locale
	if g, ok := c.cache[key]; ok {
		return g, nil
	}

	p, _, found, err := c.app.ResolveVoiceProfile(CmdContext(c.cmd), c.proj, c.root, VoiceResolveOptions{
		Store: c.store,
		Point: point,
	})
	if err != nil {
		return governingContext{}, err
	}
	if !found {
		p = nil
	}
	rules, err := c.app.ResolveTermRulesFor(c.cmd, locale, point)
	if err != nil {
		return governingContext{}, err
	}

	id, version, fingerprint := coreprofile.GovernanceContext(p, rules)
	g := governingContext{fingerprint: fingerprint, profileID: id, profileVersion: version}
	c.cache[key] = g
	return g, nil
}

// sourceBlockCache reads each source file once for a run. Two locales of one
// file ask the same question of the same bytes, and a project's content is read
// enough times by one gate run already.
type sourceBlockCache struct {
	app    *App
	ctx    context.Context
	blocks map[string][]*model.Block
}

func newSourceBlockCache(a *App, ctx context.Context) *sourceBlockCache {
	return &sourceBlockCache{app: a, ctx: ctx, blocks: map[string][]*model.Block{}}
}

func (s *sourceBlockCache) forPath(path string) ([]*model.Block, error) {
	if b, ok := s.blocks[path]; ok {
		return b, nil
	}
	b, err := s.app.readBlocks(s.ctx, path, s.app.SourceLocale())
	if err != nil {
		return nil, err
	}
	s.blocks[path] = b
	return b, nil
}
