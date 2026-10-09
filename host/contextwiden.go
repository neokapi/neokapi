package host

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// What widening a rule would newly govern, read before a person widens it.
//
// The scope arithmetic is widenScope, the one function WidenContextOperation
// and a keep with WidenTo use, so the preview shows the scope the widening
// will record. Reach is then read from what this machine holds: the declared
// points of each recipe a checkout is here for, and the units of each
// projection a checkout has built. What could not be read is listed, with
// the reason, rather than counted as nothing.

// ContextWidenPreviewRequest asks what widening a rule would newly reach.
type ContextWidenPreviewRequest struct {
	// Project is the recipe path, or the workspace key of a project with no
	// checkout on this machine. Empty resolves the project the way every other
	// command does. The rule's own project is examined whichever project the
	// request came through.
	Project string
	// ID names the rule, or a decision about it.
	ID string
	// To is "workspace", "project", or the name of a coordinate axis the rule
	// would stop being specific about (ContextWidenRequest.To).
	To string
}

// ContextWidenPreview is what widening a rule would change. Nothing is
// recorded.
type ContextWidenPreview struct {
	// ID is the rule and Project the project whose work produced it.
	ID      string               `json:"id"`
	Project workspace.ProjectKey `json:"project"`
	// To is the widening asked for.
	To string `json:"to"`
	// Rule is the rule itself, so the preview names what would reach further.
	Rule contextop.Subject `json:"rule"`
	// From is the rule's scope now and Scope its scope once widened.
	From  contextop.Scope `json:"from"`
	Scope contextop.Scope `json:"scope"`
	// Projects are the other projects of the workspace the rule would newly
	// answer in, for a widening to the workspace.
	Projects []ContextWidenProject `json:"projects"`
	// Points are the declared points the rule would newly cover, in every
	// project whose recipe is on this machine.
	Points []ContextWidenPoint `json:"points"`
	// Units are the units the rule would newly match, in every project whose
	// projection is built on this machine, at most maxContextWidenUnits of
	// them.
	Units []ContextWidenUnit `json:"units"`
	// Coverage says which projects the units were read from and which were
	// not, and why.
	Coverage ContextWidenCoverage `json:"coverage"`
}

// ContextWidenProject is one project a widened rule would newly answer in.
type ContextWidenProject struct {
	Key  workspace.ProjectKey `json:"key"`
	Name string               `json:"name,omitempty"`
	// CheckedOut reports a checkout of the project on this machine.
	CheckedOut bool `json:"checked_out"`
}

