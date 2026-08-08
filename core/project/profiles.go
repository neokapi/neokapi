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

// The two axes of the context space, as they travel on the sync wire. The
// framework names them because both are structural: a profile IS the product
// value and a channel IS the channel value, so neither is a taxonomy a recipe
// gets to invent.
const (
	// ProductAxis carries the profile name.
	ProductAxis = "product"
	// ChannelAxis carries the collection's channel.
	ChannelAxis = "channel"
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

	// Terms is a STANDALONE terms store governing this product's content — a
	// file the recipe points at, resolved relative to the project root. Empty
	// is the ordinary case: the project's own store governs.
	Terms string `yaml:"terms,omitempty" json:"terms,omitempty"`

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

// String renders the ref in the qualified form a recipe writes.
func (r ChannelRef) String() string {
	if r.Profile == "" {
		return r.Channel
	}
	return r.Profile + "/" + r.Channel
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
			return ChannelRef{}, fmt.Errorf("channel %q must name its profile — write %s", ref, strings.Join(owners, " or "))
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
// Voice and Terms are resolved already, so a caller applies them without
// knowing whether they came from a profile or from the project defaults.
// Channel is passed on to profile resolution, which selects the matching
// override inside the voice profile.
type ResolvedGovernance struct {
	// Channel is the collection's channel, empty when it binds to none.
	Channel string
	// Voice is the matched profile's voice, else defaults.voice. nil
	// when neither binds one.
	Voice *VoiceBinding
	// Terms is the matched profile's standalone terms store, as written in the
	// recipe — relative to the project root unless absolute. Empty means the
	// project's own store governs, which is the ordinary case.
	Terms string
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
}

// ActiveAt reports whether this governance is in force at the given instant. A
// governance with no validity window is always in force.
func (rc *ResolvedGovernance) ActiveAt(at time.Time) bool {
	return rc.Validity.Matches(graph.ScopeAt(at))
}

// ResolveGovernance returns the governance in force over the named content
// collection.
//
// An empty or unknown collection name resolves the project's default point, so
// a caller holding a path no collection claims — an ad-hoc file, a pattern that
// matched nothing — keeps the project's voice rather than none. The error is a
// channel reference that does not resolve; validation runs the same resolution
// for every collection at load, so a project that loaded cleanly has no failure
// left here.
func (p *KapiProject) ResolveGovernance(collection string) (*ResolvedGovernance, error) {
	ref := ChannelRef{}
	if collection != "" {
		for i := range p.Collections {
			c := &p.Collections[i]
			if c.Name != collection {
				continue
			}
			r, err := p.ResolveChannel(c.Channel)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", collectionSubject(i, c.Name), err)
			}
			ref = r
			break
		}
	}
	return p.governanceAt(ref), nil
}

// ResolveGovernanceForPath returns the governance in force over one file,
// resolving the point the file sits at rather than the collection's as a whole.
// It honors a per-item `channel:` override where the matching item declares one,
// otherwise the item's collection channel — so one file in a docs collection can
// ship on a different channel, or under a different profile, than its neighbours.
//
// relPath is project-relative and slash-separated; matching uses the same
// first-match-wins glob walk as CollectionForPath. A path no item claims
// resolves the project's default point.
func (p *KapiProject) ResolveGovernanceForPath(relPath string) (*ResolvedGovernance, error) {
	ref, _, err := p.channelRefForPath(relPath)
	if err != nil {
		return nil, err
	}
	return p.governanceAt(ref), nil
}

// channelRefForPath finds the channel binding in force for a file: the matching
// item's own `channel:` when it declares one, otherwise its collection's. It
// returns the resolved ref, the collection name (for error context), and any
// resolution error. A path nothing matches yields the zero ref — the default
// point — and no error.
func (p *KapiProject) channelRefForPath(relPath string) (ChannelRef, string, error) {
	for i := range p.Collections {
		coll := &p.Collections[i]
		for _, item := range coll.EffectiveItems() {
			if item.Path == "" {
				continue
			}
			if !MatchGlob(item.Path, relPath) {
				continue
			}
			chosen := coll.Channel
			if item.Channel != "" {
				chosen = item.Channel
			}
			ref, err := p.ResolveChannel(chosen)
			if err != nil {
				return ChannelRef{}, coll.Name, fmt.Errorf("%s: %w", collectionSubject(i, coll.Name), err)
			}
			return ref, coll.Name, nil
		}
	}
	return ChannelRef{}, "", nil
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
	if prof.Terms != "" {
		rc.Terms = prof.Terms
	}
	// Load has already validated the window, so a parse error cannot surface here.
	if v, err := prof.Validity(); err == nil {
		rc.Validity = v
	}
	return rc
}

// ResolveGovernanceAt resolves the governance in force over a collection AT a
// point in time, honouring profile validity: a profile outside its declared
// window does not govern, so resolution falls back to the project's default
// point. Plain ResolveGovernance is the as-declared view (it carries the window
// unfiltered); this is the as-of view a run takes when it wants an expired
// profile to stop applying rather than merely be flagged.
func (p *KapiProject) ResolveGovernanceAt(collection string, at time.Time) (*ResolvedGovernance, error) {
	rc, err := p.ResolveGovernance(collection)
	if err != nil {
		return nil, err
	}
	if rc.Profile != "" && !rc.ActiveAt(at) {
		return p.governanceAt(ChannelRef{}), nil
	}
	return rc, nil
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
// both venues resolve the same voice for the same content. A profile's `terms:`
// is a path into the local project — a file the recipe points at — and there is
// nothing on the wire for it: the server governs terminology from the workspace
// vocabulary instead. A recipe that binds terms per profile therefore still
// resolves differently depending on where the loop ran, and that is what the
// remaining warning is about.
func (p *KapiProject) BindsTermsByProfile() bool {
	for _, name := range sortedKeys(p.Profiles) {
		if p.Profiles[name].Terms != "" {
			return true
		}
	}
	return false
}

// CollectionForPath returns the name of the content collection whose item
// pattern matches relPath — a project-relative, slash-separated path — so a run
// can resolve the governance for the file it is about to process. Matching uses
// the same glob semantics and first-match-wins order as target resolution
// (MatchGlob over EffectiveItems, core/project/glob.go). Returns "" when
// nothing matches or the first match came from an unnamed bare entry: both mean
// "the project's default point governs this file".
func (p *KapiProject) CollectionForPath(relPath string) string {
	for i := range p.Collections {
		coll := &p.Collections[i]
		for _, item := range coll.EffectiveItems() {
			if item.Path == "" {
				continue
			}
			if MatchGlob(item.Path, relPath) {
				return coll.Name
			}
		}
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
