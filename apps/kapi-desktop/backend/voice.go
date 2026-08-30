package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// The voice surface: every point in the project's context space that can carry
// a voice, with the profile resolved at each.
//
// A voice binds at a profile, so the points that can differ are the project's
// own default point and each declared profile. Resolution goes through
// host.LoadCollectionVoice — the same ladder `kapi check`, a run and the
// context explorer take — so this page cannot show a voice the loop would not
// apply.
//
// Each point is resolved twice, and both answers are reported: as declared
// (windows carried) says what the recipe states, and at the governance instant
// (windows applied) says what is in force now. A binding the instant excluded
// is named in Fallback rather than quietly replaced, because governance
// changing on a date has to be visible.

// VoiceValidityDTO is a declared validity window and where it stands at the
// resolution instant.
type VoiceValidityDTO struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// State is "active", "upcoming" or "expired" at the resolution instant.
	State string `json:"state"`
}

// VoiceFallbackDTO records a binding the instant excluded, and what governs in
// its place.
type VoiceFallbackDTO struct {
	Profile string `json:"profile"`
	// Expired is true when the window closed; false when it has not opened.
	Expired  bool   `json:"expired"`
	Boundary string `json:"boundary,omitempty"`
	// Governing is the profile that governs instead, empty for the project's
	// default point.
	Governing string `json:"governing,omitempty"`
	Message   string `json:"message"`
}