// ContextWidenPoint is one declared point a widened rule would newly cover.
type ContextWidenPoint struct {
	Project workspace.ProjectKey `json:"project"`
	// Ref addresses the point the way a collection names it, empty for the
	// project's own point, and Label names it for a reader.
	Ref         string            `json:"ref"`
	Label       string            `json:"label"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Collections sit exactly here.
	Collections []string `json:"collections"`
}

// ContextWidenUnit is one unit a widened rule would newly match.
type ContextWidenUnit struct {
	Project workspace.ProjectKey `json:"project"`
	// Document is the file, relative to the project root, and Unit the block
	// in it.
	Document string `json:"document"`
	Unit     string `json:"unit"`
	// Text is the unit's source text, cut to an excerpt.
	Text string `json:"text"`
	// Matches is how many times the text holds a form the rule avoids.
	Matches int `json:"matches"`
}

// ContextWidenCoverage says what the preview read and what it did not.
type ContextWidenCoverage struct {
	// Examined lists each project whose projection was read for units.
	Examined []ContextWidenExamined `json:"examined"`
	// NotExamined lists each project the preview could not read units from,
	// with the reason.
	NotExamined []ContextWidenGap `json:"not_examined"`
	// Truncated reports that Units holds the first maxContextWidenUnits of
	// more; Examined carries the full counts.
	Truncated bool `json:"truncated,omitempty"`
}

// ContextWidenExamined is one project whose projection the preview read.
type ContextWidenExamined struct {
	Project workspace.ProjectKey `json:"project"`
	Name    string               `json:"name,omitempty"`
	// Units is how many units the projection holds and Matched how many the
	// widened rule would newly match.
	Units   int `json:"units"`
	Matched int `json:"matched"`
}

// ContextWidenGap is one project the preview could not read units from.
type ContextWidenGap struct {
	Project workspace.ProjectKey `json:"project"`
	Name    string               `json:"name,omitempty"`
	// Reason is one of the ContextWiden* reasons, or what went wrong reading
	// the project's recipe.
	Reason string `json:"reason"`
}

// The reasons a preview gives for a project it did not read.
const (
	// ContextWidenNoCheckout: the project's files are not on this machine, so
	// neither its declared points nor its content could be read.
	ContextWidenNoCheckout = "no checkout on this machine"
	// ContextWidenNoProjection: a checkout is here, and no projection of its
	// content has been built, so its units could not be read.
	ContextWidenNoProjection = "no projection of its content is built on this machine"
	// ContextWidenNoPattern: the rule names no wording to look for, so no unit
	// can match it.
	ContextWidenNoPattern = "the rule names no wording to look for"
)

// maxContextWidenUnits bounds the units a preview lists. The counts in
// Coverage are not bounded.
const maxContextWidenUnits = 200

// maxContextWidenExcerpt bounds a listed unit's text, in runes.
const maxContextWidenExcerpt = 200

// PreviewContextWidening reports what widening a rule would newly govern:
// the scope it would hold at, the projects and declared points it would newly
// cover, and the units it would newly match wherever a projection is built on
// this machine. It records nothing.
func (a *App) PreviewContextWidening(ctx context.Context, req ContextWidenPreviewRequest) (ContextWidenPreview, error) {
	if req.To == "" {
		return ContextWidenPreview{}, errors.New("name what to widen to: \"workspace\", \"project\", or an axis")
	}
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextWidenPreview{}, err
	}
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextWidenPreview{}, err
	}
	if target.Subject.Kind == contextop.SubjectNone {
		return ContextWidenPreview{}, fmt.Errorf("operation %s carries no rule to widen", contextop.ShortID(target.ID))
	}
	if target.Project != s.key {
		// The rule belongs to another project than the request came through.
		// Its reach is read from its own project.
		if s, err = a.contextOpsFor(ctx, target.Project); err != nil {
			return ContextWidenPreview{}, err
		}
	}
	widened, err := widenScope(target, req.To)
	if err != nil {
		return ContextWidenPreview{}, err
	}

	p := &widenPreview{
		app:  a,
		from: target.Scope,
		to:   widened,
		at:   a.GovernanceInstant(),
		re:   widenMatcher(target.Subject),
		out: ContextWidenPreview{
			ID:       target.ID,
			Project:  target.Project,
			To:       req.To,
			Rule:     target.Subject,
			From:     target.Scope,
			Scope:    widened,
			Projects: []ContextWidenProject{},
			Points:   []ContextWidenPoint{},
			Units:    []ContextWidenUnit{},
			Coverage: ContextWidenCoverage{Examined: []ContextWidenExamined{}, NotExamined: []ContextWidenGap{}},
		},
	}
	if err := p.examine(ctx, s.key, a.projectDisplayName(ctx, s), s.recipe, true); err != nil {
		return ContextWidenPreview{}, err
	}
	if req.To == WidenToWorkspace {
		regs, err := s.ws.Projects(ctx)
		if err != nil {
			return ContextWidenPreview{}, err
		}
		for _, reg := range regs {
			if reg.Key == s.key {
				continue
			}
			recipe := registeredCheckout(reg)
			name := workspaceRegistrationName(reg)
			p.out.Projects = append(p.out.Projects, ContextWidenProject{Key: reg.Key, Name: name, CheckedOut: recipe != ""})
			if err := p.examine(ctx, reg.Key, name, recipe, false); err != nil {
				return ContextWidenPreview{}, err
			}
		}
	}
	p.finish()
	return p.out, nil
}

// projectDisplayName is what a person calls the session's project: the
// recipe's name when a checkout is loaded, the registry's otherwise.
func (a *App) projectDisplayName(ctx context.Context, s *contextOpsSession) string {
	if s.proj != nil && s.proj.Name != "" {
		return s.proj.Name
	}
	if reg, ok, err := s.ws.Lookup(ctx, s.key); err == nil && ok {
		return workspaceRegistrationName(reg)
	}
	return string(s.key)
}

// widenPreview accumulates one preview over the projects it examines.
type widenPreview struct {
	app      *App
	from, to contextop.Scope
	at       time.Time
	// re matches the wording the rule avoids, nil for a rule that names none.
	re  *regexp.Regexp
	out ContextWidenPreview
}

// covers reports whether a point at coordinates is newly covered: the widened
// scope reaches it and, in the rule's own project, the scope now does not. In
// another project the rule answers nowhere yet, so every point the widened
// scope reaches is new.
func (p *widenPreview) covers(coordinates map[string]string, current bool) bool {
	return p.to.Covers(coordinates) && !(current && p.from.Covers(coordinates))
}

// examine reads one project's reach: its declared points from its recipe,
// and its units from its projection. A project with neither on this machine
// is listed as not examined.
func (p *widenPreview) examine(ctx context.Context, key workspace.ProjectKey, name, recipe string, current bool) error {
	if recipe == "" {
		p.gap(key, name, ContextWidenNoCheckout)
		return nil
	}
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		p.gap(key, name, fmt.Sprintf("its recipe could not be read: %v", err))
		return nil
	}
	for _, pt := range DeclaredPoints(proj, p.at) {
		if !p.covers(pt.Coordinates, current) {
			continue
		}
		p.out.Points = append(p.out.Points, ContextWidenPoint{
			Project:     key,
			Ref:         pt.Ref.String(),
			Label:       PointLabel(pt.Ref),
			Coordinates: pt.Coordinates,
			Collections: pt.Collections,
		})
	}
	if p.re == nil {
		p.gap(key, name, ContextWidenNoPattern)
		return nil
	}
	read, matched, built, err := p.units(ctx, key, filepath.Dir(recipe), proj, current)
	if err != nil {
		return err
	}
	if !built {
		p.gap(key, name, ContextWidenNoProjection)
		return nil
	}
	p.out.Coverage.Examined = append(p.out.Coverage.Examined, ContextWidenExamined{Project: key, Name: name, Units: read, Matched: matched})
	return nil
}

// gap records a project the preview could not read units from.
func (p *widenPreview) gap(key workspace.ProjectKey, name, reason string) {
	p.out.Coverage.NotExamined = append(p.out.Coverage.NotExamined, ContextWidenGap{Project: key, Name: name, Reason: reason})
}

// units reads the projection a checkout at root has built and collects the
// units the widened rule would newly match. built is false when the checkout
// holds no projection, which is also the answer for one that has no store at
// all: a preview creates nothing.
func (p *widenPreview) units(ctx context.Context, key workspace.ProjectKey, root string, proj *project.KapiProject, current bool) (read, matched int, built bool, err error) {
	db, err := p.app.ProjectDB(withExistingStoresOnly(ctx), root)
	if errors.Is(err, errNoProjectStore) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	has, err := db.HasBlocks(ctx)
	if err != nil || !has {
		return 0, 0, false, err
	}
	sess, err := db.Blocks().Begin(ctx)
	if err != nil {
		return 0, 0, false, fmt.Errorf("open the projection of %s: %w", key, err)
	}
	defer func() { _ = sess.Rollback() }()

	// One resolution per document: every unit of a file sits at its point.
	covered := map[string]bool{}
	coveredAt := func(file string) bool {
		if held, ok := covered[file]; ok {
			return held
		}
		_, _, coordinates, perr := governedPoint(proj, project.GovernancePoint{Path: file, At: p.at})
		held := perr == nil && p.covers(coordinates, current)
		covered[file] = held
		return held
	}
	for b, berr := range sess.Blocks(blockstore.BlockFilter{}) {
		if berr != nil {
			return read, matched, true, fmt.Errorf("read the projection of %s: %w", key, berr)
		}
		if b == nil {
			continue
		}
		read++
		file := filepath.ToSlash(b.Properties.File)
		if !coveredAt(file) {
			continue
		}
		text := model.RunsText(b.SourceRuns())
		n := countUses(text, p.re)
		if n == 0 {
			continue
		}
		matched++
		p.out.Units = append(p.out.Units, ContextWidenUnit{
			Project:  key,
			Document: file,
			Unit:     b.ID,
			Text:     excerpt(text, maxContextWidenExcerpt),
			Matches:  n,
		})
	}
	return read, matched, true, nil
}

// finish orders the units, by project, document and unit, and cuts the list
// to its bound.
func (p *widenPreview) finish() {
	slices.SortFunc(p.out.Units, func(a, b ContextWidenUnit) int {
		if c := strings.Compare(string(a.Project), string(b.Project)); c != 0 {
			return c
		}
		if c := strings.Compare(a.Document, b.Document); c != 0 {
			return c
		}
		return strings.Compare(a.Unit, b.Unit)
	})
	if len(p.out.Units) > maxContextWidenUnits {
		p.out.Units = p.out.Units[:maxContextWidenUnits]
		p.out.Coverage.Truncated = true
	}
}

// widenMatcher matches the wording a rule avoids: a term rule's term and
// forms, or a content-memory pair's source. A note names none.
func widenMatcher(subject contextop.Subject) *regexp.Regexp {
	switch subject.Kind {
	case contextop.SubjectTerm:
		if subject.Term != nil {
			return formMatcher(append([]string{subject.Term.Term}, subject.Term.Forms...), subject.Term.MatchesCase())
		}
	case contextop.SubjectMemory:
		if subject.Memory != nil {
			return formMatcher([]string{subject.Memory.Source}, true)
		}
	}
	return nil
}

// excerpt cuts text to at most n runes, marking the cut.
func excerpt(text string, n int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n]) + "…"
}

// DeclaredPoint is one point a recipe declares: the project's own, a profile
// with no channel, or a profile's channel, with the collections that resolve
// to it at an instant.
type DeclaredPoint struct {
	Ref         project.ChannelRef
	Coordinates map[string]string
	// Collections sit exactly here. A point no collection sits at has an
	// empty list.
	Collections []string
}

// DeclaredPoints lists every point a recipe declares: the project's own
// first, then each profile's channels in profile order. A collection is
// listed at the point it resolves to at the instant, so one whose profile's
// window has closed appears where it is governed rather than where it was
// written.
func DeclaredPoints(proj *project.KapiProject, at time.Time) []DeclaredPoint {
	byRef := map[string][]string{}
	for _, coll := range proj.Collections {
		if coll.Name == "" {
			continue
		}
		rc, err := proj.ResolveGovernanceFor(project.GovernancePoint{Collection: coll.Name, At: at})
		if err != nil {
			continue
		}
		key := rc.Ref().String()
		byRef[key] = append(byRef[key], coll.Name)
	}
	refs := []project.ChannelRef{{}}
	for _, name := range slices.Sorted(maps.Keys(proj.Profiles)) {
		channels := proj.Profiles[name].Channels
		if len(channels) == 0 {
			// A profile that declares no channel is still a point: it can bind
			// a voice, and content can name it.
			refs = append(refs, project.ChannelRef{Profile: name})
			continue
		}
		for _, ch := range channels {
			if ch.ID == "" {
				continue
			}
			refs = append(refs, project.ChannelRef{Profile: name, Channel: ch.ID})
		}
	}
	out := make([]DeclaredPoint, 0, len(refs))
	for _, ref := range refs {
		collections := byRef[ref.String()]
		if collections == nil {
			collections = []string{}
		}
		out = append(out, DeclaredPoint{
			Ref:         ref,
			Coordinates: project.MergeCoordinates(proj.Defaults.Coordinates, ref.Coordinates(), nil),
			Collections: collections,
		})
	}
	return out
}

// PointLabel names a point for a reader: the way a collection names it, or
// "project default" for the project's own.
func PointLabel(ref project.ChannelRef) string {
	if s := ref.String(); s != "" {
		return s
	}
	return "project default"
}
