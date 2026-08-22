package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// min_score is the one governance number a voice profile carries: the ship
// gate and bulk approve-passing exclude a block scoring below it. It has always
// round-tripped on READ (the profile JSON carries it), but the create/update
// request bodies dropped it, so nothing but a hand-written recipe push could
// ever set it and the workspace was pinned to the default bar. These cases pin
// it onto every write surface.

// voiceProfileWrite posts payload to handler as a full-permission caller in
// wsID, optionally with an :id route param, and returns the recorder.
func voiceProfileWrite(t *testing.T, srv *Server, handler echo.HandlerFunc, method, wsID, profileID, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", wsID)
	c.Set("user_id", "u-min-score")
	if profileID != "" {
		c.SetParamNames("id")
		c.SetParamValues(profileID)
	}
	require.NoError(t, handler(c))
	return rec
}

// TestVoiceProfile_MinScoreRoundTripsThroughCreateAndUpdate walks the bar
// through the two write surfaces the profile editor drives: a create carries
// the authored bar, an update raises it, and clearing it (an omitted field)
// returns the profile to the default. ComplianceBar is asserted alongside the
// stored value, because the bar is what the ship gate actually reads.
func TestVoiceProfile_MinScoreRoundTripsThroughCreateAndUpdate(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	ctx := context.Background()
	const wsID = "ws-min-score-rt"

	rec := voiceProfileWrite(t, srv, srv.HandleCreateVoiceProfile, http.MethodPost, wsID, "",
		`{"name":"Acme Voice","tone":{"formality":"neutral"},"min_score":90}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created coreprofile.VoiceProfile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, 90, created.MinScore, "the create request's bar must reach the stored profile")
	assert.Equal(t, 90, created.ComplianceBar())

	stored, err := srv.VoiceStore.GetProfile(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, 90, stored.MinScore, "the bar must survive the store, not just the response")

	rec = voiceProfileWrite(t, srv, srv.HandleUpdateVoiceProfile, http.MethodPut, wsID, created.ID,
		`{"name":"Acme Voice","tone":{"formality":"neutral"},"min_score":95}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	stored, err = srv.VoiceStore.GetProfile(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, 95, stored.MinScore, "an update must be able to raise the bar")

	// An omitted min_score is how the editor clears the bar back to the
	// default — the field is optional on the wire and the handler writes what
	// it binds, so absence must mean "no bar of its own", not "keep the old".
	rec = voiceProfileWrite(t, srv, srv.HandleUpdateVoiceProfile, http.MethodPut, wsID, created.ID,
		`{"name":"Acme Voice","tone":{"formality":"neutral"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	stored, err = srv.VoiceStore.GetProfile(ctx, created.ID)
	require.NoError(t, err)
	assert.Zero(t, stored.MinScore, "an omitted bar clears back to the default")
	assert.Equal(t, coreprofile.DefaultMinScore, stored.ComplianceBar())
}

// TestVoiceProfile_MinScoreOutOfRangeIsRejected pins the 0–100 band on every
// write surface. Without it an out-of-range bar reaches the store, where
// ComplianceBar silently caps or defaults it — a profile that reads back a bar
// nobody authored.
func TestVoiceProfile_MinScoreOutOfRangeIsRejected(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	const wsID = "ws-min-score-range"

	// A valid profile to aim the update and upsert cases at.
	rec := voiceProfileWrite(t, srv, srv.HandleCreateVoiceProfile, http.MethodPost, wsID, "",
		`{"name":"Range Voice","tone":{"formality":"neutral"}}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created coreprofile.VoiceProfile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	cases := []struct {
		name        string
		handler     echo.HandlerFunc
		method      string
		id          string
		profileName string
		minScore    int
		wantCode    int
	}{
		{"create above the band", srv.HandleCreateVoiceProfile, http.MethodPost, "", "Above", 101, http.StatusBadRequest},
		{"create below the band", srv.HandleCreateVoiceProfile, http.MethodPost, "", "Below", -1, http.StatusBadRequest},
		{"create at the top of the band", srv.HandleCreateVoiceProfile, http.MethodPost, "", "Top", 100, http.StatusCreated},
		{"update above the band", srv.HandleUpdateVoiceProfile, http.MethodPut, created.ID, "Range Voice", 101, http.StatusBadRequest},
		{"update at the top of the band", srv.HandleUpdateVoiceProfile, http.MethodPut, created.ID, "Range Voice", 100, http.StatusOK},
		{"upsert above the band", srv.HandleUpsertVoiceProfile, http.MethodPost, "", "Range Voice", 101, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := fmt.Sprintf(`{"name":%q,"tone":{"formality":"neutral"},"min_score":%d}`,
				tc.profileName, tc.minScore)
			rec := voiceProfileWrite(t, srv, tc.handler, tc.method, wsID, tc.id, payload)
			assert.Equal(t, tc.wantCode, rec.Code, rec.Body.String())
			if tc.wantCode == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "min_score must be between 0 and 100")
			}
		})
	}
}

