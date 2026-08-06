package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bowsync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// The workspace's governance profiles: the points its content occupies in the
// context space, and what governs each.
//
// A profile here is DERIVED, never stored. A push declares each collection's
// coordinates and the voice they resolved to (the context content type — see
// bowrain/jobs/worker_context.go), so grouping collections by their coordinate
// point recovers the points the workspace has content at. The recipe's own
// `profiles[]` — the `when:` regions in core/project — does not cross the wire:
// the client resolves them and sends the outcome, so one broad profile
// governing two points appears here as the two points it governs.
//
// A voice the hub holds that no collection binds is listed too, undeclared. It
// exists server-side and has nowhere to apply, which is a state worth seeing.

// defaultProfileSlug names the flat default profile — the point content sits at
// when it declares no coordinates. It is the workspace's own identity, labelled
// "Brand", and it is always present whether or not anything has been pushed.
const defaultProfileSlug = "default"

// unboundProfilePrefix marks a slug built from a voice profile id rather than a
// coordinate point.
const unboundProfilePrefix = "voice~"

// maxPendingChangeSetsScanned bounds the op reads behind the pending-changes
// badge.
const maxPendingChangeSetsScanned = 100

// ContextProfileVoice is the voice governing a profile: the hub profile the
// point's collections bind, plus the size of what it enforces.
type ContextProfileVoice struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Version     int       `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`

	PreferredTerms   int `json:"preferred_terms"`
	ForbiddenTerms   int `json:"forbidden_terms"`
	CompetitorTerms  int `json:"competitor_terms"`
	PatternRules     int `json:"pattern_rules"`
	LocaleOverrides  int `json:"locale_overrides"`
	ChannelOverrides int `json:"channel_overrides"`
}

// ContextProfileCollection names one collection sitting at a profile's point.
type ContextProfileCollection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Stream      string `json:"stream,omitempty"`
	ItemLabel   string `json:"item_label"`
	// Owner is "recipe" or "workspace": whether a kapi.yaml declares this
	// collection or the hub created it.
	Owner string `json:"owner"`
}

