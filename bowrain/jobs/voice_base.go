package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/apierror"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/voicewrite"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// A push that changes a workspace voice profile names the revision of the
// profile it last pulled. The server refuses the change when the push names
// none, or when the profile has moved since; the push commit checks before it
// queues anything, and the worker checks again in the statement that writes.

// VoiceRefusal is a push refused because it would change a voice profile
// without knowing the profile as the server holds it.
type VoiceRefusal struct {
	// Profile is the profile's name.
	Profile string
	// Unversioned is set when the push named no revision. Otherwise the
	// profile changed on the server after the project last saw it.
	Unversioned bool
	// NotPulled is set beside Unversioned when the client sends revisions
	// (venue.CapabilityRevisions): the project has not pulled the profile
	// yet, as on a fresh clone, and a pull is the remedy, not an update.
	NotPulled bool
	// ChangedBy and ChangedAt are who last wrote the profile on the server
	// and when, for a profile that moved. ChangedBy is a member id or a
	// tool name; the commit handler renders it as a name.
	ChangedBy string
	ChangedAt time.Time
}

func (e *VoiceRefusal) Error() string {
	return apierror.MessageFor(e.Code(), e.Details(e.ChangedBy))
}

// Details are the refusal's API error details, with by as the name of who
// changed the profile.
func (e *VoiceRefusal) Details(by string) map[string]any {
	details := map[string]any{"profile": e.Profile}
	if !e.Unversioned && !e.ChangedAt.IsZero() {
		details["by"] = by
		details["at"] = e.ChangedAt.UTC().Format("2006-01-02 15:04 UTC")
	}
	return details
}

// Code is the API error code the refusal is answered with.
func (e *VoiceRefusal) Code() string {
	switch {
	case e.Unversioned && e.NotPulled:
		return apierror.CodeVoiceProfileNotPulled
	case e.Unversioned:
		return apierror.CodeVoiceProfileUnversioned
	}
	return apierror.CodeVoiceProfileMoved
}

// CheckVoiceBases refuses a push whose context entries would change a voice
// profile of the workspace without naming the profile's current revision.
// An entry that creates a profile, or carries one the server already holds as
// pushed, names nothing and needs nothing. sendsRevisions says the client
// sends revisions at all, which tells a project that has not pulled from a
// client too old to say.
func CheckVoiceBases(ctx context.Context, voice coreprofile.Store, workspaceID string, raw json.RawMessage, sendsRevisions bool) error {
	if voice == nil || workspaceID == "" {
		return nil
	}
	entries, err := parseContextEntries(raw)
	if err != nil || len(entries) == 0 {
		return err
	}
	revisions, ok := voice.(interface {
		Revisions(ctx context.Context, ids []string) (map[string]string, error)
	})
	if !ok {
		return nil
	}
	var profiles []*coreprofile.VoiceProfile
	for _, e := range entries {
		if e == nil || e.VoiceProfile == "" || len(e.VoiceProfileJson) == 0 {
			continue
		}
		if profiles == nil {
			if profiles, err = voice.ListProfiles(ctx, workspaceID); err != nil {
				return fmt.Errorf("list voice profiles: %w", err)
			}
		}
		existing := profileNamed(profiles, e.VoiceProfile)
		if existing == nil {
			continue
		}
		pushed, err := decodeVoiceProfile(e.VoiceProfileJson)
		if err != nil {
			return fmt.Errorf("decode voice profile %q: %w", e.VoiceProfile, err)
		}
		if sameAuthoredVoice(existing, pushedOver(existing, pushed)) {
			continue
		}
		base := e.GetVoiceProfileRevision()
		if base == "" {
			return &VoiceRefusal{Profile: existing.Name, Unversioned: true, NotPulled: sendsRevisions}
		}
		revs, err := revisions.Revisions(ctx, []string{existing.ID})
		if err != nil {
			return fmt.Errorf("read voice profile revision: %w", err)
		}
		if revs[existing.ID] != base {
			return &VoiceRefusal{Profile: existing.Name, ChangedBy: existing.UpdatedBy, ChangedAt: existing.UpdatedAt}
		}
	}
	return nil
}

// profileNamed is the profile named name, compared case-insensitively.
func profileNamed(profiles []*coreprofile.VoiceProfile, name string) *coreprofile.VoiceProfile {
	for _, p := range profiles {
		if p != nil && strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

// asVoiceRefusal names the profile a write refused for a missing or stale
// base, so the job's failure says what to do.
func asVoiceRefusal(err error, name string) error {
	switch {
	case errors.Is(err, voicewrite.ErrNoBase):
		return &VoiceRefusal{Profile: name, Unversioned: true}
	case errors.Is(err, storage.ErrStale):
		return &VoiceRefusal{Profile: name}
	}
	return err
}