// authoredProfileRequest is a VoiceProfileRequest with EVERY field set to a
// distinguishable non-zero value. `mark` distinguishes one call's values from
// another's so a create and a later update cannot pass by accident.
func authoredProfileRequest(name, mark string, minScore int) VoiceProfileRequest {
	return VoiceProfileRequest{
		Name:        name,
		Description: "described " + mark,
		Tone: coreprofile.ToneProfile{
			Personality: []string{"clear-" + mark}, Formality: "formal",
			Emotion: "warm", Humor: "light", Guidelines: "guide " + mark,
		},
		Style: coreprofile.StyleRules{
			ActiveVoice: true, SentenceLength: "short", PersonPOV: "second",
			Contractions: "never",
			ProhibitedPatterns: []coreprofile.Pattern{
				{Regex: "x" + mark, Description: "no " + mark, Severity: "major"},
			},
			RequiredPatterns: []coreprofile.Pattern{
				{Regex: "y" + mark, Description: "yes " + mark, Severity: "minor"},
			},
		},
		Vocabulary: coreprofile.VocabularyRules{
			PreferredTerms:  []coreprofile.TermRule{{Term: "use-" + mark, Replacement: "utilize"}},
			ForbiddenTerms:  []coreprofile.TermRule{{Term: "bad-" + mark}},
			CompetitorTerms: []coreprofile.TermRule{{Term: "rival-" + mark}},
			Abbreviations:   map[string]string{"abbr": mark},
		},
		Examples: []coreprofile.VoiceExample{
			{Before: "before " + mark, After: "after " + mark, Explanation: "why", Category: "tone"},
		},
		Locales: map[model.LocaleID]coreprofile.LocaleOverride{
			"de": {Formality: "formal", CulturalNotes: "note " + mark},
		},
		Channels: map[string]coreprofile.ChannelOverride{
			"email": {Tone: &coreprofile.ToneProfile{Formality: "formal", Personality: []string{mark}}},
		},
		Personas: map[string]coreprofile.PersonaOverride{
			"jordan": {Avoided: []coreprofile.TermRule{{Term: "synergy-" + mark}}},
		},
		MinScore: minScore,
	}
}

// assertProfileCarriesRequest asserts that every field of req reached the
// stored profile. It walks the request by reflection and pairs each field with
// the same-named field on VoiceProfile, so a field added to the request and
// forgotten in a handler — or dropped by the store's column list, which is what
// happened to min_score — fails here by name rather than going unnoticed.
func assertProfileCarriesRequest(t *testing.T, req VoiceProfileRequest, stored *coreprofile.VoiceProfile) {
	t.Helper()
	rv := reflect.ValueOf(req)
	sv := reflect.ValueOf(*stored)
	for i := range rv.NumField() {
		field := rv.Type().Field(i)
		want := rv.Field(i)
		require.False(t, want.IsZero(),
			"authoredProfileRequest leaves %s at its zero value, so this test cannot see it dropped",
			field.Name)
		got := sv.FieldByName(field.Name)
		require.True(t, got.IsValid(), "VoiceProfile has no field named %s", field.Name)
		assert.Equal(t, want.Interface(), got.Interface(),
			"%s did not survive the write: the handler or the store dropped it", field.Name)
	}
}

// TestVoiceProfile_EveryAuthoredFieldSurvivesTheWrite closes the whole class
// min_score fell into: a field the request declares, the handler forgets (or
// the store's column list omits), and the profile then reads back at its zero
// value with nothing failing anywhere. Every field of VoiceProfileRequest is
// set to a non-zero value, written through create and then through update, and
// compared field-by-field against what the store hands back.
func TestVoiceProfile_EveryAuthoredFieldSurvivesTheWrite(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	ctx := context.Background()
	const wsID = "ws-authored-fields"

	created := authoredProfileRequest("Authored Voice", "one", 77)
	payload, err := json.Marshal(created)
	require.NoError(t, err)
	rec := voiceProfileWrite(t, srv, srv.HandleCreateVoiceProfile, http.MethodPost, wsID, "", string(payload))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var body coreprofile.VoiceProfile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	stored, err := srv.VoiceStore.GetProfile(ctx, body.ID)
	require.NoError(t, err)
	assertProfileCarriesRequest(t, created, stored)

	updated := authoredProfileRequest("Authored Voice", "two", 63)
	payload, err = json.Marshal(updated)
	require.NoError(t, err)
	rec = voiceProfileWrite(t, srv, srv.HandleUpdateVoiceProfile, http.MethodPut, wsID, body.ID, string(payload))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	stored, err = srv.VoiceStore.GetProfile(ctx, body.ID)
	require.NoError(t, err)
	assertProfileCarriesRequest(t, updated, stored)
}

// TestVoiceUpsert_MinScoreIsAuthoredContent proves the push upsert treats the
// bar as authored content on both halves of its contract: a first push carries
// it, and a re-push that changes ONLY the bar is an update with a new version —
// not the "unchanged" no-op it would be if voiceContentOf ignored the field.
func TestVoiceUpsert_MinScoreIsAuthoredContent(t *testing.T) {
	srv := setupVoiceLoopServer(t)
	const wsID = "ws-min-score-upsert"

	rec, res := upsertVoiceProfile(t, srv, wsID, "u-1",
		`{"name":"Push Voice","tone":{"formality":"neutral"},"min_score":85}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.NotNil(t, res.Profile)
	assert.Equal(t, "created", res.Action)
	assert.Equal(t, 85, res.Profile.MinScore, "the pushed bar must reach the created profile")

	_, same := upsertVoiceProfile(t, srv, wsID, "u-1",
		`{"name":"Push Voice","tone":{"formality":"neutral"},"min_score":85}`)
	assert.Equal(t, "unchanged", same.Action, "an identical re-push stays a no-op")

	_, raised := upsertVoiceProfile(t, srv, wsID, "u-1",
		`{"name":"Push Voice","tone":{"formality":"neutral"},"min_score":95}`)
	assert.Equal(t, "updated", raised.Action, "a bar-only change is a content change")
	require.NotNil(t, raised.Profile)
	assert.Equal(t, 95, raised.Profile.MinScore)
	assert.Equal(t, 2, raised.Profile.Version, "the raised bar lands as a new version")
}
