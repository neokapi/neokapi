package project

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"gopkg.in/yaml.v3"
)

// The context space.
//
// Content is written for a point in a two-axis space: the PRODUCT it belongs to
// and the CHANNEL it ships on. The space is structural rather than declared — a
// key under `profiles:` is a product, the channels that profile lists are the
// channels that product ships on, and a collection names the point its content
// sits at with one `channel:` reference.
//
// A profile binds governance — a voice, a vocabulary — to its whole product.
// The channel then refines it: after a profile is selected, the collection's
// channel selects the matching override inside that profile's voice
// (profile.VoiceProfile.Channels). A channel the profile declares no override
// for is not an error — the base voice applies, which is the right answer for a
// voice that reads the same everywhere.
//
// Values are slugs, never concept references. A concept is designed to be
// renamed and deprecated as vocabulary is revised, and governance that moved
// when someone edited a term would be governance nobody could rely on. A
// profile and a channel may each *carry* a concept for display; resolution
// never looks at it.

// The axes of the context space, as they travel on the sync wire.
//
// Two of them are STRUCTURAL and the framework names them, because a recipe
// does not get to invent them: a profile IS the product value and a channel IS
// the channel value, both derived from one `channel:` key.
//
// The rest are DECLARED. The coordinate map is open — the wire carries
// map<string,string>, the entry hash folds whatever it finds in sorted order,
// and the graph writes each axis as a property — so a project names the
// dimensions its content actually varies along. Brand is the one that comes up
// first and often enough to have a constant, but nothing here is a closed set.
const (
	// ProductAxis carries the profile name.
	ProductAxis = "product"
	// ChannelAxis carries the collection's channel.
	ChannelAxis = "channel"
	// BrandAxis carries the brand a point sits under. Coarser than product: a
	// workspace has brands, a brand has products, a product ships channels.
	//
	// It is an axis rather than a subsystem. Content sits AT a brand the way it
	// sits at a product; what GOVERNS it there — a voice profile, terms, gates —
	// is bound at the point rather than being part of it. Named here so the
	// spelling is one thing rather than each recipe's own.
	BrandAxis = "brand"

	// ModeAxis carries what KIND of document sits at a point, in the sense
	// Diátaxis means: tutorial, how-to, reference, explanation.
	//
	// Declared rather than structural, like brand, and named here for the same
	// reason: the spelling should be one thing. What makes it worth naming is
	// that correct style is a function of mode. Hedging is wrong in a tutorial
	// and right in an explanation; a list is right in reference and wrong in
	// explanation. One profile applied flatly across a tutorial, a reference
	// page and an architecture note is wrong for at least one of them, and no
	// amount of tone or vocabulary fixes that — it is a different axis.
	//
	// The values below are conventions rather than an enum, because the
	// coordinate map is open and a project that splits its documentation some
	// other way is not wrong.
	ModeAxis = "mode"
)

// The Diátaxis modes, as ModeAxis conventionally spells them.
//
// The classification test is two questions: is the reader ACTING or
// UNDERSTANDING, and are they ACQUIRING skill or APPLYING it. Tutorial is
// acting while acquiring, how-to is acting while applying, explanation is
// understanding while acquiring, reference is understanding while applying.
const (
	ModeTutorial    = "tutorial"
	ModeHowTo       = "how-to"
	ModeReference   = "reference"
	ModeExplanation = "explanation"
)

// slugRule describes the shape of a profile name and of a channel, for the
// error message that rejects anything else.
const slugRule = "lowercase letters, digits and hyphens, starting with a letter or digit"

// slugPattern is that shape. Profile names and channels are machine
// identifiers: stable, never translated, and comparable byte for byte.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// conceptPattern is the shape of a concept reference — an identifier carrying
// no whitespace, conventionally `term:<id>`. The reference is not resolved
// here: a recipe has to load without a terms store, so validation only rejects
// what could never be an id.
var conceptPattern = regexp.MustCompile(`^\S+$`)

// DefaultVoiceField names the recipe key a voice came from when no profile
// bound one. Exported because a caller that wants the profile's conventional
// file to answer BEFORE the project default has to be able to tell the two
// apart, and ResolvedGovernance.VoiceField is where that is recorded.
const DefaultVoiceField = "defaults.voice"

