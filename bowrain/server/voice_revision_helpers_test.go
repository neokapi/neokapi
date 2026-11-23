package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// voiceRevisionOf is a profile's current revision, the base a client that
// read it names when it writes.
func voiceRevisionOf(t *testing.T, s *Server, id string) string {
	t.Helper()
	store, ok := s.VoiceStore.(voiceRevisions)
	require.True(t, ok, "the voice store keeps revisions")
	revs, err := store.Revisions(t.Context(), []string{id})
	require.NoError(t, err)
	return revs[id]
}

// voiceRevisionNamed is the revision of the workspace profile named name, or
// empty when there is none.
func voiceRevisionNamed(t *testing.T, s *Server, wsID, name string) string {
	t.Helper()
	profiles, err := s.VoiceStore.ListProfiles(t.Context(), wsID)
	require.NoError(t, err)
	for _, p := range profiles {
		if strings.EqualFold(p.Name, name) {
			return voiceRevisionOf(t, s, p.ID)
		}
	}
	return ""
}

// withBaseRevision adds base_revision to a JSON body that names none, as a
// client that read the profile at that revision sends it.
func withBaseRevision(t *testing.T, body, base string) string {
	t.Helper()
	if base == "" || strings.Contains(body, `"base_revision"`) {
		return body
	}
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &fields))
	fields["base_revision"] = base
	out, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(out)
}

// updateVoiceProfileAt writes p through the store on its current revision.
func updateVoiceProfileAt(ctx context.Context, t *testing.T, s *Server, p *coreprofile.VoiceProfile) error {
	t.Helper()
	store, ok := s.VoiceStore.(interface {
		UpdateProfileAt(ctx context.Context, p *coreprofile.VoiceProfile, base string) error
	})
	require.True(t, ok, "the voice store checks the revision an update names")
	return store.UpdateProfileAt(ctx, p, voiceRevisionOf(t, s, p.ID))
}
