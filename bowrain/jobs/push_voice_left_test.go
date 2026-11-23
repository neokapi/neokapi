package jobs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	voicestore "github.com/neokapi/neokapi/bowrain/voice"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
)

func voiceEntries(t *testing.T, description string) json.RawMessage {
	t.Helper()
	voice, err := json.Marshal(&coreprofile.VoiceProfile{
		Name: "Acme Voice", Description: description, Tone: coreprofile.ToneProfile{Formality: "neutral"},
	})
	require.NoError(t, err)
	raw, err := json.Marshal([]*pb.SyncContextEntry{
		contextEntry("docs", map[string]string{"product": "kapi"}, "docs", "Acme Voice", voice),
	})
	require.NoError(t, err)
	return raw
}

// A push from a project whose voice already lives in the workspace leaves the
// profile as the workspace holds it, and its status names the profile, so
// the person who pushed learns where it is edited.
func TestPush_LeavesTheWorkspacesVoiceAndSaysSo(t *testing.T) {
	e := newPartsEnv(t)
	ctx := t.Context()
	require.NoError(t, e.push(t, "job-first", map[string]any{"contexts": voiceEntries(t, "first")}))
	require.NoError(t, e.push(t, "job-changed", map[string]any{"contexts": voiceEntries(t, "changed in the repository")}))

	voice := e.deps.VoiceStore.(*voicestore.PostgresVoiceStore)
	profiles, err := voice.ListProfiles(ctx, partsWorkspace)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "first", profiles[0].Description, "the workspace's profile is left as it is")

	raw, err := e.deps.JobStore.PushParts(ctx, "push-job-changed")
	require.NoError(t, err)
	var parts venue.PushParts
	require.NoError(t, json.Unmarshal([]byte(raw), &parts))
	assert.Equal(t, []string{"Acme Voice"}, parts.VoiceLeft)
}
