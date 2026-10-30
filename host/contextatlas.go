package host

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/place"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/terms"
)

// The context atlas (C-11): one read of everything a project's context holds,
// laid out for exploring rather than for deciding. Each record sits at a point
// on the project's axes and carries the statuses it has held over time; each
// event is a moment on one of two clocks, the instant something was recorded
// and the instant a window opens or closes. `kapi context atlas --json` prints
// it, and the desktop's Explore view draws it. The JSON field names are a
// contract with that view (web/docs/contribute/implementation/context/context-atlas.md).

// ContextAtlasFormat and ContextAtlasVersion identify the atlas document.
const (
	ContextAtlasFormat  = "kapi-context-atlas"
	ContextAtlasVersion = 1
	// contextAtlasLimit is how many records an atlas carries when the request
	// sets no limit.
	contextAtlasLimit = 5000
)

// Atlas record families: what a record is.
const (
	AtlasFamilyRule    = "rule"
	AtlasFamilyNote    = "note"
	AtlasFamilyImport  = "import"
	AtlasFamilyMemory  = "memory"
	AtlasFamilyTerm    = "term"
	AtlasFamilyContent = "content"
)

// Atlas axis sources: where an axis comes from.
const (
	AtlasSourceStructural = "structural"
	AtlasSourceDeclared   = "declared"
	AtlasSourceDerived    = "derived"
)

// The derived axes the atlas puts on its records.
const (
	AtlasAxisFamily  = "family"
	AtlasAxisActor   = "actor"
	AtlasAxisProject = "project"
	AtlasAxisLocale  = "locale"
	AtlasAxisRegion  = "region"
)

// Atlas event kinds beside the operation kinds.
const (
	AtlasEventWindowOpens  = "window-opens"
	AtlasEventWindowCloses = "window-closes"
	// AtlasEventTermAdded is a concept entering the terms store, at the
	// instant the store records it was created.
	AtlasEventTermAdded = "term-added"
)

// ContextAtlasRequest names what an atlas reads.
type ContextAtlasRequest struct {
	// Project is the recipe path or the workspace key; empty resolves the
	// project the way every other command does.
	Project string
	// AllProjects reads every project in the workspace. Content cells are then
	// omitted, and so are the terms, which each project keeps in its own store.
	AllProjects bool
	// Content includes content cells: unit counts by collection, locale and
	// state, measured the way `kapi status` measures them.
	Content bool
	// Limit caps the records and the events; zero is 5000. What a cap left out
	// is said in Notes.
	Limit int
}

// ContextAtlas is one read of a project's context, for exploring.
type ContextAtlas struct {
	Format      string        `json:"format"`
	Version     int           `json:"version"`
	Project     string        `json:"project,omitempty"`
	GeneratedAt time.Time     `json:"generated_at"`
	Span        AtlasSpan     `json:"span"`
	Axes        []AtlasAxis   `json:"axes"`
	Places      []AtlasPlace  `json:"places"`
	Records     []AtlasRecord `json:"records"`
	Events      []AtlasEvent  `json:"events"`
	Notes       []string      `json:"notes"`
}

// AtlasSpan is the earliest and the latest instant across records and events.
type AtlasSpan struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// AtlasAxis is one axis the records sit on.
type AtlasAxis struct {
	ID     string       `json:"id"`
	Label  string       `json:"label,omitempty"`
	Kind   string       `json:"kind"`
	Source string       `json:"source"`
	Values []AtlasValue `json:"values"`
}

// AtlasValue is one value of an axis. A geo value's place is described in the
// atlas's places, by the same id.
type AtlasValue struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// AtlasPlace is one place of the project's gazetteer (core/place): where it
// sits, what it is made of, and how to draw it. Containment is the
// declaration alone; a shape is for drawing.
type AtlasPlace struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Names    map[string]string `json:"names,omitempty"`
	Type     string            `json:"type"`
	Within   []string          `json:"within,omitempty"`
	Of       []string          `json:"of,omitempty"`
	Centroid *[2]float64       `json:"centroid,omitempty"`
	Refs     []string          `json:"refs,omitempty"`
	Shape    string            `json:"shape,omitempty"`
	// Geometry is the Polygon or MultiPolygon geometry of a declared place's
	// shape file.
	Geometry *AtlasGeometry `json:"geometry,omitempty"`
	Declared bool           `json:"declared,omitempty"`
}

// AtlasGeometry is a GeoJSON Polygon or MultiPolygon geometry: its type and
// its coordinates as the file writes them, [lon, lat] positions in rings.
type AtlasGeometry struct {
	Type        string `json:"type"`
	Coordinates any    `json:"coordinates"`
}

