package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/apierror"
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
		Tone: coreprofile.ToneProfile{Formality: "neutral"},
	}
	require.NoError(t, vs.CreateProfile(t.Context(), profile))
	return h, profile
}

// pushedVoice is the contexts part of a push carrying profile, naming base.
func pushedVoice(t *testing.T, name, base string, profile *coreprofile.VoiceProfile) string {
	t.Helper()
	raw, err := json.Marshal(profile)
	require.NoError(t, err)
	out, err := json.Marshal([]*pb.SyncContextEntry{{
		Name: "docs", Owner: "recipe", ContentHash: "hash-docs",
		VoiceProfile: name, VoiceProfileJson: raw, VoiceProfileRevision: base,
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

// A push that would change a workspace voice profile is refused at the
// commit, before anything is queued, with the remedy for its case: a push
// that names no revision is told to update kapi-bowrain, then pull; one that
// names an older revision is told to pull.
func TestSyncPushCommit_AVoiceChangeNamesTheRevisionItPulled(t *testing.T) {
	h, profile := voicePushHarness(t)
	formal := *profile
	formal.Tone.Formality = "formal"
	current := voiceRevisionOf(t, h.srv, profile.ID)

	rec := h.commit(t, pushedVoice(t, "Acme Voice", "", &formal), "", h.currentRef(t))
	require.Equal(t, http.StatusPreconditionRequired, rec.Code, rec.Body.String())
	code, message := refusalCode(t, rec.Body.Bytes())
	assert.Equal(t, apierror.CodeVoiceProfileUnversioned, code)
	assert.Contains(t, message, "`kapi plugin update bowrain`, run `kapi pull`, then push again")

	rec = h.commit(t, pushedVoice(t, "Acme Voice", "an-older-revision", &formal), "", h.currentRef(t))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	code, message = refusalCode(t, rec.Body.Bytes())
	assert.Equal(t, apierror.CodeVoiceProfileMoved, code)
	assert.Contains(t, message, "Run `kapi pull`, then push again")
	assert.NotContains(t, message, "plugin update", "a current plugin is told only to pull")

	// The current revision, an unchanged profile without one, and a new
	// profile without one all pass the voice check.
	for name, contexts := range map[string]string{
		"current revision": pushedVoice(t, "Acme Voice", current, &formal),
		"unchanged":        pushedVoice(t, "acme voice", "", profile),
		"new profile":      pushedVoice(t, "Other Voice", "", &coreprofile.VoiceProfile{Name: "Other Voice"}),
	} {
		rec := h.commit(t, contexts, "", h.currentRef(t))
		code, _ := refusalCode(t, rec.Body.Bytes())
		assert.NotContains(t, []string{apierror.CodeVoiceProfileUnversioned, apierror.CodeVoiceProfileMoved}, code, name)
	}
}

// A pull reports each bound profile's revision, which a push that changes
// the profile names.
func TestSyncPull_CarriesTheVoiceProfileRevision(t *testing.T) {
	h, profile := voicePushHarness(t)
	ctx := t.Context()
	require.NoError(t, h.store.CreateCollection(ctx, &platstore.Collection{
		ProjectID: h.project.ID, Stream: refTestStream, Name: "docs", Owner: "recipe",
		ConnectorConfig: map[string]string{coreprofile.PropertyProfileID: profile.ID},
	}))
	entries := h.srv.pullContextEntries(ctx, h.project.ID, refTestStream)
	require.Len(t, entries, 1)
	assert.Equal(t, "Acme Voice", entries[0].VoiceProfile)
	assert.Equal(t, voiceRevisionOf(t, h.srv, profile.ID), entries[0].VoiceProfileRevision)
}