// Profile binds governance to one product, and declares the channels that
// product ships on.
//
// The map key under `profiles:` is the profile's name: the product-axis value
// its collections carry, and the directory under `.kapi/profiles/` holding the
// files it overrides. A profile that binds neither a voice nor a vocabulary is
// still a profile — that directory is the binding, and a project keeping its
// overrides there should not have to restate every one of them in the recipe.
type Profile struct {
	// Channels are the surfaces this product's content ships on. A
	// collection binds to one of them through its `channel:`.
	Channels []Channel `yaml:"channels,omitempty" json:"channels,omitempty"`

	// Voice is the voice profile governing this product's content, in the
	// same forms as defaults.voice (a bare path, or profile_file / profile /
	// pack). nil keeps defaults.voice.
	Voice *VoiceBinding `yaml:"voice,omitempty" json:"voice,omitempty"`

	// TermStore is a STANDALONE terms store governing this product's content —
	// a file the recipe points at, resolved relative to the project root. Empty
	// is the ordinary case: the project's own store governs.
	//
	// Spelled `termstore:` rather than `terms:` because a store is not its
	// contents, and because `terms` is already the dnt-check tool's own key for
	// a list of strings. It matches the `--termstore` selector.
	TermStore string `yaml:"termstore,omitempty" json:"termstore,omitempty"`

	// Concept optionally references the concept that names this product, for
	// display. Carried and shape-checked; never resolved during matching.
	Concept string `yaml:"concept,omitempty" json:"concept,omitempty"`

	// ValidFrom and ValidTo bound the profile's governance in time — from when
	// until when this profile is the one in force. Recipe-declared as a date
	// (`YYYY-MM-DD`) or an RFC3339 instant, parsed and range-checked at load;
	// empty is unbounded. The window is the same half-open model terms and graph
	// edges carry (ValidFrom inclusive, ValidTo exclusive). ResolveGovernanceAt
	// honours it and `kapi context search` surfaces it.
	ValidFrom string `yaml:"valid_from,omitempty" json:"valid_from,omitempty"`
	ValidTo   string `yaml:"valid_to,omitempty" json:"valid_to,omitempty"`
}

// Validity parses the profile's declared window into the shared graph.Validity
// vocabulary — the same temporal model the terms store and the graph edges use,
// so a profile's "from when until when" reads and matches identically to a
// term's. It returns nil when the profile declares no bounds, and an error when
// a bound is unparseable or the range is inverted.
func (pr Profile) Validity() (*graph.Validity, error) {
	from, err := parseValidityBound(pr.ValidFrom)
	if err != nil {
		return nil, fmt.Errorf("valid_from: %w", err)
	}
	to, err := parseValidityBound(pr.ValidTo)
	if err != nil {
		return nil, fmt.Errorf("valid_to: %w", err)
	}
	if from == nil && to == nil {
		return nil, nil
	}
	if from != nil && to != nil && to.Before(*from) {
		return nil, fmt.Errorf("valid_to %q is before valid_from %q", pr.ValidTo, pr.ValidFrom)
	}
	return &graph.Validity{ValidFrom: from, ValidTo: to}, nil
}

// parseValidityBound accepts a bare date or an RFC3339 instant; a bare date is
// read as midnight UTC, so a `valid_to` of a date excludes that whole day, which
// is the half-open reading a reader expects from "until".
func parseValidityBound(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("%q is not a date (want YYYY-MM-DD or an RFC3339 instant)", s)
}

// Channel is one surface a product ships on. The short form is the slug
// itself; the long form adds the concept that names it:
//
//	profiles:
//	  bowrain:
//	    channels:
//	      - id: docs
//	        concept: term:9a1c0f42b7
//	      - app
type Channel struct {
	// ID is the slug: the channel as a collection's `channel:` writes it.
	ID string `yaml:"id" json:"id"`
	// Concept optionally references the concept that names this channel, for
	// display. Carried and shape-checked; never resolved during matching.
	Concept string `yaml:"concept,omitempty" json:"concept,omitempty"`
}