// AtlasRecord is one thing the context holds: a rule, a note, a content-memory
// pair, a term, or a cell of content.
type AtlasRecord struct {
	ID          string               `json:"id"`
	Family      string               `json:"family"`
	Label       string               `json:"label"`
	Weight      int                  `json:"weight"`
	Coordinates map[string]string    `json:"coordinates"`
	Recorded    time.Time            `json:"recorded"`
	ValidFrom   *time.Time           `json:"valid_from,omitempty"`
	ValidTo     *time.Time           `json:"valid_to,omitempty"`
	History     []AtlasState         `json:"history"`
	Short       string               `json:"short,omitempty"`
	Actor       *contextop.Actor     `json:"actor,omitempty"`
	Evidence    []contextop.Evidence `json:"evidence,omitempty"`
	Text        string               `json:"text,omitempty"`
	Scope       string               `json:"scope,omitempty"`
	ContestedBy []string             `json:"contested_by,omitempty"`
}

// AtlasState is the state a record held from an instant on.
type AtlasState struct {
	At    time.Time `json:"at"`
	State string    `json:"state"`
}

// AtlasEvent is one moment on the atlas's clocks.
type AtlasEvent struct {
	ID        string           `json:"id"`
	At        time.Time        `json:"at"`
	Effective *time.Time       `json:"effective,omitempty"`
	Kind      string           `json:"kind"`
	Record    string           `json:"record,omitempty"`
	Actor     *contextop.Actor `json:"actor,omitempty"`
	Label     string           `json:"label,omitempty"`
}

// The states a content cell takes: the content ladder's wire values, with
// `not-started` for a unit nothing has drafted and `stale` for one whose
// translation was made from source wording the project no longer holds.
const (
	atlasContentNotStarted = "not-started"
	atlasContentStale      = "stale"
)

// ContextAtlas reads one project's context, or the workspace's, as an atlas.
func (a *App) ContextAtlas(ctx context.Context, req ContextAtlasRequest) (ContextAtlas, error) {
	b := atlasBuilder{
		app:   a,
		limit: req.Limit,
		out: ContextAtlas{
			Format:      ContextAtlasFormat,
			Version:     ContextAtlasVersion,
			GeneratedAt: time.Now().UTC(),
			Axes:        []AtlasAxis{},
			Places:      []AtlasPlace{},
			Records:     []AtlasRecord{},
			Events:      []AtlasEvent{},
			Notes:       []string{},
		},
		used: map[string]map[string]bool{},
	}
	if b.limit <= 0 {
		b.limit = contextAtlasLimit
	}
	if req.AllProjects {
		ws, err := a.Workspace(ctx)
		if err != nil {
			return ContextAtlas{}, err
		}
		b.ledger = a.contextLedger(ctx, ws)
		b.all = true
		b.ws = ws
	} else {
		s, err := a.contextOps(ctx, req.Project)
		if err != nil {
			return ContextAtlas{}, err
		}
		b.session = s
		b.ledger, b.key, b.proj = s.ledger, s.key, s.proj
		b.out.Project = string(s.key)
	}
	b.space = b.proj.AxisSpace()

	if err := b.operations(ctx); err != nil {
		return ContextAtlas{}, err
	}
	switch {
	case b.all:
		b.note("Terms are read for one project at a time; this atlas reads every project's context operations.")
	default:
		if err := b.terms(ctx); err != nil {
			return ContextAtlas{}, err
		}
	}
	switch {
	case !req.Content:
	case b.all:
		b.note("Content cells are measured for one project at a time.")
	case b.session == nil || !b.session.checkedOut():
		b.note("Content cells were not measured: this project has no checkout on this machine.")
	default:
		if err := b.content(ctx); err != nil {
			return ContextAtlas{}, err
		}
	}
	b.profileWindows()
	// Time order on the recorded clock, the order a histogram reads them in.
	slices.SortStableFunc(b.out.Events, func(x, y AtlasEvent) int {
		return cmp.Or(x.At.Compare(y.At), strings.Compare(x.ID, y.ID))
	})
	b.axes(ctx)
	b.places()
	b.span()
	b.note("Unit decisions are not read into the atlas; `kapi status` reports them.")
	return b.out, nil
}

// atlasBuilder accumulates one atlas.
type atlasBuilder struct {
	app     *App
	ledger  *contextop.Ledger
	session *contextOpsSession
	ws      *workspace.Workspace
	key     workspace.ProjectKey
	proj    *project.KapiProject
	space   axis.Space
	all     bool
	limit   int
	out     ContextAtlas
	// used collects every value a record carries, per axis.
	used map[string]map[string]bool
}

func (b *atlasBuilder) note(text string) { b.out.Notes = append(b.out.Notes, text) }

// add appends a record, collecting the values it uses.
func (b *atlasBuilder) add(r AtlasRecord) {
	for name, value := range r.Coordinates {
		if value == "" {
			continue
		}
		if b.used[name] == nil {
			b.used[name] = map[string]bool{}
		}
		b.used[name][value] = true
	}
	b.out.Records = append(b.out.Records, r)
}

// room is how many more records the limit allows.
func (b *atlasBuilder) room() int { return max(0, b.limit-len(b.out.Records)) }

