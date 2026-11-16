package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/voicewrite"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
	"github.com/neokapi/neokapi/core/workspace"
)

const voiceWriteKey = "prj_voiceledgeraaaaaaaaa"

// voiceWriteHarness is a server whose content, auth and voice stores share
// one database, as in production, with a project in the workspace whose
// context is bound, and a second member, Bea.
func voiceWriteHarness(t *testing.T) (s *Server, wsID, owner string, proj *platstore.Project) {
	t.Helper()
	s, wsID, owner = newRecheckHarness(t)
	ctx := t.Context()
	require.NoError(t, s.AuthStore.CreateUser(ctx, &platauth.User{ID: "member-b", Email: "bea@rc.test", Name: "Bea"}))
	require.NoError(t, s.AuthStore.AddMember(ctx, wsID, "member-b", platauth.RoleAdmin))

	proj = &platstore.Project{
		Name: "Voice Proj", WorkspaceID: wsID, DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"}, Properties: map[string]string{},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))
	cos, ok := platstore.ContextObjects(s.ContentStore)
	require.True(t, ok)
	_, err := cos.BindContextKey(ctx, proj.ID, voiceWriteKey)
	require.NoError(t, err)
	return s, wsID, owner, proj
}

// voiceRequest is an echo context for a voice route in wsID, signed in as
// userID with every permission.
func voiceRequest(s *Server, method, body, wsID, userID string, params ...string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := s.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", wsID)
	c.Set("user_id", userID)
	if len(params) > 0 {
		names := make([]string, 0, len(params)/2)
		values := make([]string, 0, len(params)/2)
		for i := 0; i+1 < len(params); i += 2 {
			names = append(names, params[i])
			values = append(values, params[i+1])
		}
		c.SetParamNames(names...)
		c.SetParamValues(values...)
	}
	return c, rec
}

// ledgerVoiceEdits reads the voice profile writes the project's ledger
// records, newest first.
func ledgerVoiceEdits(t *testing.T, s *Server, projectID string) []contextop.Record {
	t.Helper()
	ledger, err := s.ContextLedger(t.Context(), projectID, LedgerMember{})
	require.NoError(t, err)
	records, err := ledger.Records(t.Context(), contextop.Filter{Kinds: []contextop.Kind{contextop.KindEdit}})
	require.NoError(t, err)
	var out []contextop.Record
	for _, r := range records {
		if r.Voice != nil {
			out = append(out, r)
		}
	}
	return out
}

// ownerProfile stores a profile the owner wrote before the ledger recorded
// voice writes: it is in the store, and no operation names it.
func ownerProfile(t *testing.T, s *Server, wsID, owner string) *coreprofile.VoiceProfile {
	t.Helper()
	p := &coreprofile.VoiceProfile{ID: id.New(), Name: "Docs", Scope: wsID, CreatedBy: owner}
	require.NoError(t, s.VoiceStore.CreateProfile(t.Context(), p))
	return p
}

// TestVoiceProfileWrites_RecordOneLedgerOperation: every server surface that
// writes a voice profile records the write once on the ledger of the
// workspace's projects, as the member who made it.
func TestVoiceProfileWrites_RecordOneLedgerOperation(t *testing.T) {
	starters, err := packs.List()
	require.NoError(t, err)
	require.NotEmpty(t, starters)

	tests := []struct {
		name string
		// existing says a profile the owner wrote is in the store first.
		existing bool
		// call runs the surface as member-b and returns the written profile's
		// id.
		call     func(t *testing.T, s *Server, wsID string, existing *coreprofile.VoiceProfile) string
		wantNote string
		wantWhat string
	}{
		{
			name: "web create",
			call: func(t *testing.T, s *Server, wsID string, _ *coreprofile.VoiceProfile) string {
				c, rec := voiceRequest(s, http.MethodPost, `{"name":"New Voice"}`, wsID, "member-b")
				require.NoError(t, s.HandleCreateVoiceProfile(c))
				require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
				return decodeJSON[coreprofile.VoiceProfile](t, rec).ID
			},
			wantNote: "created in Bowrain", wantWhat: "created",
		},
		{
			name: "starter pack", call: func(t *testing.T, s *Server, wsID string, _ *coreprofile.VoiceProfile) string {
				c, rec := voiceRequest(s, http.MethodPost, `{"pack":"`+starters[0]+`","name":"From Pack"}`, wsID, "member-b", "ws", "rc")
				require.NoError(t, s.HandleCreateFromStarter(c))
				require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
				return decodeJSON[coreprofile.VoiceProfile](t, rec).ID
			},
			wantNote: "created from the " + starters[0] + " starter pack", wantWhat: "created",
		},
		{
			name: "web edit", existing: true,
			call: func(t *testing.T, s *Server, wsID string, p *coreprofile.VoiceProfile) string {
				c, rec := voiceRequest(s, http.MethodPut, `{"name":"Docs","description":"Plain."}`, wsID, "member-b", "id", p.ID)
				require.NoError(t, s.HandleUpdateVoiceProfile(c))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				return p.ID
			},
			wantNote: "edited in Bowrain", wantWhat: "edited",
		},
		{
			name: "web delete", existing: true,
			call: func(t *testing.T, s *Server, wsID string, p *coreprofile.VoiceProfile) string {
				c, rec := voiceRequest(s, http.MethodDelete, "", wsID, "member-b", "id", p.ID)
				require.NoError(t, s.HandleDeleteVoiceProfile(c))
				require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
				return p.ID
			},
			wantNote: "removed in Bowrain", wantWhat: "removed",
		},
		{
			name: "upsert endpoint", existing: true,
			call: func(t *testing.T, s *Server, wsID string, p *coreprofile.VoiceProfile) string {
				c, rec := voiceRequest(s, http.MethodPost, `{"name":"Docs","description":"Upserted."}`, wsID, "member-b")
				require.NoError(t, s.HandleUpsertVoiceProfile(c))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				return p.ID
			},
			wantNote: "written through the voice profile upsert", wantWhat: "edited",
		},
		{
			name: "context-scan approval",
			call: func(t *testing.T, s *Server, wsID string, _ *coreprofile.VoiceProfile) string {
				wireContextScan(t, s)
				job := &jobs.ContextScanJob{
					ID: id.New(), WorkspaceID: wsID, WorkspaceSlug: "rc",
					Status: jobs.ContextScanStatusCompleted, Request: json.RawMessage(`{"paste_text":"Plain."}`),
				}
				require.NoError(t, s.ContextScanStore.CreateContextScanJob(t.Context(), job))
				require.NoError(t, s.ContextScanStore.UpdateContextScanJobStatus(t.Context(), job.ID, jobs.ContextScanStatusCompleted, ""))
				c, rec := voiceRequest(s, http.MethodPost, `{"profile":{"name":"Scanned Voice"}}`, wsID, "member-b", "ws", "rc", "id", job.ID)
				require.NoError(t, s.HandleApproveContextScan(c))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				return decodeJSON[ContextScanApproveResponse](t, rec).Profile.ID
			},
			wantNote: "approved from a context scan", wantWhat: "created",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, wsID, owner, proj := voiceWriteHarness(t)
			var existing *coreprofile.VoiceProfile
			if tt.existing {
				existing = ownerProfile(t, s, wsID, owner)
			}
			profileID := tt.call(t, s, wsID, existing)

			edits := ledgerVoiceEdits(t, s, proj.ID)
			require.Len(t, edits, 1, "one record of the write")
			op := edits[0]
			assert.Equal(t, profileID, op.Voice.Profile)
			assert.Equal(t, contextop.ActorPerson, op.Actor.Kind)
			assert.Equal(t, "Bea", op.Actor.Name, "the member who wrote it, not the profile's creator")
			assert.Equal(t, tt.wantNote, op.Note)
			assert.Contains(t, op.Subject.Text, "voice profile "+profileID+" "+tt.wantWhat)
			assert.Equal(t, tt.existing, op.Voice.Previous != "", "a write over a stored profile keeps what it replaced")
			assert.Equal(t, tt.wantWhat == "removed", op.Voice.Wrote == "", "a removal leaves nothing to fingerprint")
		})
	}
}