// UnmarshalYAML accepts both forms: a scalar is the id, a mapping is the full
// value.
func (c *Channel) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		c.ID = node.Value
		return nil
	}
	type channelAlias Channel
	var alias channelAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*c = Channel(alias)
	return nil
}

// MarshalYAML writes back the form the channel was authored in, so saving a
// recipe does not expand a plain list of slugs into a list of mappings.
func (c Channel) MarshalYAML() (any, error) {
	if c.Concept == "" {
		return c.ID, nil
	}
	type channelAlias Channel
	return channelAlias(c), nil
}

// ChannelRef is a collection's `channel:` resolved to the point it names. The
// zero value is the project's default point: the collection bound to nothing,
// governed by defaults.voice.
type ChannelRef struct {
	// Profile is the profile that declares Channel, empty when the
	// collection binds to nothing.
	Profile string
	// Channel is the channel within that profile.
	Channel string
}

// Coordinates renders the ref as the point it names on the sync wire, omitting
// the axes it does not set. The default point renders as nil, which is what a
// collection claiming no point should put on the wire.
func (r ChannelRef) Coordinates() map[string]string {
	if r.Profile == "" && r.Channel == "" {
		return nil
	}
	out := make(map[string]string, 2)
	if r.Profile != "" {
		out[ProductAxis] = r.Profile
	}
	if r.Channel != "" {
		out[ChannelAxis] = r.Channel
	}
	return out
}