// operations reads the context log: one record per operation that carries a
// rule, a note or a content-memory pair, with the statuses it held over time,
// and one event per operation.
func (b *atlasBuilder) operations(ctx context.Context) error {
	all, err := b.ledger.Records(ctx, contextop.Filter{Project: b.key})
	if err != nil {
		return err
	}
	// A check run records what a check found and changes nothing the context
	// holds, so the atlas leaves it out.
	all = slices.DeleteFunc(all, func(r contextop.Record) bool { return r.Kind == contextop.KindCheck })
	histories, complete, err := b.ledger.Histories(ctx, contextop.Filter{Project: b.key, Subjects: true}, 0)
	if err != nil {
		return err
	}
	if !complete {
		b.note(fmt.Sprintf("The log holds more than %d instants, so each record's history shows where it started and where it stands now.", contextop.HistoryLimit))
	}
	// Oldest first: the order a history reads in.
	slices.Reverse(all)
	byID := make(map[string]contextop.Record, len(all))
	for _, r := range all {
		byID[r.ID] = r
	}
	// windowSetAt is when each record's window was last set: the record itself,
	// or the keep that gave it one.
	windowSetAt := map[string]time.Time{}
	var bearers []contextop.Record
	for _, r := range all {
		if r.Kind.Bears() {
			if r.Subject.Kind != contextop.SubjectNone {
				bearers = append(bearers, r)
			}
			if r.Scope.Windowed() {
				windowSetAt[r.ID] = r.At
			}
			continue
		}
		if target, ok := atlasBearer(byID, r); ok && r.Scope.Windowed() {
			windowSetAt[target.ID] = r.At
		}
	}

	if over := len(bearers) - b.limit; over > 0 {
		b.note(fmt.Sprintf("%d of %d recorded rules, notes and pairs are in the atlas; the %d oldest were left out.", b.limit, len(bearers), over))
		bearers = bearers[over:]
	}
	inAtlas := map[string]bool{}
	for _, r := range bearers {
		inAtlas[r.ID] = true
		b.add(b.operationRecord(r, histories[r.ID]))
		if r.Scope.Windowed() {
			b.windowEvents(r.ID, subjectLabel(r.Subject), windowSetAt[r.ID], r.Scope.ValidFrom, r.Scope.ValidTo)
		}
	}

	events := make([]AtlasEvent, 0, len(all))
	for _, r := range all {
		e := AtlasEvent{ID: r.ID, At: r.At, Kind: string(r.Kind), Actor: actorRef(r.Actor)}
		target := r
		if !r.Kind.Bears() {
			target, _ = atlasBearer(byID, r)
		}
		if inAtlas[target.ID] {
			e.Record = target.ID
		}
		e.Label = subjectLabel(target.Subject)
		switch {
		case e.Label != "":
		case r.Correction != nil:
			e.Label = fmt.Sprintf("%q to %q", r.Correction.From, r.Correction.To)
		case r.Kind == contextop.KindReset && r.Before != "":
			e.Label = "back to before #" + contextop.ShortID(r.Before)
		}
		events = append(events, e)
	}
	if over := len(events) - b.limit; over > 0 {
		b.note(fmt.Sprintf("%d of %d context operations are in the event list; the %d oldest were left out.", b.limit, len(events), over))
		events = events[over:]
	}
	b.out.Events = append(b.out.Events, events...)
	reach := "this project"
	if b.all {
		reach = "every project in the workspace"
	}
	noun := "operations"
	if len(all) == 1 {
		noun = "operation"
	}
	b.note(fmt.Sprintf("Read %d context %s of %s.", len(all), noun, reach))
	return nil
}

// operationRecord renders one subject-bearing operation as a record.
func (b *atlasBuilder) operationRecord(r contextop.Record, history []contextop.StatusAt) AtlasRecord {
	family := AtlasFamilyNote
	switch {
	case r.Kind == contextop.KindImport:
		// A file a person read in: its rows arrive as the term and content
		// memory records, and the import itself is the provenance.
		family = AtlasFamilyImport
	case r.Subject.Kind == contextop.SubjectTerm:
		family = AtlasFamilyRule
	case r.Subject.Kind == contextop.SubjectMemory:
		family = AtlasFamilyMemory
	}
	coordinates := maps.Clone(r.Scope.Coordinates)
	if coordinates == nil {
		coordinates = map[string]string{}
	}
	coordinates[AtlasAxisFamily] = family
	coordinates[AtlasAxisActor] = string(cmp.Or(r.Actor.Kind, contextop.ActorPerson))
	if b.all && r.Project != "" {
		coordinates[AtlasAxisProject] = string(r.Project)
	}
	if r.Subject.Memory != nil {
		setLocale(coordinates, r.Subject.Memory.TargetLocale)
	}
	out := AtlasRecord{
		ID:          r.ID,
		Family:      family,
		Label:       subjectLabel(r.Subject),
		Weight:      1,
		Coordinates: coordinates,
		Recorded:    r.At,
		ValidFrom:   r.Scope.ValidFrom,
		ValidTo:     r.Scope.ValidTo,
		Short:       r.Short,
		Actor:       actorRef(r.Actor),
		Evidence:    r.Evidence,
		Text:        r.Subject.Text,
		Scope:       r.Scope.Describe(),
		ContestedBy: r.ContestedBy,
	}
	for _, h := range history {
		out.History = append(out.History, AtlasState{At: h.At, State: string(h.Status)})
	}
	if len(out.History) == 0 {
		out.History = []AtlasState{{At: r.At, State: string(r.Status)}}
	}
	return out
}