// VoiceBindingDTO is the recipe binding that selected a profile.
type VoiceBindingDTO struct {
	// Kind is "profile_file", "pack" or "profile" — the three forms a
	// `voice:` binding takes.
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// VoicePointDTO is one point and the voice in force there.
type VoicePointDTO struct {
	Point ContextPointDTO `json:"point"`
	// Label names the point for a reader: a profile name, or the project's
	// default point.
	Label       string            `json:"label"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Channels the profile declares. The default point declares none.
	Channels []string `json:"channels,omitempty"`
	// Collections resolving to this point at the governance instant.
	Collections []string `json:"collections"`
	// Field is the recipe key the governing binding was declared on.
	Field string `json:"field,omitempty"`
	// Source is where the profile was loaded from: a path, `pack:<name>` or
	// `store:<name>`.
	Source    string           `json:"source,omitempty"`
	Binding   *VoiceBindingDTO `json:"binding,omitempty"`
	TermStore string           `json:"termstore,omitempty"`
	// Profile is the voice as authored, with the overrides unapplied so each
	// locale, channel and persona can be read as itself.
	Profile  *coreprofile.VoiceProfile `json:"profile,omitempty"`
	Guide    string                    `json:"guide,omitempty"`
	Validity *VoiceValidityDTO         `json:"validity,omitempty"`
	Fallback *VoiceFallbackDTO         `json:"fallback,omitempty"`
	Notes    []string                  `json:"notes,omitempty"`
}

// ProjectVoiceResult is every point the recipe declares, with its voice.
type ProjectVoiceResult struct {
	// At is the instant governance was resolved at, RFC3339.
	At     string          `json:"at"`
	Points []VoicePointDTO `json:"points"`
	Notes  []string        `json:"notes,omitempty"`
}

// ProjectVoice resolves the voice profile in force at every point the open
// project declares.
//
// The first point is always the project's own default; a profile follows for
// each the recipe declares, in name order. A point whose profile binds no voice
// of its own reports the one it inherits, with the recipe key that bound it, so
// a reader can see both what governs and where the decision was made.
func (a *App) ProjectVoice(tabID string) (*ProjectVoiceResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("the tab has no project recipe on disk")
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextExplorerTimeout)
	defer cancel()

	proj := op.Project
	root := filepath.Dir(op.Path)
	engine := a.hostEngine()
	at := engine.GovernanceInstant()

	out := &ProjectVoiceResult{At: at.Format(time.RFC3339), Points: []VoicePointDTO{}}

	// A recipe may bind a profile by name, which only the project's voice store
	// can resolve. Without it such a binding reports as not found, which reads
	// as "nothing governs here" when something does.
	store, release, serr := engine.ProjectVoiceStore(ctx, root)
	if serr != nil {
		out.Notes = append(out.Notes, fmt.Sprintf("voice store: %v", serr))
	}
	defer release()

	collections := collectionsByProfile(proj, at)

	points := []project.GovernancePoint{{At: at}}
	for _, name := range sortedKeys(proj.Profiles) {
		points = append(points, project.GovernancePoint{Profile: name, At: at})
	}

	for _, pt := range points {
		row, err := a.voicePoint(ctx, proj, root, store, pt, collections)
		if err != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("%s: %v", pointLabel(pt.Profile), err))
			continue
		}
		out.Points = append(out.Points, row)
	}
	return out, nil
}

// voicePoint resolves one point: what the recipe declares there, what is in
// force at the instant, and the profile itself.
func (a *App) voicePoint(
	ctx context.Context,
	proj *project.KapiProject,
	root string,
	store coreprofile.Store,
	pt project.GovernancePoint,
	collections map[string][]string,
) (VoicePointDTO, error) {
	// The as-declared resolution carries the window unapplied, so the point
	// reports its own binding even when the instant excluded it.
	declared, err := proj.ResolveGovernanceFor(project.GovernancePoint{Profile: pt.Profile})
	if err != nil {
		return VoicePointDTO{}, err
	}

	profile, rc, source, found, verr := a.hostEngine().LoadCollectionVoice(
		ctx, proj, root, host.VoiceResolveOptions{Store: store, Point: pt},
	)
	if verr != nil {
		return VoicePointDTO{}, verr
	}

	ref := project.ChannelRef{Profile: pt.Profile}
	row := VoicePointDTO{
		Point: ContextPointDTO{
			Profile: pt.Profile,
			Ref:     declared.VoiceField,
			Default: pt.Profile == "",
		},
		Label:       pointLabel(pt.Profile),
		Coordinates: project.MergeCoordinates(proj.Defaults.Coordinates, ref.Coordinates(), nil),
		Channels:    profileChannels(proj, pt.Profile),
		Collections: collections[pt.Profile],
		Field:       declared.VoiceField,
		TermStore:   declared.TermStore,
		Binding:     voiceBinding(declared.Voice),
		Validity:    validityDTO(declared.Validity, pt.At),
	}
	if row.Collections == nil {
		row.Collections = []string{}
	}
	if rc != nil {
		row.Fallback = fallbackDTO(rc.Fallback)
		// The window excluded this point's own binding, so what loaded is the
		// profile that governs in its place, on that profile's recipe key.
		if rc.Fallback != nil {
			row.Field = rc.VoiceField
		}
	}
	if found && profile != nil {
		row.Profile = profile
		row.Source = source
		row.Guide = coreprofile.RenderVoiceGuide(profile)
	} else {
		row.Notes = append(row.Notes, "no voice profile binds at this point")
	}
	return row, nil
}

// pointLabel names a point for a reader.
func pointLabel(profile string) string {
	if profile == "" {
		return "project default"
	}
	return profile
}

// profileChannels lists the channels a profile declares, in recipe order.
func profileChannels(proj *project.KapiProject, name string) []string {
	if name == "" {
		return nil
	}
	prof, ok := proj.Profiles[name]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(prof.Channels))
	for _, ch := range prof.Channels {
		if ch.ID == "" {
			continue
		}
		out = append(out, ch.ID)
	}
	return out
}

// collectionsByProfile groups the project's collections by the profile
// governing them at the instant, so each point lists what sits under it.
func collectionsByProfile(proj *project.KapiProject, at time.Time) map[string][]string {
	out := map[string][]string{}
	for _, coll := range proj.Collections {
		if coll.Name == "" {
			continue
		}
		rc, err := proj.ResolveGovernanceFor(project.GovernancePoint{Collection: coll.Name, At: at})
		if err != nil {
			continue
		}
		key := rc.Ref().Profile
		out[key] = append(out[key], coll.Name)
	}
	for key := range out {
		sort.Strings(out[key])
	}
	return out
}

// voiceBinding renders the recipe binding that selected a profile.
func voiceBinding(b *project.VoiceBinding) *VoiceBindingDTO {
	if b == nil {
		return nil
	}
	switch {
	case b.ProfileFile != "":
		return &VoiceBindingDTO{Kind: "profile_file", Value: b.ProfileFile}
	case b.Pack != "":
		return &VoiceBindingDTO{Kind: "pack", Value: b.Pack}
	case b.Profile != "":
		return &VoiceBindingDTO{Kind: "profile", Value: b.Profile}
	}
	return nil
}

// validityDTO renders a declared window and where it stands at the instant.
func validityDTO(v *graph.Validity, at time.Time) *VoiceValidityDTO {
	if v == nil {
		return nil
	}
	out := &VoiceValidityDTO{State: "active"}
	if v.ValidFrom != nil {
		out.From = v.ValidFrom.Format(time.RFC3339)
		if at.Before(*v.ValidFrom) {
			out.State = "upcoming"
		}
	}
	if v.ValidTo != nil {
		out.To = v.ValidTo.Format(time.RFC3339)
		if !at.Before(*v.ValidTo) {
			out.State = "expired"
		}
	}
	return out
}

// fallbackDTO renders a skipped binding and what governs instead.
func fallbackDTO(f *project.GovernanceFallback) *VoiceFallbackDTO {
	if f == nil {
		return nil
	}
	out := &VoiceFallbackDTO{
		Profile:   f.Profile,
		Expired:   f.Expired,
		Governing: f.Governing,
		Message:   f.String(),
	}
	if !f.Boundary.IsZero() {
		out.Boundary = f.Boundary.Format(time.RFC3339)
	}
	return out
}