// MergeCoordinates is a collection's point: the project defaults, overlaid with
// what its `channel:` derives, overlaid with whatever it declares itself.
//
// Most specific wins, per axis. The order is the whole rule — a project states
// its brand once under `defaults:` and every collection inherits it, while a
// collection that genuinely sits elsewhere says so and only that axis moves.
// Repeating an axis on every entry is how a declared axis becomes boilerplate
// and then drifts.
//
// A nil result means the default point, which is a real place: the graph writes
// a coordinate node for it, and content governed by nothing but the project's
// own defaults sits there.
func MergeCoordinates(defaults, derived, declared map[string]string) map[string]string {
	out := make(map[string]string, len(defaults)+len(derived)+len(declared))
	for _, layer := range []map[string]string{defaults, derived, declared} {
		for axis, value := range layer {
			if axis == "" || value == "" {
				continue
			}
			out[axis] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// String renders the ref in the qualified form a recipe writes. A ref that sets
// only one axis renders that axis alone: a profile addressed with no channel is
// a point in its own right, and `profile/` would name a channel called nothing.
func (r ChannelRef) String() string {
	switch {
	case r.Profile == "":
		return r.Channel
	case r.Channel == "":
		return r.Profile
	default:
		return r.Profile + "/" + r.Channel
	}
}

// ResolveChannel resolves one `channel:` reference against the declared
// profiles.
//
// A reference is always the qualified `profile/channel` — a channel is a
// surface OF a product, and the binding reads as one. A bare channel name is
// an error that spells out the qualified form(s), so the fix is a copy-paste.
//
// An empty ref resolves to the default point.
func (p *KapiProject) ResolveChannel(ref string) (ChannelRef, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ChannelRef{}, nil
	}
	profile, channel, qualified := strings.Cut(ref, "/")
	if !qualified {
		var owners []string
		for _, name := range sortedKeys(p.Profiles) {
			if declaresChannel(p.Profiles[name].Channels, ref) {
				owners = append(owners, name+"/"+ref)
			}
		}
		if len(owners) > 0 {
			return ChannelRef{}, fmt.Errorf("channel %q must name its profile. Write %s", ref, strings.Join(owners, " or "))
		}
		return ChannelRef{}, fmt.Errorf("channel %q must be `profile/channel`, and no profile declares %q (declared: %s)",
			ref, ref, p.declaredChannels())
	}
	if strings.Contains(channel, "/") {
		return ChannelRef{}, fmt.Errorf("channel %q has too many parts (want `profile/channel`)", ref)
	}
	prof, ok := p.Profiles[profile]
	if !ok {
		return ChannelRef{}, fmt.Errorf("channel %q names profile %q, which profiles: does not declare (declared: %s)",
			ref, profile, p.declaredProfiles())
	}
	if !declaresChannel(prof.Channels, channel) {
		return ChannelRef{}, fmt.Errorf("channel %q is not declared by profile %q (declared: %s)",
			ref, profile, renderChannels(prof.Channels))
	}
	return ChannelRef{Profile: profile, Channel: channel}, nil
}

// ResolvedGovernance is the governance in force over one collection's content:
// the profile its channel resolved to, and what a run carries from it.
//
// Voice and TermStore are resolved already, so a caller applies them without
// knowing whether they came from a profile or from the project defaults.
// Channel is passed on to profile resolution, which selects the matching
// override inside the voice profile.
type ResolvedGovernance struct {
	// Channel is the collection's channel, empty when it binds to none.
	Channel string
	// Voice is the matched profile's voice, else defaults.voice. nil
	// when neither binds one.
	Voice *VoiceBinding
	// TermStore is the matched profile's standalone terms store, as written in
	// the recipe — relative to the project root unless absolute. Empty means the
	// project's own store governs, which is the ordinary case.
	TermStore string
	// VoiceField names the recipe key Voice came from (`profiles.bowrain.voice`
	// or `defaults.voice`), so a profile that cannot be loaded names the line
	// to fix.
	VoiceField string
	// Profile is the matched profile's name — the directory under
	// `.kapi/profiles/` a caller looks in for the files that profile overrides.
	// Empty when the collection bound to nothing, which is when the flat
	// default governs.
	Profile string
	// Validity is the matched profile's declared window, nil when the profile
	// (or the default point) bounds nothing. ResolveGovernance carries it
	// unfiltered — the as-declared view — while ResolveGovernanceAt applies it.
	Validity *graph.Validity
	// Fallback records a binding that did NOT govern because the resolution
	// instant fell outside its window, and what governs in its place. nil when
	// every declared binding was in force (the ordinary case) and for the
	// as-declared view, which applies no window at all. A caller reports it:
	// governance changing on a date has to be visible, never silent.
	Fallback *GovernanceFallback
}

// GovernanceFallback is one profile that stopped governing because the run's
// instant sits outside its declared window, together with the profile that
// governs instead.
type GovernanceFallback struct {
	// Profile is the profile whose window excluded the instant.
	Profile string
	// Expired distinguishes the two boundaries: true when the instant is at or
	// after valid_to, false when it is before valid_from.
	Expired bool
	// Boundary is the bound that excluded it — valid_to when Expired, else
	// valid_from.
	Boundary time.Time
	// Governing names the profile that governs in its place, empty when
	// resolution fell all the way through to the project's default point.
	Governing string
}

// String renders the transition as the note a run, a check or a retrieval
// answer prints — it names the rule that stopped applying, the date it stopped,
// and what took over, because a governance change nobody is told about is
// indistinguishable from a bug.
func (f *GovernanceFallback) String() string {
	if f == nil {
		return ""
	}
	governing := "the project default"
	if f.Governing != "" {
		governing = fmt.Sprintf("profile %q", f.Governing)
	}
	if f.Expired {
		return fmt.Sprintf("profile %q expired %s; governing with %s",
			f.Profile, FormatValidityBound(&f.Boundary), governing)
	}
	return fmt.Sprintf("profile %q is not in force until %s; governing with %s",
		f.Profile, FormatValidityBound(&f.Boundary), governing)
}

// FormatValidityBound renders a window bound as a bare date when it falls on
// midnight UTC (the form a recipe usually authors), otherwise the full RFC3339
// instant. A nil bound is open on that side and renders empty.
func FormatValidityBound(t *time.Time) string {
	if t == nil {
		return ""
	}
	u := t.UTC()
	if u.Hour() == 0 && u.Minute() == 0 && u.Second() == 0 && u.Nanosecond() == 0 {
		return u.Format("2006-01-02")
	}
	return u.Format(time.RFC3339)
}

// Ref renders the resolved governance back as the point it names, so a caller
// carrying coordinates (a push, a graph write) carries what actually governed
// rather than what the recipe declared. A governance that fell through to the
// default point renders as the zero ref, which puts no coordinates on the wire.
func (rc *ResolvedGovernance) Ref() ChannelRef {
	if rc == nil {
		return ChannelRef{}
	}
	return ChannelRef{Profile: rc.Profile, Channel: rc.Channel}
}

// ActiveAt reports whether this governance is in force at the given instant. A
// governance with no validity window is always in force.
func (rc *ResolvedGovernance) ActiveAt(at time.Time) bool {
	return rc.Validity.Matches(graph.ScopeAt(at))
}

// GovernancePoint names the place in the context space to resolve governance
// for, and the instant to resolve it at. It is the argument of the one
// resolution function every caller goes through (ResolveGovernanceFor), so a
// run, a check, a retrieval answer and a push cannot disagree about what
// governs a file.
type GovernancePoint struct {
	// Profile names a profile directly, with no channel — the point a caller
	// holds when it has a profile and no content location (an ad-hoc run, a
	// question asked of one product's governance). It outranks Collection and
	// Path, which name a location instead, and a name the recipe does not
	// declare is an error rather than a fall-through: a caller that asked about
	// a specific profile is not served by the project default.
	Profile string
	// Collection names a content collection. Used when Path is empty; an
	// unknown name resolves the project's default point.
	Collection string
	// Path is a project-relative, slash-separated file path. An item that claims
	// it wins over Collection, because a file is the finest declared point: that
	// item may carry its own `channel:`. A path no item claims falls back to
	// Collection, then to the project's default point.
	Path string
	// At is the instant to resolve at — the run's wall clock. A profile whose
	// window excludes it does not govern, and resolution falls through to the
	// next binding as if it were absent. The zero value is the AS-DECLARED
	// view: every declared binding governs and carries its window unapplied,
	// which is what a surface reporting the recipe wants and what no run does.
	At time.Time
}

// ResolveGovernanceFor resolves the governance in force at one point, at one
// instant — the single resolution every production caller uses.
//
// Resolution walks the declared bindings from the finest to the coarsest: a
// content item's own `channel:`, then its collection's, then the project's
// default point. A binding whose profile is outside its validity window at
// pt.At is skipped exactly as if the recipe did not declare it, and the
// transition is recorded on the result's Fallback so the caller can say so.
//
// The error is a channel reference that does not resolve; validation runs the
// same resolution for every collection and every item at load, so a project
// that loaded cleanly has no failure left here.
func (p *KapiProject) ResolveGovernanceFor(pt GovernancePoint) (*ResolvedGovernance, error) {
	ladder, err := p.governanceLadder(pt)
	if err != nil {
		return nil, err
	}
	var skipped *GovernanceFallback
	for _, ref := range ladder {
		rc := p.governanceAt(ref)
		if pt.At.IsZero() || rc.ActiveAt(pt.At) {
			if skipped != nil {
				skipped.Governing = rc.Profile
				rc.Fallback = skipped
			}
			return rc, nil
		}
		if skipped == nil {
			skipped = newGovernanceFallback(rc, pt.At)
		}
	}
	rc := p.governanceAt(ChannelRef{})
	rc.Fallback = skipped
	return rc, nil
}

// ResolveGovernance returns the governance the recipe declares for the named
// content collection, AS DECLARED: validity windows are carried on the result,
// not applied. It is the view a surface reporting the recipe takes;
// ResolveGovernanceFor with a non-zero At is the view a run takes.
//
// An empty or unknown collection name resolves the project's default point, so
// a caller holding a path no collection claims — an ad-hoc file, a pattern that
// matched nothing — keeps the project's voice rather than none.
func (p *KapiProject) ResolveGovernance(collection string) (*ResolvedGovernance, error) {
	return p.ResolveGovernanceFor(GovernancePoint{Collection: collection})
}

// ResolveGovernanceForPath returns the governance the recipe declares over one
// file, resolving the point the file sits at rather than its collection's as a
// whole: a per-item `channel:` override wins over the collection's, so one file
// in a docs collection can ship on a different channel, or under a different
// profile, than its neighbours.
//
// relPath is project-relative and slash-separated; matching uses the same
// first-match-wins glob walk as CollectionForPath. A path no item claims
// resolves the project's default point. Windows are carried, not applied —
// ResolveGovernanceFor with a non-zero At applies them.
func (p *KapiProject) ResolveGovernanceForPath(relPath string) (*ResolvedGovernance, error) {
	return p.ResolveGovernanceFor(GovernancePoint{Path: relPath})
}

// governanceLadder returns the channel references that could govern a point,
// finest first, with duplicates collapsed: an item that repeats its
// collection's channel names one binding, not two.
func (p *KapiProject) governanceLadder(pt GovernancePoint) ([]ChannelRef, error) {
	if pt.Profile != "" {
		if _, ok := p.Profiles[pt.Profile]; !ok {
			return nil, fmt.Errorf("profile %q is not declared by this project (declared: %s)",
				pt.Profile, p.declaredProfiles())
		}
		// A profile names one rung and no channel: there is no location under it
		// to refine the answer with.
		return []ChannelRef{{Profile: pt.Profile}}, nil
	}
	declared, subject, err := p.declaredChannelsFor(pt)
	if err != nil {
		return nil, err
	}
	var ladder []ChannelRef
	for _, ch := range declared {
		if ch == "" {
			continue
		}
		ref, rerr := p.ResolveChannel(ch)
		if rerr != nil {
			return nil, fmt.Errorf("%s: %w", subject, rerr)
		}
		if len(ladder) > 0 && ladder[len(ladder)-1] == ref {
			continue
		}
		ladder = append(ladder, ref)
	}
	return ladder, nil
}

// declaredChannelsFor returns the `channel:` references a point declares, finest
// first, together with the recipe subject to name in an error. A path is matched
// against every item with the same first-match-wins glob walk as
// CollectionForPath; a collection is looked up by name, and answers when no item
// claimed the path.
func (p *KapiProject) declaredChannelsFor(pt GovernancePoint) ([]string, string, error) {
	if pt.Path != "" {
		if item, i, ok := p.ItemForPath(pt.Path); ok {
			coll := &p.Collections[i]
			return []string{item.Channel, coll.Channel}, collectionSubject(i, coll.Name), nil
		}
		// A path nothing claims falls back to the collection the caller named,
		// if any: an input outside the declared globs still belongs to the run
		// that was scoped to that collection.
	}
	if pt.Collection == "" {
		return nil, "", nil
	}
	for i := range p.Collections {
		coll := &p.Collections[i]
		if coll.Name != pt.Collection {
			continue
		}
		return []string{coll.Channel}, collectionSubject(i, coll.Name), nil
	}
	return nil, "", nil
}

// newGovernanceFallback describes why a binding did not govern at an instant:
// which boundary of its window excluded it, and when.
func newGovernanceFallback(rc *ResolvedGovernance, at time.Time) *GovernanceFallback {
	f := &GovernanceFallback{Profile: rc.Profile}
	if rc.Validity == nil {
		return f
	}
	if to := rc.Validity.ValidTo; to != nil && !at.Before(*to) {
		f.Expired, f.Boundary = true, *to
		return f
	}
	if from := rc.Validity.ValidFrom; from != nil {
		f.Boundary = *from
	}
	return f
}

// governanceAt resolves the governance at one point.
func (p *KapiProject) governanceAt(ref ChannelRef) *ResolvedGovernance {
	rc := &ResolvedGovernance{
		Channel:    ref.Channel,
		Voice:      p.Defaults.Voice,
		VoiceField: DefaultVoiceField,
	}
	if ref.Profile == "" {
		return rc
	}
	prof, ok := p.Profiles[ref.Profile]
	if !ok {
		return rc
	}
	rc.Profile = ref.Profile
	if prof.Voice != nil {
		rc.Voice, rc.VoiceField = prof.Voice, fmt.Sprintf("profiles.%s.voice", ref.Profile)
	}
	if prof.TermStore != "" {
		rc.TermStore = prof.TermStore
	}
	// Load has already validated the window, so a parse error cannot surface here.
	if v, err := prof.Validity(); err == nil {
		rc.Validity = v
	}
	return rc
}

// ResolveGovernanceAt resolves the governance in force over a collection AT a
// point in time, honouring profile validity: a profile outside its declared
// window does not govern, so resolution falls through to the next binding.
// Plain ResolveGovernance is the as-declared view; this is the as-of view a run
// takes, and it is ResolveGovernanceFor with the collection and the instant.
func (p *KapiProject) ResolveGovernanceAt(collection string, at time.Time) (*ResolvedGovernance, error) {
	return p.ResolveGovernanceFor(GovernancePoint{Collection: collection, At: at})
}

// ProfileWindow is a declared profile and its validity, for surfaces that report
// which governance is in force and until when.
type ProfileWindow struct {
	Name     string
	Validity *graph.Validity
}

// ProfileWindows returns every profile that declares a validity window, in name
// order. Profiles that bound nothing are omitted — there is no window to report.
func (p *KapiProject) ProfileWindows() []ProfileWindow {
	var out []ProfileWindow
	for _, name := range sortedKeys(p.Profiles) {
		v, err := p.Profiles[name].Validity()
		if err != nil || v == nil {
			continue
		}
		out = append(out, ProfileWindow{Name: name, Validity: v})
	}
	return out
}

// HasContextSpace reports whether the recipe governs content through the
// context space at all — it declares profiles. False for every recipe that
// never opted in, which is what lets a caller skip the whole mechanism.
func (p *KapiProject) HasContextSpace() bool {
	return len(p.Profiles) > 0
}

// BindsTermsByProfile reports whether any profile binds a VOCABULARY, as
// opposed to a voice.
//
// The distinction is a venue one. A collection's channel and the voice it
// selects are carried to a connected server by the context content type, so
// both venues resolve the same voice for the same content. A profile's
// `termstore:` is a path into the local project — a file the recipe points at —
// and there is nothing on the wire for it: the server governs terminology from
// the workspace vocabulary instead. A recipe that binds terms per profile therefore still
// resolves differently depending on where the loop ran, and that is what the
// remaining warning is about.
func (p *KapiProject) BindsTermsByProfile() bool {
	for _, name := range sortedKeys(p.Profiles) {
		if p.Profiles[name].TermStore != "" {
			return true
		}
	}
	return false
}

// ItemForPath returns the content item that claims relPath, a project-relative
// slash-separated path: the first item in recipe order, across collections,
// whose pattern matches it, with its collection's base and languages folded in
// (EffectiveItems). The index of that collection comes back beside it; ok is
// false when no item claims the path.
//
// This is the one path-to-item rule. ProjectContext.ResolveContent applies it
// when it expands the recipe into files, and every lookup that starts from a
// path (governance, format, target, reader configuration) asks here, so a file
// is claimed by the same item whichever direction the question is asked from.
func (p *KapiProject) ItemForPath(relPath string) (item ContentItem, collIdx int, ok bool) {
	for i := range p.Collections {
		for _, candidate := range p.Collections[i].EffectiveItems() {
			if candidate.Path == "" || !MatchGlob(candidate.Path, relPath) {
				continue
			}
			return candidate, i, true
		}
	}
	return ContentItem{}, -1, false
}

// CollectionForPath returns the name of the content collection whose item
// claims relPath — a project-relative, slash-separated path — so a run can
// resolve the governance for the file it is about to process. The item is the
// one ItemForPath names. Returns "" when nothing matches or the claiming item
// is an unnamed bare entry: both mean "the project's default point governs
// this file".
func (p *KapiProject) CollectionForPath(relPath string) string {
	if _, i, ok := p.ItemForPath(relPath); ok {
		return p.Collections[i].Name
	}
	return ""
}

// ---------------------------------------------------------------------------
// validation
// ---------------------------------------------------------------------------

// validateContextSpace checks the context model at load: the declared profiles
// and every collection's channel reference — so an unresolvable recipe fails on
// load rather than at the moment a run has to pick a voice.
func (p *KapiProject) validateContextSpace() error {
	if err := p.validateProfiles(); err != nil {
		return err
	}
	for i := range p.Collections {
		c := &p.Collections[i]
		// A per-file `channel:` override resolves the same way and must resolve at
		// load too, so an undeclared axis fails on load rather than when a run
		// reaches that one file.
		for j := range c.Content {
			if c.Content[j].Channel == "" {
				continue
			}
			if _, err := p.ResolveChannel(c.Content[j].Channel); err != nil {
				return fmt.Errorf("%s: content[%d]: %w", collectionSubject(i, c.Name), j, err)
			}
		}
		if c.Channel == "" {
			continue
		}
		if c.Name == "" {
			return fmt.Errorf("collections[%d]: channel requires a named collection (add `name:`)", i)
		}
		if _, err := p.ResolveChannel(c.Channel); err != nil {
			return fmt.Errorf("%s: %w", collectionSubject(i, c.Name), err)
		}
	}
	return nil
}

// validateProfiles checks every profile: a slug-shaped name, slug-shaped
// channels declared once each, a voice selecting exactly one source, and
// well-formed concept references.
func (p *KapiProject) validateProfiles() error {
	for _, name := range sortedKeys(p.Profiles) {
		prof := p.Profiles[name]
		if !slugPattern.MatchString(name) {
			return fmt.Errorf("profiles: name %q is not a slug (%s)", name, slugRule)
		}
		if prof.Concept != "" && !conceptPattern.MatchString(prof.Concept) {
			return fmt.Errorf("profiles.%s: invalid concept reference %q (want a concept id, e.g. term:9a1c0f42b7)", name, prof.Concept)
		}
		seen := make(map[string]bool, len(prof.Channels))
		for i, ch := range prof.Channels {
			switch {
			case ch.ID == "":
				return fmt.Errorf("profiles.%s.channels[%d]: a channel needs an id", name, i)
			case !slugPattern.MatchString(ch.ID):
				return fmt.Errorf("profiles.%s: channel %q is not a slug (%s)", name, ch.ID, slugRule)
			case seen[ch.ID]:
				return fmt.Errorf("profiles.%s: channel %q is declared twice", name, ch.ID)
			case ch.Concept != "" && !conceptPattern.MatchString(ch.Concept):
				return fmt.Errorf("profiles.%s: channel %q has an invalid concept reference %q (want a concept id, e.g. term:9a1c0f42b7)",
					name, ch.ID, ch.Concept)
			}
			seen[ch.ID] = true
		}
		if err := prof.Voice.validate(fmt.Sprintf("profiles.%s.voice", name)); err != nil {
			return err
		}
		if _, err := prof.Validity(); err != nil {
			return fmt.Errorf("profiles.%s: %w", name, err)
		}
	}
	return nil
}

// declaresChannel reports whether a profile declares the given channel.
func declaresChannel(declared []Channel, id string) bool {
	for _, c := range declared {
		if c.ID == id {
			return true
		}
	}
	return false
}

// renderChannels renders one profile's channels for an error message, in the
// order the recipe declares them.
func renderChannels(declared []Channel) string {
	if len(declared) == 0 {
		return "none"
	}
	ids := make([]string, 0, len(declared))
	for _, c := range declared {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, ", ")
}

// declaredProfiles renders the declared profile names for an error message,
// sorted so the message is the same on every load.
func (p *KapiProject) declaredProfiles() string {
	if len(p.Profiles) == 0 {
		return "none"
	}
	return strings.Join(sortedKeys(p.Profiles), ", ")
}

// declaredChannels renders every channel any profile declares, qualified, for
// the error that rejects a bare reference nothing owns.
func (p *KapiProject) declaredChannels() string {
	var all []string
	for _, name := range sortedKeys(p.Profiles) {
		for _, ch := range p.Profiles[name].Channels {
			all = append(all, name+"/"+ch.ID)
		}
	}
	if len(all) == 0 {
		return "none"
	}
	sort.Strings(all)
	return strings.Join(all, ", ")
}

// collectionSubject names a collection by recipe location and name, so an error
// about its channel points at the entry to edit.
func collectionSubject(i int, name string) string {
	return fmt.Sprintf("collections[%d]: collection %q", i, name)
}
