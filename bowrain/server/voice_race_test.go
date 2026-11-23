package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/labstack/echo/v5"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"

	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// Concurrent edits of one profile from the version they all read: one lands,
// the rest are told the profile changed, and the version the edit replaced is
// archived. Before the revision, every edit landed under the same version
// number, and the archive kept only one of the versions they replaced.
func TestConcurrentVoiceEditsLandOnceAndArchiveWhatTheyReplace(t *testing.T) {
	s, wsID, _, _ := voiceWriteHarness(t)
	ctx := t.Context()
	profile := &coreprofile.VoiceProfile{ID: "vp-race", Scope: wsID, Name: "Docs", Version: 1}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, profile))
	base := voiceRevisionOf(t, s, profile.ID)

	const writers = 8
	codes := make([]int, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			body := fmt.Sprintf(`{"name":"Docs","description":"edit %d","base_revision":%q}`, i, base)
			c, rec := voiceRequest(s, http.MethodPut, body, wsID, "member-b", "id", profile.ID)
			if err := s.HandleUpdateVoiceProfile(c); err != nil {
				codes[i] = -1
				return
			}
			codes[i] = rec.Code
		})
	}
	wg.Wait()

	landed := 0
	for i, code := range codes {
		switch code {
		case http.StatusOK:
			landed++
		case http.StatusConflict:
		default:
			t.Fatalf("writer %d: %d", i, code)
		}
	}
	assert.Equal(t, 1, landed, "one edit lands on the version they all read")

	stored, err := s.VoiceStore.GetProfile(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, 1+landed, stored.Version)
	versions, err := s.VoiceStore.ListProfileVersions(ctx, profile.ID)
	require.NoError(t, err)
	assert.Len(t, versions, landed, "every version an edit replaced is archived")
}

// A delete names the revision of the profile the person saw: one that names
// none, or an older one, is refused, and the profile stays.
func TestVoiceDeleteIsBasedOnARevision(t *testing.T) {
	s, wsID, _, _ := voiceWriteHarness(t)
	ctx := t.Context()
	profile := &coreprofile.VoiceProfile{ID: "vp-del", Scope: wsID, Name: "Docs", Version: 1}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, profile))
	read := voiceRevisionOf(t, s, profile.ID)
	c, rec := voiceRequest(s, http.MethodPut, `{"name":"Docs","description":"edited"}`, wsID, "member-b", "id", profile.ID)
	require.NoError(t, s.HandleUpdateVoiceProfile(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	del := func(target string) int {
		req := httptest.NewRequest(http.MethodDelete, target, nil)
		rec := httptest.NewRecorder()
		c := s.GetEcho().NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.Set("workspace_id", wsID)
		c.Set("user_id", "member-b")
		c.SetPathValues(echo.PathValues{{Name: "id", Value: profile.ID}})
		require.NoError(t, s.HandleDeleteVoiceProfile(c))
		return rec.Code
	}
	assert.Equal(t, http.StatusPreconditionRequired, del("/"))
	assert.Equal(t, http.StatusConflict, del("/?base_revision="+read), "a delete made over a later edit is refused")
	_, err := s.VoiceStore.GetProfile(ctx, profile.ID)
	require.NoError(t, err, "the profile stays")
	assert.Equal(t, http.StatusNoContent, del("/?base_revision="+voiceRevisionOf(t, s, profile.ID)))
}

// An update based on a revision a later edit replaced is refused, with the
// profile as it now stands, and writes nothing.
func TestVoiceUpdateOnAnOlderRevisionIsRefused(t *testing.T) {
	s, wsID, _, _ := voiceWriteHarness(t)
	ctx := t.Context()
	profile := &coreprofile.VoiceProfile{ID: "vp-stale", Scope: wsID, Name: "Docs", Version: 1}
	require.NoError(t, s.VoiceStore.CreateProfile(ctx, profile))
	read := voiceRevisionOf(t, s, profile.ID)

	put := func(body string) *httptest.ResponseRecorder {
		c, rec := voiceRequest(s, http.MethodPut, body, wsID, "member-b", "id", profile.ID)
		require.NoError(t, s.HandleUpdateVoiceProfile(c))
		return rec
	}
	require.Equal(t, http.StatusOK, put(fmt.Sprintf(`{"name":"Docs","description":"first","base_revision":%q}`, read)).Code)
	rec := put(fmt.Sprintf(`{"name":"Docs","description":"second","base_revision":%q}`, read))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "voice_profile_changed")

	stored, err := s.VoiceStore.GetProfile(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "first", stored.Description, "a refused update writes nothing")
}
