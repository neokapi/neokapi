package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// The point map lists the default context point and each profile/channel pair,
// with governance resolved by ResolveGovernanceFor. A project without profiles
// still has one row for its default point.

// ProjectPointDTO is one point, and what governs content sitting there.
type ProjectPointDTO struct {
	// Ref addresses the point the way a collection names it: `profile/channel`,
	// a bare profile, or empty for the project's own point.
	Ref string `json:"ref"`
	// Label names it for a reader.
	Label       string            `json:"label"`
	Profile     string            `json:"profile,omitempty"`
	Channel     string            `json:"channel,omitempty"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Default is true for the project's own point.
	Default bool `json:"default"`
	// Collections sitting exactly here.
	Collections []string `json:"collections"`
	// Voice is the profile in force, by name.
	Voice string `json:"voice,omitempty"`
	// VoiceField is the recipe key that bound it.
	VoiceField string            `json:"voice_field,omitempty"`
	TermStore  string            `json:"termstore,omitempty"`
	Validity   *VoiceValidityDTO `json:"validity,omitempty"`
	Fallback   *VoiceFallbackDTO `json:"fallback,omitempty"`
}

// ProjectPointsResult is the point map.
type ProjectPointsResult struct {
	At     string            `json:"at"`
	Points []ProjectPointDTO `json:"points"`
	Notes  []string          `json:"notes,omitempty"`
}

// ProjectPoints lists every coordinate point the open recipe declares, with the
// collections at each and the voice governing there.
//
// The project's own point leads, then each declared profile's channels in
// recipe order. A collection is listed at the point it actually resolves to, so
// one whose profile's window has closed appears where it is governed rather
// than where it was written.
func (a *App) ProjectPoints(tabID string) (*ProjectPointsResult, error) {
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

	out := &ProjectPointsResult{At: at.Format(time.RFC3339), Points: []ProjectPointDTO{}}

	store, release, serr := engine.ProjectVoiceStore(ctx, root)
	if serr != nil {
		out.Notes = append(out.Notes, fmt.Sprintf("voice store: %v", serr))
	}
	defer release()

	for _, point := range host.DeclaredPoints(proj, at) {
		row, err := a.projectPoint(ctx, proj, root, store, point, at)
		if err != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("%s: %v", host.PointLabel(point.Ref), err))
			continue
		}
		out.Points = append(out.Points, row)
	}
	return out, nil
}

// projectPoint resolves what governs at one point.
func (a *App) projectPoint(
	ctx context.Context,
	proj *project.KapiProject,
	root string,
	store coreprofile.Store,
	point host.DeclaredPoint,
	at time.Time,
) (ProjectPointDTO, error) {
	ref := point.Ref
	pt := project.GovernancePoint{Profile: ref.Profile, At: at}
	declared, err := proj.ResolveGovernanceFor(project.GovernancePoint{Profile: ref.Profile})
	if err != nil {
		return ProjectPointDTO{}, err
	}

	row := ProjectPointDTO{
		Ref:         ref.String(),
		Label:       host.PointLabel(ref),
		Profile:     ref.Profile,
		Channel:     ref.Channel,
		Default:     ref.Profile == "",
		Coordinates: point.Coordinates,
		Collections: point.Collections,
		VoiceField:  declared.VoiceField,
		TermStore:   declared.TermStore,
		Validity:    validityDTO(declared.Validity, at),
	}
	if row.Collections == nil {
		row.Collections = []string{}
	}

	profile, rc, _, found, verr := a.hostEngine().LoadCollectionVoice(
		ctx, proj, root, host.VoiceResolveOptions{Store: store, Point: pt},
	)
	if verr != nil {
		return ProjectPointDTO{}, verr
	}
	if rc != nil {
		row.Fallback = fallbackDTO(rc.Fallback)
		if rc.Fallback != nil {
			row.VoiceField = rc.VoiceField
		}
	}
	if found && profile != nil {
		row.Voice = profile.Name
	}
	return row, nil
}

