package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/neokapi/neokapi/bowrain/storage"
	bvoice "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/bowrain/voicewrite"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// VoiceProfileResponse is a voice profile as the API returns it: the profile
// and its revision, which an update names to say what it was based on.
type VoiceProfileResponse struct {
	*coreprofile.VoiceProfile
	Revision string `json:"revision,omitempty"`
}

// voiceRevisions is a voice store that keeps a revision per profile.
type voiceRevisions interface {
	Revisions(ctx context.Context, ids []string) (map[string]string, error)
}

// withVoiceRevisions pairs each profile with its revision.
func (s *Server) withVoiceRevisions(ctx context.Context, profiles ...*coreprofile.VoiceProfile) ([]*VoiceProfileResponse, error) {
	out := make([]*VoiceProfileResponse, len(profiles))
	for i, p := range profiles {
		out[i] = &VoiceProfileResponse{VoiceProfile: p}
	}
	store, ok := s.VoiceStore.(voiceRevisions)
	if !ok || len(profiles) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if p != nil {
			ids = append(ids, p.ID)
		}
	}
	revs, err := store.Revisions(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range out {
		if r.VoiceProfile != nil {
			r.Revision = revs[r.ID]
		}
	}
	return out, nil
}

// withVoiceRevision is withVoiceRevisions for one profile.
func (s *Server) withVoiceRevision(ctx context.Context, p *coreprofile.VoiceProfile) (*VoiceProfileResponse, error) {
	if p == nil {
		return nil, nil
	}
	out, err := s.withVoiceRevisions(ctx, p)
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

// errVoiceProfileGone is a write based on a profile that no longer exists.
var errVoiceProfileGone = errors.New("the voice profile was removed or renamed after it was read")

// staleVoiceProfile is an update refused because the profile changed after it
// was read; ID names the profile so the refusal can carry it as it stands.
type staleVoiceProfile struct{ ID string }

func (e *staleVoiceProfile) Error() string { return storage.ErrStale.Error() }
func (e *staleVoiceProfile) Unwrap() error { return storage.ErrStale }

// voiceStale names the profile a stale update was refused for.
func voiceStale(err error, id string) error {
	if errors.Is(err, storage.ErrStale) {
		return &staleVoiceProfile{ID: id}
	}
	return err
}

// refuseVoiceWrite answers a voice profile write the store refused, or
// reports any other error as the server's.
func (s *Server) refuseVoiceWrite(c *echo.Context, err error) error {
	var stale *staleVoiceProfile
	switch {
	case errors.Is(err, voicewrite.ErrNoBase):
		return refuseUnversioned(c, "voice profiles")
	case errors.As(err, &stale):
		current, gerr := s.VoiceStore.GetProfile(c.Request().Context(), stale.ID)
		if gerr != nil {
			return refuseGone(c, "voice profile")
		}
		resp, rerr := s.withVoiceRevision(c.Request().Context(), current)
		if rerr != nil {
			return serverErr(c, rerr)
		}
		return refuseChanged(c, "voice_profile_changed", "voice profile", resp)
	case errors.Is(err, bvoice.ErrProfileNotFound), errors.Is(err, errVoiceProfileGone):
		return refuseGone(c, "voice profile")
	case errors.Is(err, storage.ErrBusy):
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "another save of this voice profile is still in progress; try again"})
	case err != nil:
		return serverErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