// ContextProfile is one governance profile: a point in the context space, what
// governs it, and the content that sits there.
type ContextProfile struct {
	// Slug identifies the profile in a URL. "default" for the flat point,
	// `axis~value` pairs joined by "." for a declared one, `voice~<id>` for a
	// voice bound to no point.
	Slug string `json:"slug"`
	// Name is the profile's conventional name — the `when:` values joined by a
	// hyphen in alphabetical axis order, which is also the directory a project
	// keeps its overrides in (core/project.ProfileBinding.ConventionalName).
	// Empty for the default profile, which has no directory of its own.
	Name string `json:"name"`
	// Label is the display name: the conventional name, or "Brand" for the
	// default profile and the voice's own name for an unbound voice.
	Label     string `json:"label"`
	IsDefault bool   `json:"is_default"`
	// Declared is true when a push placed content at this point.
	Declared    bool              `json:"declared"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Channel is the point's value on the channel axis, the one axis the
	// framework reads for itself: it selects an override inside the voice.
	Channel     string                     `json:"channel,omitempty"`
	Voice       *ContextProfileVoice       `json:"voice,omitempty"`
	Collections []ContextProfileCollection `json:"collections"`
	// PendingChanges counts change-sets in review that carry a voice rule for
	// this profile's voice. Concept edits are workspace-wide and carry no
	// point, so they are not counted here.
	PendingChanges int `json:"pending_changes"`

	// voiceID is the bound profile id read off the collections, resolved into
	// Voice before the response is written.
	voiceID string
}

// ContextProfileTerms is the vocabulary every profile shares. Terms are governed
// per workspace and scoped by market and validity, not by coordinates, so the
// base is one number and a profile's own vocabulary lives on its voice.
type ContextProfileTerms struct {
	ConceptCount int `json:"concept_count"`
}

// ContextProfileScan is the workspace's most recent brand scan.
type ContextProfileScan struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ContextProfilesResponse is the profile aggregation for one workspace.
type ContextProfilesResponse struct {
	Profiles []ContextProfile    `json:"profiles"`
	Axes     []string            `json:"axes"`
	Terms    ContextProfileTerms `json:"terms"`
	Scan     *ContextProfileScan `json:"scan,omitempty"`
	// ScanScope says what a scan covers. "workspace" today: a scan job carries
	// no project, collection or coordinate, so its result is reported on the
	// default profile rather than split across points it never distinguished.
	ScanScope string `json:"scan_scope"`
}

// HandleListContextProfiles returns the workspace's governance profiles,
// aggregated from the collections its pushes declared, the voices the hub holds,
// and the workspace vocabulary. Read-only: nothing here is persisted, and a
// store that is unavailable leaves its part of the answer empty rather than
// failing the page. Gated by workspace membership and the content-read
// permission, matching the concept and collection reads it aggregates.
func (s *Server) HandleListContextProfiles(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil || s.Services == nil || s.Services.Project == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	ctx := c.Request().Context()
	wsID, _ := c.Get("workspace_id").(string)
	wsSlug := c.Param("ws")

	projects, err := s.Services.Project.ListProjects(ctx)
	if err != nil {
		return serverErr(c, err)
	}

	voices := s.contextProfileVoices(ctx, wsID)
	points := newProfilePoints()
	for _, p := range projects {
		if p == nil || p.WorkspaceID != wsID || p.Archived {
			continue
		}
		s.collectProfilePoints(ctx, points, p)
	}

	resp := ContextProfilesResponse{
		Profiles:  points.profiles(voices),
		Axes:      points.axes(),
		Terms:     ContextProfileTerms{ConceptCount: s.workspaceConceptCount(ctx, wsSlug)},
		Scan:      s.latestBrandScan(ctx, wsSlug),
		ScanScope: "workspace",
	}
	resp.Profiles = append(resp.Profiles, unboundVoiceProfiles(voices, points.boundVoiceIDs)...)
	s.countPendingVoiceChanges(ctx, wsID, resp.Profiles)
	return c.JSON(http.StatusOK, resp)
}

// profilePoints groups collections by the coordinate point they sit at.
type profilePoints struct {
	bySlug        map[string]*ContextProfile
	boundVoiceIDs map[string]bool
	axisNames     map[string]bool
}

func newProfilePoints() *profilePoints {
	p := &profilePoints{
		bySlug:        map[string]*ContextProfile{},
		boundVoiceIDs: map[string]bool{},
		axisNames:     map[string]bool{},
	}
	// The default point always exists: it is where content that declares no
	// coordinates sits, and it is the workspace's own identity even before a
	// single push.
	p.bySlug[defaultProfileSlug] = &ContextProfile{
		Slug:        defaultProfileSlug,
		Label:       "Brand",
		IsDefault:   true,
		Collections: []ContextProfileCollection{},
	}
	return p
}

// axes lists every axis the workspace's points name, sorted.
func (p *profilePoints) axes() []string {
	names := make([]string, 0, len(p.axisNames))
	for name := range p.axisNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// collectProfilePoints folds one project's collections into the point index.
// Store failures are skipped: a project whose collections cannot be read leaves
// the rest of the board standing.
func (s *Server) collectProfilePoints(ctx context.Context, points *profilePoints, p *store.Project) {
	streams := []string{"main"}
	if list, err := s.ContentStore.ListStreams(ctx, p.ID, false); err == nil {
		for _, st := range list {
			if st != nil && st.Name != "" && st.Name != "main" {
				streams = append(streams, st.Name)
			}
		}
	}

	seen := map[string]bool{}
	for _, stream := range streams {
		cols, err := s.ContentStore.ListCollections(ctx, p.ID, stream)
		if err != nil {
			continue
		}
		for _, col := range cols {
			if col == nil || seen[col.ID] {
				continue
			}
			seen[col.ID] = true
			points.add(p, col)
		}
	}
}

// add files one collection under its point, creating the point on first sight.
func (p *profilePoints) add(project *store.Project, col *store.Collection) {
	slug := profilePointSlug(col.Context)
	entry, ok := p.bySlug[slug]
	if !ok {
		entry = &ContextProfile{
			Slug:        slug,
			Name:        profilePointName(col.Context),
			Coordinates: cloneCoordinates(col.Context),
			Collections: []ContextProfileCollection{},
		}
		entry.Label = entry.Name
		p.bySlug[slug] = entry
	}
	entry.Declared = true
	for axis := range col.Context {
		p.axisNames[axis] = true
	}

	entry.Collections = append(entry.Collections, ContextProfileCollection{
		ID:          col.ID,
		Name:        col.Name,
		ProjectID:   project.ID,
		ProjectName: project.Name,
		Stream:      col.Stream,
		ItemLabel:   col.ItemLabel,
		Owner:       bowsync.NormalizeContextOwner(col.Owner),
	})

	// The bound voice and channel come from the two keys core/profile reads on
	// the collection. The first collection at a point that names a voice sets
	// it; a point whose collections disagree is a recipe that resolved two
	// voices for one point, which the loader refuses, so the first is the answer.
	if entry.voiceID == "" {
		entry.voiceID = col.ConnectorConfig[coreprofile.PropertyProfileID]
	}
	if entry.Channel == "" {
		entry.Channel = col.ConnectorConfig[coreprofile.PropertyChannel]
	}
	if entry.voiceID != "" {
		p.boundVoiceIDs[entry.voiceID] = true
	}
}

// profiles renders the index in a stable order: the default first, then the
// declared points by name.
func (p *profilePoints) profiles(voices map[string]*ContextProfileVoice) []ContextProfile {
	out := make([]ContextProfile, 0, len(p.bySlug))
	for _, entry := range p.bySlug {
		entry.Voice = voices[entry.voiceID]
		sortProfileCollections(entry.Collections)
		out = append(out, *entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDefault != out[j].IsDefault {
			return out[i].IsDefault
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

func sortProfileCollections(cols []ContextProfileCollection) {
	sort.SliceStable(cols, func(i, j int) bool {
		if cols[i].ProjectName != cols[j].ProjectName {
			return cols[i].ProjectName < cols[j].ProjectName
		}
		return cols[i].Name < cols[j].Name
	})
}

// profilePointSlug encodes a point as a URL segment. Axis and value are joined
// with "~" and the pairs with ".", so no value containing a hyphen can collide
// with an axis name that does.
func profilePointSlug(coords map[string]string) string {
	if len(coords) == 0 {
		return defaultProfileSlug
	}
	parts := make([]string, 0, len(coords))
	for _, axis := range sortedAxes(coords) {
		parts = append(parts, axis+"~"+coords[axis])
	}
	return strings.Join(parts, ".")
}

// profilePointName renders a point the way core/project names a profile's
// directory: the values joined by a hyphen in alphabetical axis order.
func profilePointName(coords map[string]string) string {
	parts := make([]string, 0, len(coords))
	for _, axis := range sortedAxes(coords) {
		parts = append(parts, coords[axis])
	}
	return strings.Join(parts, "-")
}

func sortedAxes(coords map[string]string) []string {
	axes := make([]string, 0, len(coords))
	for axis := range coords {
		axes = append(axes, axis)
	}
	sort.Strings(axes)
	return axes
}

func cloneCoordinates(coords map[string]string) map[string]string {
	if len(coords) == 0 {
		return nil
	}
	out := make(map[string]string, len(coords))
	for k, v := range coords {
		out[k] = v
	}
	return out
}

// contextProfileVoices reads the workspace's voice profiles into the summary the
// cards show, keyed by id.
func (s *Server) contextProfileVoices(ctx context.Context, wsID string) map[string]*ContextProfileVoice {
	out := map[string]*ContextProfileVoice{}
	if s.BrandStore == nil || wsID == "" {
		return out
	}
	profiles, err := s.BrandStore.ListProfiles(ctx, wsID)
	if err != nil {
		return out
	}
	for _, p := range profiles {
		if p == nil {
			continue
		}
		out[p.ID] = &ContextProfileVoice{
			ID:               p.ID,
			Name:             p.Name,
			Description:      p.Description,
			Version:          p.Version,
			UpdatedAt:        p.UpdatedAt,
			PreferredTerms:   len(p.Vocabulary.PreferredTerms),
			ForbiddenTerms:   len(p.Vocabulary.ForbiddenTerms),
			CompetitorTerms:  len(p.Vocabulary.CompetitorTerms),
			PatternRules:     len(p.Style.ProhibitedPatterns) + len(p.Style.RequiredPatterns),
			LocaleOverrides:  len(p.Locales),
			ChannelOverrides: len(p.Channels),
		}
	}
	return out
}

// unboundVoiceProfiles lists the voices no collection binds, ordered by name.
func unboundVoiceProfiles(voices map[string]*ContextProfileVoice, bound map[string]bool) []ContextProfile {
	out := make([]ContextProfile, 0)
	for id, voice := range voices {
		if bound[id] {
			continue
		}
		out = append(out, ContextProfile{
			Slug:        unboundProfilePrefix + id,
			Label:       voice.Name,
			Voice:       voice,
			Collections: []ContextProfileCollection{},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

// countPendingVoiceChanges fills PendingChanges from the change-sets in review,
// matching a voice-rule op's profile_id to each profile's bound voice.
func (s *Server) countPendingVoiceChanges(ctx context.Context, wsID string, profiles []ContextProfile) {
	if s.KnowledgeStore == nil || wsID == "" {
		return
	}
	sets, err := s.KnowledgeStore.ListChangeSets(ctx, wsID, knowledge.ChangeSetInReview)
	if err != nil {
		return
	}
	perVoice := map[string]int{}
	for i, cs := range sets {
		if cs == nil {
			continue
		}
		// The ops are a second read per change-set, so the count is bounded.
		// Past the cap the badge under-reports rather than turning a page into
		// a fan-out; a review queue that deep has its own page.
		if i >= maxPendingChangeSetsScanned {
			break
		}
		ops, err := s.KnowledgeStore.ListOps(ctx, wsID, cs.ID)
		if err != nil {
			continue
		}
		for voiceID := range voiceProfileIDsInOps(ops) {
			perVoice[voiceID]++
		}
	}
	for i := range profiles {
		if profiles[i].Voice != nil {
			profiles[i].PendingChanges = perVoice[profiles[i].Voice.ID]
		}
	}
}

// voiceProfileIDsInOps returns the voice profiles a change-set's ops touch. Only
// the voice-rule ops carry one; concept and relation edits are workspace-wide.
func voiceProfileIDsInOps(ops []*knowledge.ChangeSetOp) map[string]bool {
	out := map[string]bool{}
	for _, op := range ops {
		if op == nil {
			continue
		}
		switch op.Op {
		case knowledge.OpVoiceRuleAdd, knowledge.OpVoiceRuleRemove:
		default:
			continue
		}
		var payload struct {
			ProfileID string `json:"profile_id"`
		}
		if err := json.Unmarshal(op.Payload, &payload); err != nil || payload.ProfileID == "" {
			continue
		}
		out[payload.ProfileID] = true
	}
	return out
}

// workspaceConceptCount reports how many concepts the workspace vocabulary
// holds — the base every profile shares. Zero when the store is unavailable.
func (s *Server) workspaceConceptCount(ctx context.Context, wsSlug string) int {
	if s.wsStores == nil || wsSlug == "" {
		return 0
	}
	tb, err := s.wsStores.getTB(wsSlug)
	if err != nil {
		return 0
	}
	_, total, err := tb.Search(ctx, "", "", "", 0, 1)
	if err != nil {
		return 0
	}
	return total
}

// latestBrandScan returns the workspace's most recent scan, or nil when none has
// run. A scan carries no coordinates, so this is the whole workspace's answer.
func (s *Server) latestBrandScan(ctx context.Context, wsSlug string) *ContextProfileScan {
	if s.BrandScanStore == nil || wsSlug == "" {
		return nil
	}
	jobs, err := s.BrandScanStore.ListBrandScanJobs(ctx, wsSlug, 1)
	if err != nil || len(jobs) == 0 {
		return nil
	}
	j := jobs[0]
	return &ContextProfileScan{
		JobID:     j.ID,
		Status:    string(j.Status),
		Progress:  j.Progress,
		UpdatedAt: j.UpdatedAt,
	}
}