// terms reads the project's terms store: one record per concept, at the
// point the concept is scoped to.
func (b *atlasBuilder) terms(ctx context.Context) error {
	s := b.session
	if s == nil {
		return nil
	}
	if s.checkedOut() {
		if info, err := os.Stat(project.LayoutAt(s.root).StorePath()); err != nil || info.IsDir() {
			b.note("This project has no store yet, so it holds no terms.")
			return nil
		}
	}
	p, err := s.projector(ctx)
	if err != nil {
		return err
	}
	store := p.Terms()
	if store == nil {
		return nil
	}
	concepts, err := store.Concepts(ctx)
	if err != nil {
		return fmt.Errorf("read the project's terms: %w", err)
	}
	slices.SortStableFunc(concepts, func(x, y terms.Concept) int {
		return cmp.Or(x.CreatedAt.Compare(y.CreatedAt), strings.Compare(x.ID, y.ID))
	})
	if room := b.room(); len(concepts) > room {
		b.note(fmt.Sprintf("%d of the %d concepts in the terms store are in the atlas.", room, len(concepts)))
		concepts = concepts[len(concepts)-room:]
	}
	source := ""
	if b.proj != nil {
		source = string(b.proj.Defaults.SourceLanguage)
	}
	for _, c := range concepts {
		r := b.conceptRecord(c, model.LocaleID(source))
		b.add(r)
		if !r.Recorded.IsZero() {
			b.out.Events = append(b.out.Events, AtlasEvent{
				ID: r.ID + ":added", At: r.Recorded, Kind: AtlasEventTermAdded, Record: r.ID, Label: r.Label,
			})
		}
		if r.ValidFrom != nil || r.ValidTo != nil {
			b.windowEvents(r.ID, r.Label, r.Recorded, r.ValidFrom, r.ValidTo)
		}
	}
	return nil
}

// conceptRecord renders one concept as a term record: its head term, with the
// forms it discourages, at the point it is scoped to.
func (b *atlasBuilder) conceptRecord(c terms.Concept, source model.LocaleID) AtlasRecord {
	head := c.HeadTerm(source)
	if head == nil && len(c.Terms) > 0 {
		head = &c.Terms[0]
	}
	label, locale := c.ID, ""
	var window *terms.Term
	if head != nil {
		label, locale = head.Text, string(head.Locale)
		var avoided []string
		for i, t := range c.Terms {
			if t.Locale == head.Locale && t.Status.Discouraged() {
				avoided = append(avoided, strconv.Quote(t.Text))
				if window == nil && t.Validity != nil {
					window = &c.Terms[i]
				}
			}
		}
		if len(avoided) > 0 {
			label = fmt.Sprintf("%q, not %s", head.Text, strings.Join(avoided, ", "))
		}
	}
	if window == nil {
		for i, t := range c.Terms {
			if t.Validity != nil && (t.Validity.ValidFrom != nil || t.Validity.ValidTo != nil) {
				window = &c.Terms[i]
				break
			}
		}
	}
	coordinates := maps.Clone(c.Coordinates())
	if coordinates == nil {
		coordinates = map[string]string{}
	}
	if profile := c.Profile(); profile != "" && coordinates[project.ProductAxis] == "" {
		coordinates[project.ProductAxis] = profile
	}
	coordinates[AtlasAxisFamily] = AtlasFamilyTerm
	setLocale(coordinates, locale)
	id := c.ID
	if !strings.HasPrefix(id, "term:") {
		id = "term:" + id
	}
	out := AtlasRecord{
		ID:          id,
		Family:      AtlasFamilyTerm,
		Label:       label,
		Weight:      1,
		Coordinates: coordinates,
		Recorded:    c.CreatedAt.UTC(),
		History:     []AtlasState{{At: c.CreatedAt.UTC(), State: string(contextop.StatusEstablished)}},
		Text:        c.Definition,
	}
	if window != nil {
		out.ValidFrom, out.ValidTo = window.Validity.ValidFrom, window.Validity.ValidTo
	}
	return out
}

