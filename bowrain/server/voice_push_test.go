package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	bvoice "github.com/neokapi/neokapi/bowrain/voice"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
)

// voicePushHarness is a ref harness whose project belongs to a workspace with
// a voice store, holding one profile.
func voicePushHarness(t *testing.T) (*refHarness, *coreprofile.VoiceProfile) {
	t.Helper()
	h := newRefHarness(t)
	h.project.WorkspaceID = "ws-voice"
	require.NoError(t, h.store.UpdateProject(t.Context(), h.project))
	vs, err := bvoice.NewPostgresVoiceStore(pgtest.NewTestDB(t))
	require.NoError(t, err)
	h.srv.VoiceStore = vs
	profile := &coreprofile.VoiceProfile{
		ID: "vp-acme", Scope: "ws-voice", Name: "Acme Voice", Version: 1,
		Tone:      coreprofile.ToneProfile{Formality: "neutral"},
		CreatedBy: "user-ana",
	}
	require.NoError(t, vs.CreateProfile(t.Context(), profile))
	return h, profile
}

// pushedVoice is the contexts part of a push carrying profile.
func pushedVoice(t *testing.T, name string, profile *coreprofile.VoiceProfile) string {
	t.Helper()
	raw, err := json.Marshal(profile)
	require.NoError(t, err)
	out, err := json.Marshal([]*pb.SyncContextEntry{{
		Name: "docs", Owner: "recipe", ContentHash: "hash-docs",
		VoiceProfile: name, VoiceProfileJson: raw,
	}})
	require.NoError(t, err)
	return string(out)
}

func refusalCode(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var out map[string]any
	if json.Unmarshal(body, &out) != nil {
		return "", ""
	}
	code, _ = out["error"].(string)
	message, _ = out["message"].(string)
	return code, message
}

// A push from any plugin, released or current, is accepted whatever voice
// content it carries: the commit refuses none of it. The worker creates a
// profile the workspace does not hold and leaves one it holds as it is.
func TestSyncPushCommit_AcceptsAnyPushedVoice(t *testing.T) {
	h, profile := voicePushHarness(t)
	formal := *profile
	formal.Tone.Formality = "formal"
	for name, contexts := range map[string]string{
		"a changed profile":    pushedVoice(t, "Acme Voice", &formal),
		"an unchanged profile": pushedVoice(t, "acme voice", profile),
		"a new profile":        pushedVoice(t, "Other Voice", &coreprofile.VoiceProfile{Name: "Other Voice"}),
	} {
		rec := h.commit(t, contexts, "", h.currentRef(t))
		code, _ := refusalCode(t, rec.Body.Bytes())
		assert.NotContains(t, code, "voice", name)
		assert.Less(t, rec.Code, http.StatusBadRequest, "%s: %s", name, rec.Body.String())
	}
}

// A pull names each collection's voice profile, and carries nothing a client
// does not read: no profile content travels with it.
func TestSyncPull_NamesTheVoiceProfile(t *testing.T) {
	h, profile := voicePushHarness(t)
	ctx := t.Context()
	require.NoError(t, h.store.CreateCollection(ctx, &platstore.Collection{
		ProjectID: h.project.ID, Stream: refTestStream, Name: "docs", Owner: "recipe",
		ConnectorConfig: map[string]string{coreprofile.PropertyProfileID: profile.ID},
	}))
	entries := h.srv.pullContextEntries(ctx, h.project.ID, refTestStream)
	require.Len(t, entries, 1)
	assert.Equal(t, "Acme Voice", entries[0].VoiceProfile)
	assert.Empty(t, entries[0].VoiceProfileJson)
}