// TestVoiceProfileWrite_UnchangedUpsertRecordsNothing: an upsert of the
// content the store holds writes nothing, so the ledger records nothing.
func TestVoiceProfileWrite_UnchangedUpsertRecordsNothing(t *testing.T) {
	s, wsID, owner, proj := voiceWriteHarness(t)
	ownerProfile(t, s, wsID, owner)
	c, rec := voiceRequest(s, http.MethodPost, `{"name":"Docs"}`, wsID, "member-b")
	require.NoError(t, s.HandleUpsertVoiceProfile(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, voiceUpsertUnchanged, decodeJSON[VoiceProfileUpsertResponse](t, rec).Action)
	assert.Empty(t, ledgerVoiceEdits(t, s, proj.ID))
}

// deckVoiceRuleCard is the project's voice rule card for regex, as reviewer
// reads the deck.
func deckVoiceRuleCard(t *testing.T, s *Server, wsID string, proj *platstore.Project, rv deckReviewer, regex string) contextDeckCard {
	t.Helper()
	src, err := s.contextDeckSource(t.Context(), wsID, "rc", proj, rv.userID)
	require.NoError(t, err)
	cards, err := s.contextItems(t.Context(), src, rv)
	require.NoError(t, err)
	for _, c := range cards {
		if c.Kind == deckKindVoiceRule && c.Pattern != nil && c.Pattern.Regex == regex {
			return c
		}
	}
	require.Failf(t, "no voice rule card", "regex %s", regex)
	return contextDeckCard{}
}

// TestContextDeck_VoiceRuleAuthorIsTheLedgers: the deck names the author of a
// voice rule from the ledger alone. A web edit by Bea is Bea's work, not the
// work of the owner who created the profile; a rule a push from a claude-code
// shell added is agent-authored; a rule no recorded write added names no one.
func TestContextDeck_VoiceRuleAuthorIsTheLedgers(t *testing.T) {
	s, wsID, owner, proj := voiceWriteHarness(t)
	ctx := t.Context()

	profile := ownerProfile(t, s, wsID, owner)
	profile.Style.ProhibitedPatterns = []coreprofile.Pattern{{Regex: `\butilize\b`, Description: "Say use."}}
	require.NoError(t, s.VoiceStore.UpdateProfile(ctx, profile))
	proj.Properties[coreprofile.PropertyProfileID] = profile.ID
	require.NoError(t, s.ContentStore.UpdateProject(ctx, proj))

	// Bea adds a rule in the web editor.
	body := `{"name":"Docs","style":{"prohibited_patterns":[
		{"regex":"\\butilize\\b","description":"Say use."},
		{"regex":"\\bleverage\\b","description":"Say use."}]}}`
	c, rec := voiceRequest(s, http.MethodPut, body, wsID, "member-b", "id", profile.ID)
	require.NoError(t, s.HandleUpdateVoiceProfile(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// A push from a claude-code shell adds a third: the client's operation
	// is in the project's ledger, and the push writes the profile carrying
	// its id, so no second operation is recorded.
	before, err := s.VoiceStore.GetProfile(ctx, profile.ID)
	require.NoError(t, err)
	after := *before
	after.Style.ProhibitedPatterns = append(append([]coreprofile.Pattern{}, before.Style.ProhibitedPatterns...),
		coreprofile.Pattern{Regex: `\bsynergy\b`, Description: "Say what it does."})
	clientOp := putClientVoiceEdit(t, s, proj.ID, before, &after)
	objects, _ := s.contextObjects()
	ledger := &workspaceVoiceLedger{s: s, wsID: wsID, objects: objects,
		put: func(ctx context.Context, projectID, pushedBy string, obj platstore.ContextObject) error {
			return objects.PutContextObjects(ctx, projectID, pushedBy, []platstore.ContextObject{obj})
		}}
	res, err := voicewrite.Apply(ctx, s.VoiceStore, ledger, voicewrite.Write{
		Before: before, After: &after, Actor: contextop.Actor{Kind: contextop.ActorPerson},
		PushedBy: owner, Note: "pushed with `kapi push`", Recorded: clientOp,
	})
	require.NoError(t, err)
	assert.Equal(t, clientOp, res.Op, "the client's operation is the record")
	require.Len(t, ledgerVoiceEdits(t, s, proj.ID), 2, "Bea's edit and the client's, nothing more")

	asOwner := deckReviewer{userID: owner, name: "Owner", perms: platauth.PermAll}
	asBea := deckReviewer{userID: "member-b", name: "Bea", perms: platauth.PermAll}

	byBea := deckVoiceRuleCard(t, s, wsID, proj, asOwner, `\bleverage\b`)
	assert.Equal(t, "Bea", byBea.Author.Name)
	assert.False(t, byBea.Author.AgentAuthored)
	assert.False(t, byBea.OwnWork, "the owner created the profile, and Bea wrote this rule")
	assert.Equal(t, contextop.BasisPeer, byBea.Basis)
	assert.True(t, deckVoiceRuleCard(t, s, wsID, proj, asBea, `\bleverage\b`).OwnWork)

	byAgent := deckVoiceRuleCard(t, s, wsID, proj, asOwner, `\bsynergy\b`)
	assert.True(t, byAgent.Author.AgentAuthored)
	assert.Equal(t, "claude-code", byAgent.Author.Host)
	assert.Equal(t, contextop.BasisAgentAuthored, byAgent.Basis)
	require.NotNil(t, byAgent.Origin)
	assert.Equal(t, clientOp, byAgent.Origin.ID)

	unrecorded := deckVoiceRuleCard(t, s, wsID, proj, asOwner, `\butilize\b`)
	assert.Empty(t, unrecorded.Author.Name, "no recorded write added it, and the store names no author")
	assert.False(t, unrecorded.OwnWork)
}

// putClientVoiceEdit puts in the project's ledger the edit operation a kapi
// client records for a write from before to after in a claude-code shell, as
// a push of its segment does, and returns its id.
func putClientVoiceEdit(t *testing.T, s *Server, projectID string, before, after *coreprofile.VoiceProfile) string {
	t.Helper()
	edit, err := contextop.NewVoiceEdit(before, after)
	require.NoError(t, err)
	log := &capturedLog{}
	rec, err := contextop.NewLedger(log, nil).Append(t.Context(), contextop.Record{
		Project: voiceWriteKey,
		Actor:   contextop.Actor{Kind: contextop.ActorPerson},
		Kind:    contextop.KindEdit,
		Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "voice profile " + after.ID + " saved"},
		Note:    "saved in the voice editor (recorded as a person in a claude-code shell)",
		Voice:   edit,
	})
	require.NoError(t, err)
	data, err := workspace.EncodeSegment(log.ops)
	require.NoError(t, err)
	cos, _ := s.contextObjects()
	require.NoError(t, cos.PutContextObjects(t.Context(), projectID, "member-b",
		[]platstore.ContextObject{{Name: workspace.SegmentName("laptop", rec.ID), Data: data}}))
	return rec.ID
}

// capturedLog is a contextop.Log that keeps what it is given and reads
// nothing back.
type capturedLog struct{ ops []workspace.Op }

func (l *capturedLog) Record(_ context.Context, ops ...workspace.Op) ([]workspace.Op, error) {
	for i := range ops {
		ops[i].ID = workspace.NewOpID(ops[i].At, "")
	}
	l.ops = append(l.ops, ops...)
	return ops, nil
}

func (l *capturedLog) Select(context.Context, workspace.OpQuery) ([]workspace.Op, error) {
	return nil, nil
}