// content measures the project's content the way `kapi status` does and adds
// one record per collection, locale and state, weighted by its unit count.
func (b *atlasBuilder) content(ctx context.Context) error {
	s := b.session
	a := b.app
	defer a.scopeSourceLang()()
	a.ResolveSourceLang(s.proj.Defaults.SourceLanguage)
	units, err := a.UnitsFromProject(s.proj, s.root, "")
	if err != nil {
		return fmt.Errorf("resolve content: %w", err)
	}
	if len(units) == 0 {
		b.note("Content cells: the project names no target language, so there is no translated content to count.")
		return nil
	}
	unread := a.newUnreadSet()
	var tally *convergence.CoverageTally
	err = a.withParseCache(s.root, func() error {
		var terr error
		tally, terr = a.coverageTally(ctx, s.proj, s.root, units, nil, unread)
		return terr
	})
	if err != nil {
		return fmt.Errorf("measure content: %w", err)
	}
	at := b.out.GeneratedAt
	cells := 0
	for _, scope := range tally.Scopes() {
		cov, _ := tally.Coverage(scope)
		stale := tally.Stale(scope)
		_, _, point, perr := governedPoint(s.proj, project.GovernancePoint{Collection: scope.Collection, At: at})
		if perr != nil {
			point = nil
		}
		for _, state := range slices.Sorted(maps.Keys(cov.Counts)) {
			n := cov.Counts[state]
			wire := cmp.Or(state, atlasContentNotStarted)
			if state == string(model.TargetStatusDraft) {
				n -= stale
			}
			if n > 0 {
				b.add(b.contentRecord(scope, wire, n, point, at))
				cells++
			}
		}
		if stale > 0 {
			b.add(b.contentRecord(scope, atlasContentStale, stale, point, at))
			cells++
		}
	}
	b.note(fmt.Sprintf("Content cells count units per collection, language and state as they stand now (%d cells); earlier states are not known.", cells))
	return nil
}

// contentRecord renders one content cell.
func (b *atlasBuilder) contentRecord(scope convergence.Scope, state string, n int, point map[string]string, at time.Time) AtlasRecord {
	coordinates := maps.Clone(point)
	if coordinates == nil {
		coordinates = map[string]string{}
	}
	coordinates[AtlasAxisFamily] = AtlasFamilyContent
	setLocale(coordinates, scope.Locale)
	collection := cmp.Or(scope.Collection, "(no collection)")
	return AtlasRecord{
		ID:          "content:" + scope.Collection + ":" + scope.Locale + ":" + state,
		Family:      AtlasFamilyContent,
		Label:       collection + ", " + scope.Locale + ", " + state,
		Weight:      n,
		Coordinates: coordinates,
		History:     []AtlasState{{At: at, State: state}},
	}
}

// profileWindows adds the window boundaries of the profiles the recipe bounds
// in time. A recipe records no instant a window was declared, so these sit on
// the effective clock alone.
func (b *atlasBuilder) profileWindows() {
	if b.proj == nil {
		return
	}
	for _, w := range b.proj.ProfileWindows() {
		label := "profile " + w.Name
		if from := w.Validity.ValidFrom; from != nil {
			b.out.Events = append(b.out.Events, AtlasEvent{ID: "profile:" + w.Name + ":opens", At: from.UTC(), Kind: AtlasEventWindowOpens, Label: label})
		}
		if to := w.Validity.ValidTo; to != nil {
			b.out.Events = append(b.out.Events, AtlasEvent{ID: "profile:" + w.Name + ":closes", At: to.UTC(), Kind: AtlasEventWindowCloses, Label: label})
		}
	}
}

// windowEvents adds the opening and the closing of a record's window: recorded
// at the instant the window was set, effective at its bounds.
func (b *atlasBuilder) windowEvents(record, label string, set time.Time, from, to *time.Time) {
	add := func(kind string, bound *time.Time) {
		if bound == nil {
			return
		}
		effective := bound.UTC()
		e := AtlasEvent{ID: kind + ":" + record, At: set, Kind: kind, Record: record, Label: label, Effective: &effective}
		if set.IsZero() {
			e.At, e.Effective = effective, nil
		}
		b.out.Events = append(b.out.Events, e)
	}
	add(AtlasEventWindowOpens, from)
	add(AtlasEventWindowCloses, to)
}

// axes lays out every axis the records sit on: the structural ones, the ones
// the recipe declares or the records carry, and the derived ones.
func (b *atlasBuilder) axes(ctx context.Context) {
	derived := map[string]bool{AtlasAxisFamily: true, AtlasAxisActor: true, AtlasAxisProject: true, AtlasAxisLocale: true, AtlasAxisRegion: true}
	var out []AtlasAxis

	for _, name := range []string{project.ProductAxis, project.ChannelAxis} {
		values := maps.Clone(b.used[name])
		if values == nil {
			values = map[string]bool{}
		}
		if b.proj != nil {
			for profile, p := range b.proj.Profiles {
				if name == project.ProductAxis {
					values[profile] = true
					continue
				}
				for _, ch := range p.Channels {
					values[ch.ID] = true
				}
			}
		}
		if len(values) == 0 {
			continue
		}
		out = append(out, AtlasAxis{ID: name, Kind: string(axis.KindCategory), Source: AtlasSourceStructural, Values: plainValues(slices.Sorted(maps.Keys(values)), nil)})
	}

	declared := map[string]bool{}
	for _, name := range b.proj.DeclaredAxisNames() {
		declared[name] = true
	}
	for name := range b.used {
		if !derived[name] && name != project.ProductAxis && name != project.ChannelAxis {
			declared[name] = true
		}
	}
	for _, name := range slices.Sorted(maps.Keys(declared)) {
		out = append(out, b.declaredAxis(name))
	}

	for _, name := range []string{AtlasAxisFamily, AtlasAxisActor, AtlasAxisProject, AtlasAxisLocale, AtlasAxisRegion} {
		used := b.used[name]
		if len(used) == 0 {
			continue
		}
		ax := AtlasAxis{ID: name, Kind: string(axis.KindCategory), Source: AtlasSourceDerived}
		switch name {
		case AtlasAxisFamily:
			ax.Values = plainValues(inOrder(used, []string{AtlasFamilyRule, AtlasFamilyNote, AtlasFamilyMemory, AtlasFamilyTerm, AtlasFamilyContent, AtlasFamilyImport}), nil)
		case AtlasAxisActor:
			ax.Values = plainValues(inOrder(used, []string{string(contextop.ActorPerson), string(contextop.ActorAgent), string(contextop.ActorTool)}), nil)
		case AtlasAxisProject:
			ax.Values = plainValues(slices.Sorted(maps.Keys(used)), b.projectLabels(ctx, used))
		case AtlasAxisRegion:
			ax.Kind = string(axis.KindGeo)
			ax.Values = geoValues(used, b.space.Places())
		default:
			ax.Values = plainValues(slices.Sorted(maps.Keys(used)), nil)
		}
		out = append(out, ax)
	}
	b.out.Axes = out
}

// declaredAxis lays out one declared axis as the recipe types it.
func (b *atlasBuilder) declaredAxis(name string) AtlasAxis {
	spec := b.space.Spec(name)
	used := b.used[name]
	ax := AtlasAxis{ID: name, Label: spec.Label, Kind: string(spec.EffectiveKind()), Source: AtlasSourceDeclared}
	switch spec.EffectiveKind() {
	case axis.KindGeo:
		all := maps.Clone(used)
		if all == nil {
			all = map[string]bool{}
		}
		for _, p := range b.space.Places().Declared() {
			all[p.ID] = true
		}
		ax.Values = geoValues(all, b.space.Places())
	case axis.KindRange:
		values := slices.Collect(maps.Keys(used))
		slices.SortFunc(values, func(x, y string) int {
			rx, ex := axis.ParseRange(x)
			ry, ey := axis.ParseRange(y)
			if ex != nil || ey != nil {
				return strings.Compare(x, y)
			}
			return cmp.Or(cmp.Compare(rx.Min, ry.Min), cmp.Compare(rx.Max, ry.Max))
		})
		ax.Values = plainValues(values, spec.Labels)
	default:
		values := slices.Clone(spec.Values)
		for _, v := range slices.Sorted(maps.Keys(used)) {
			if !slices.Contains(values, v) {
				values = append(values, v)
			}
		}
		ax.Values = plainValues(values, spec.Labels)
	}
	if ax.Values == nil {
		ax.Values = []AtlasValue{}
	}
	return ax
}

// projectLabels names each project key by its registration, where the
// workspace knows one.
func (b *atlasBuilder) projectLabels(ctx context.Context, keys map[string]bool) map[string]string {
	if b.ws == nil {
		return nil
	}
	out := map[string]string{}
	for key := range keys {
		reg, ok, err := b.ws.Lookup(ctx, workspace.ProjectKey(key))
		if err == nil && ok && reg.Name != "" {
			out[key] = reg.Name
		}
	}
	return out
}

// span bounds every instant the atlas carries.
func (b *atlasBuilder) span() {
	var from, to time.Time
	see := func(t time.Time) {
		if t.IsZero() {
			return
		}
		if from.IsZero() || t.Before(from) {
			from = t
		}
		if t.After(to) {
			to = t
		}
	}
	for _, r := range b.out.Records {
		see(r.Recorded)
		for _, h := range r.History {
			see(h.At)
		}
		if r.ValidFrom != nil {
			see(*r.ValidFrom)
		}
		if r.ValidTo != nil {
			see(*r.ValidTo)
		}
	}
	for _, e := range b.out.Events {
		see(e.At)
		if e.Effective != nil {
			see(*e.Effective)
		}
	}
	if from.IsZero() {
		from, to = b.out.GeneratedAt, b.out.GeneratedAt
	}
	b.out.Span = AtlasSpan{From: from, To: to}
}

// geoValues lays out geo values by id and name: places made of places first,
// then wider kinds of place before narrower ones (place.Type.Rank), then by
// name.
func geoValues(used map[string]bool, g *place.Gazetteer) []AtlasValue {
	type value struct {
		id, label string
		madeOf    bool
		rank      int
	}
	values := make([]value, 0, len(used))
	for id := range used {
		v := value{id: id, rank: place.Type("").Rank()}
		if p, ok := g.Place(id); ok {
			v.label, v.madeOf, v.rank = p.Name, p.MadeOf(), p.Type.Rank()
		}
		values = append(values, v)
	}
	slices.SortFunc(values, func(x, y value) int {
		if x.madeOf != y.madeOf {
			if x.madeOf {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(x.rank, y.rank), strings.Compare(cmp.Or(x.label, x.id), cmp.Or(y.label, y.id)), strings.Compare(x.id, y.id))
	})
	out := make([]AtlasValue, 0, len(values))
	for _, v := range values {
		out = append(out, AtlasValue{ID: v.id, Label: v.label})
	}
	return out
}

// atlasMaxShape bounds the shape file the atlas reads for one declared place.
const atlasMaxShape = 1 << 20

// places lists the gazetteer the map draws from, sorted by id. It holds every
// place a record's geo coordinate names, every place that contains one of
// those, the members of each place made of places, and every declared place.
// To that it adds the direct children of each country and subdivision in the
// set, and the countries of each region in it.
func (b *atlasBuilder) places() {
	g := b.space.Places()
	set := map[string]bool{}
	var visit func(id string)
	visit = func(id string) {
		if set[id] || !g.Has(id) {
			return
		}
		set[id] = true
		for _, a := range g.Ancestors(id) {
			visit(a)
		}
		if p, _ := g.Place(id); p.MadeOf() {
			for _, m := range p.Of {
				visit(m)
			}
		}
	}
	for name, values := range b.used {
		if name != AtlasAxisRegion && b.space.Spec(name).EffectiveKind() != axis.KindGeo {
			continue
		}
		for id := range values {
			visit(id)
		}
	}
	for _, p := range g.Declared() {
		visit(p.ID)
	}
	closure := slices.Sorted(maps.Keys(set))
	for _, id := range closure {
		p, _ := g.Place(id)
		switch p.Type {
		case place.TypeCountry, place.TypeSubdivision:
			for _, c := range g.Children(id) {
				set[c] = true
			}
		case place.TypeRegion:
			for _, c := range regionCountries(g, id) {
				set[c] = true
			}
		}
	}
	out := make([]AtlasPlace, 0, len(set))
	for _, id := range slices.Sorted(maps.Keys(set)) {
		p, _ := g.Place(id)
		ap := AtlasPlace{
			ID: p.ID, Name: p.Name, Names: p.Names, Type: string(p.Type), Within: p.Within, Of: p.Of,
			Refs: p.Refs, Shape: p.Shape, Declared: p.Declared,
		}
		if p.Centroid != nil {
			ap.Centroid = &[2]float64{round2(p.Centroid.Lat), round2(p.Centroid.Lon)}
		}
		if p.Shape != "" {
			ap.Geometry = b.shapeGeometry(p.ID, p.Shape)
		}
		out = append(out, ap)
	}
	b.out.Places = out
}

// regionCountries are the countries a region holds, through its subregions.
func regionCountries(g *place.Gazetteer, id string) []string {
	var out []string
	for _, c := range g.Children(id) {
		p, _ := g.Place(c)
		switch p.Type {
		case place.TypeCountry:
			out = append(out, c)
		case place.TypeRegion:
			out = append(out, regionCountries(g, c)...)
		}
	}
	return out
}

// shapeGeometry reads a declared place's shape file and reduces it to its
// Polygon or MultiPolygon geometry. A file the atlas cannot read, one larger
// than atlasMaxShape, and one that holds no single polygon geometry are
// skipped with a note naming the recipe key, and the place is drawn by its
// centroid or listed beside the map.
func (b *atlasBuilder) shapeGeometry(id, shape string) *AtlasGeometry {
	key := "places." + id + ".shape"
	if b.session == nil || !b.session.checkedOut() {
		b.note(fmt.Sprintf("%s: %s was not read: this project has no checkout on this machine.", key, shape))
		return nil
	}
	path := filepath.Join(b.session.root, filepath.FromSlash(shape))
	info, err := os.Stat(path)
	switch {
	case err != nil:
		b.note(fmt.Sprintf("%s: %s could not be read, so the place has no outline.", key, shape))
		return nil
	case info.Size() > atlasMaxShape:
		b.note(fmt.Sprintf("%s: %s is larger than 1 MiB, so the place has no outline.", key, shape))
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		b.note(fmt.Sprintf("%s: %s could not be read, so the place has no outline.", key, shape))
		return nil
	}
	geometry, err := polygonGeometry(data)
	if err != nil {
		b.note(fmt.Sprintf("%s: %s %s, so the place has no outline.", key, shape, err))
		return nil
	}
	return geometry
}

// polygonGeometry reduces GeoJSON to one Polygon or MultiPolygon geometry: a
// bare geometry, a Feature holding one, or a FeatureCollection of a single
// such Feature.
func polygonGeometry(data []byte) (*AtlasGeometry, error) {
	var doc struct {
		Type        string            `json:"type"`
		Coordinates json.RawMessage   `json:"coordinates"`
		Geometry    json.RawMessage   `json:"geometry"`
		Features    []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, errors.New("is not GeoJSON")
	}
	switch doc.Type {
	case "Polygon", "MultiPolygon":
		var rings []any
		if len(doc.Coordinates) == 0 || json.Unmarshal(doc.Coordinates, &rings) != nil {
			return nil, fmt.Errorf("holds a %s with no coordinates", doc.Type)
		}
		return &AtlasGeometry{Type: doc.Type, Coordinates: rings}, nil
	case "Feature":
		if len(doc.Geometry) == 0 || string(doc.Geometry) == "null" {
			return nil, errors.New("holds a feature with no geometry")
		}
		return polygonGeometry(doc.Geometry)
	case "FeatureCollection":
		if len(doc.Features) != 1 {
			return nil, fmt.Errorf("holds %d features; a place's shape is one polygon feature", len(doc.Features))
		}
		return polygonGeometry(doc.Features[0])
	case "":
		return nil, errors.New("is not GeoJSON")
	}
	return nil, fmt.Errorf("holds a %s, not a polygon", doc.Type)
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// plainValues lays out values in the order given, labelled where labels name
// them.
func plainValues(values []string, labels map[string]string) []AtlasValue {
	out := make([]AtlasValue, 0, len(values))
	for _, v := range values {
		out = append(out, AtlasValue{ID: v, Label: labels[v]})
	}
	return out
}

// inOrder lists the used values in a fixed order, then any others sorted.
func inOrder(used map[string]bool, order []string) []string {
	var out []string
	for _, v := range order {
		if used[v] {
			out = append(out, v)
		}
	}
	for _, v := range slices.Sorted(maps.Keys(used)) {
		if !slices.Contains(order, v) {
			out = append(out, v)
		}
	}
	return out
}

// setLocale puts a locale on a record's coordinates, with the place it names
// (place.OfLocale): the subdivision of its -u-sd- key, else its region
// subtag's country or region.
func setLocale(coordinates map[string]string, locale string) {
	if locale == "" {
		return
	}
	coordinates[AtlasAxisLocale] = locale
	if region := place.OfLocale(locale); region != "" {
		coordinates[AtlasAxisRegion] = region
	}
}

// subjectLabel is the phrase a person reads for a subject: a note's own
// words, and the rule or the pair as a log line phrases it.
func subjectLabel(s contextop.Subject) string {
	if s.Kind == contextop.SubjectNote {
		return s.Text
	}
	return s.Phrase()
}

// actorRef is an actor as a record carries it, with an empty kind read as a
// person.
func actorRef(a contextop.Actor) *contextop.Actor {
	if a.Kind == "" {
		a.Kind = contextop.ActorPerson
	}
	return &a
}

// atlasBearer walks from an acting operation to the subject-bearing one it
// acts on, bounded so a chain that loops ends.
func atlasBearer(byID map[string]contextop.Record, r contextop.Record) (contextop.Record, bool) {
	for range len(byID) + 1 {
		if r.Kind.Bears() {
			return r, true
		}
		next, ok := byID[r.Target]
		if r.Target == "" || !ok {
			return contextop.Record{}, false
		}
		r = next
	}
	return contextop.Record{}, false
}

// FormatText prints the atlas as a short summary: what it holds, by family and
// by axis, the span it covers, and its notes.
func (m ContextAtlas) FormatText(w io.Writer) error {
	var b strings.Builder
	project := cmp.Or(m.Project, "the workspace")
	fmt.Fprintf(&b, "Context atlas of %s\n", project)
	fmt.Fprintf(&b, "  records  %d\n  events   %d\n", len(m.Records), len(m.Events))
	if len(m.Records) > 0 || len(m.Events) > 0 {
		fmt.Fprintf(&b, "  span     %s to %s\n", m.Span.From.Format(time.DateOnly), m.Span.To.Format(time.DateOnly))
	}
	for _, ax := range m.Axes {
		counts := map[string]int{}
		for _, r := range m.Records {
			if v := r.Coordinates[ax.ID]; v != "" {
				counts[v] += r.Weight
			}
		}
		parts := make([]string, 0, len(ax.Values))
		for _, v := range ax.Values {
			if counts[v.ID] > 0 {
				parts = append(parts, fmt.Sprintf("%s %d", v.ID, counts[v.ID]))
			}
		}
		if len(parts) == 0 {
			parts = append(parts, "none used")
		}
		name := ax.ID
		if ax.Kind != string(axis.KindCategory) {
			name += " (" + ax.Kind + ")"
		}
		fmt.Fprintf(&b, "  %-22s %s\n", name, strings.Join(parts, ", "))
	}
	for _, n := range m.Notes {
		fmt.Fprintf(&b, "  note: %s\n", n)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
