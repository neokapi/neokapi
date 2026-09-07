package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/bowrain/voice"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// The tenancy these tests model is the one authz_project_test.go sets up:
// user-a belongs to ws-a and to nothing else, ws-b is somebody else's
// workspace, and each holds one voice profile.
const (
	ownProfile   = "vp-own"
	otherProfile = "vp-other"
)

// voiceFixture is an MCP server wired the way the platform wires it, over the
// real Postgres voice store, with one profile in the caller's workspace and one
// in a workspace the caller has nothing to do with. The store is the real one
// because promote_rule writes: an in-memory fake whose UpdateProfile does
// nothing cannot show that a refused promotion left the other tenant's profile
// as it was.
type voiceFixture struct {
	ms    *MCPServer
	voice coreprofile.Store
	own   string // project id in ws-a, for the tools that also take one
	other string // project id in ws-b
}

func newVoiceFixture(t *testing.T) *voiceFixture {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	vs, err := voice.NewPostgresVoiceStore(db)
	require.NoError(t, err)

	ctx := context.Background()
	for _, p := range []*coreprofile.VoiceProfile{
		{ID: ownProfile, Scope: tenantOwn, Name: "Mine"},
		{ID: otherProfile, Scope: tenantOther, Name: "Theirs", Vocabulary: coreprofile.VocabularyRules{
			ForbiddenTerms: []coreprofile.TermRule{{Term: "utilize", Replacement: "use"}},
		}},
	} {
		require.NoError(t, vs.CreateProfile(ctx, p))
	}

	ms, err := NewMCPServerWithStore(vs, cs, Config{},
		WithMembershipChecker(&fakeMembership{members: map[string]bool{tenantOwn + "/" + tenantUser: true}}),
	)
	require.NoError(t, err)

	return &voiceFixture{
		ms:    ms,
		voice: vs,
		own:   seedTenantProject(t, cs, tenantOwn, "Mine"),
		other: seedTenantProject(t, cs, tenantOther, "Theirs"),
	}
}

// profileScopedTools is every MCP tool that takes a profile id. Each entry runs
// its tool against one profile id, naming the caller's own project wherever the
// tool also needs one, so the only thing under test is the profile.
func profileScopedTools() []struct {
	family string
	tool   string
	call   func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, profileID string) error
} {
	return []struct {
		family string
		tool   string
		call   func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, profileID string) error
	}{
		{"voice", "check_vocabulary", func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, id string) error {
			_, _, err := f.ms.handleCheckVocabulary(ctx, req, checkVocabularyInput{ProfileID: id, Text: "utilize this"})
			return err
		}},
		{"voice", "get_voice_guide", func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, id string) error {
			_, _, err := f.ms.handleGetVoiceGuide(ctx, req, getVoiceGuideInput{ProfileID: id})
			return err
		}},
		{"scoring", "score_voice_compliance", func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, id string) error {
			_, _, err := f.ms.handleScoreVoiceCompliance(ctx, req, scoreVoiceComplianceInput{ProfileID: id, Text: "utilize this"})
			return err
		}},
		{"scoring", "suggest_corrections", func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, id string) error {
			_, _, err := f.ms.handleSuggestCorrections(ctx, req, suggestCorrectionsInput{ProfileID: id, Text: "utilize this"})
			return err
		}},
		{"scoring", "rewrite_in_voice", func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, id string) error {
			_, _, err := f.ms.handleRewriteInVoice(ctx, req, rewriteInVoiceInput{ProfileID: id, Text: "utilize this"})
			return err
		}},
		{"loop", "get_suggested_rules", func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, id string) error {
			_, _, err := f.ms.handleGetSuggestedRules(ctx, req, getSuggestedRulesInput{
				WorkspaceID: tenantOwn, ProfileID: id,
			})
			return err
		}},
		{"loop", "promote_rule", func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, id string) error {
			_, _, err := f.ms.handlePromoteRule(ctx, req, promoteRuleInput{
				ProfileID: id, Term: "leverage", Replacement: "use",
			})
			return err
		}},
		{"loop", "evaluate_rule", func(ctx context.Context, f *voiceFixture, req *mcp.CallToolRequest, id string) error {
			_, _, err := f.ms.handleEvaluateRule(ctx, req, evaluateRuleInput{
				ProfileID: id, Term: "leverage", ProjectID: f.own,
			})
			return err
		}},
	}
}

// TestProfileScopedTools_RefuseForeignProfile proves every MCP tool taking a
// profile id refuses a profile owned by a workspace the caller does not belong
// to, and answers "not found" rather than "forbidden" so the refusal does not
// confirm the profile exists.
func TestProfileScopedTools_RefuseForeignProfile(t *testing.T) {
	f := newVoiceFixture(t)
	req := callAs(tenantUser)

	for _, tc := range profileScopedTools() {
		t.Run(tc.family+"/"+tc.tool, func(t *testing.T) {
			err := tc.call(t.Context(), f, req, otherProfile)
			require.Error(t, err, "a profile in another workspace must be refused")
			require.ErrorIs(t, err, ErrProfileNotFound)
			assert.NotContains(t, err.Error(), "forbidden")
			assert.NotContains(t, err.Error(), "not authorized")
		})
	}
}

// TestProfileScopedTools_AllowOwnProfile proves the same tools still work on a
// profile in the caller's own workspace: the guard refuses a tenant boundary,
// not every call.
func TestProfileScopedTools_AllowOwnProfile(t *testing.T) {
	f := newVoiceFixture(t)
	req := callAs(tenantUser)

	for _, tc := range profileScopedTools() {
		t.Run(tc.family+"/"+tc.tool, func(t *testing.T) {
			err := tc.call(t.Context(), f, req, ownProfile)
			require.NoError(t, err, "the caller's own profile must stay reachable")
		})
	}
}

// TestProfileScopedTools_RefuseUnauthenticatedCaller proves a call carrying no
// token reaches nothing: the membership check has no identity to satisfy it.
func TestProfileScopedTools_RefuseUnauthenticatedCaller(t *testing.T) {
	f := newVoiceFixture(t)

	for _, req := range []*mcp.CallToolRequest{nil, callAs(""), callAs("nobody")} {
		_, _, err := f.ms.handleGetVoiceGuide(t.Context(), req, getVoiceGuideInput{ProfileID: ownProfile})
		require.ErrorIs(t, err, ErrProfileNotFound)
	}
}

// TestPromoteRule_LeavesForeignProfileUntouched proves the write the issue
// named never lands: the other workspace's profile keeps its vocabulary, its
// version and its decision log.
func TestPromoteRule_LeavesForeignProfileUntouched(t *testing.T) {
	f := newVoiceFixture(t)
	ctx := t.Context()

	before, err := f.voice.GetProfile(ctx, otherProfile)
	require.NoError(t, err)

	_, _, err = f.ms.handlePromoteRule(ctx, callAs(tenantUser), promoteRuleInput{
		ProfileID: otherProfile, Term: "leverage", Replacement: "use",
	})
	require.ErrorIs(t, err, ErrProfileNotFound)

	after, err := f.voice.GetProfile(ctx, otherProfile)
	require.NoError(t, err)
	assert.Equal(t, before.Version, after.Version, "the foreign profile's version was not bumped")
	assert.Equal(t, before.Vocabulary.ForbiddenTerms, after.Vocabulary.ForbiddenTerms,
		"no forbidden term was planted in the foreign profile")

	decisions, err := f.voice.ListRuleDecisions(ctx, otherProfile)
	require.NoError(t, err)
	assert.Empty(t, decisions, "no decision was recorded against the foreign profile")

	// The same promotion into the caller's own profile still works.
	_, out, err := f.ms.handlePromoteRule(ctx, callAs(tenantUser), promoteRuleInput{
		ProfileID: ownProfile, Term: "leverage", Replacement: "use",
	})
	require.NoError(t, err)
	assert.True(t, out.Promoted)
	assert.Equal(t, 2, out.Version)
}

// TestAuthorizeProfile_AbsentAndForeignReadAlike proves the refusal does not
// distinguish a profile that is not there from one that belongs to somebody
// else, so a caller cannot enumerate ids.
func TestAuthorizeProfile_AbsentAndForeignReadAlike(t *testing.T) {
	f := newVoiceFixture(t)
	req := callAs(tenantUser)

	_, absent := f.ms.authorizeProfile(t.Context(), req, "no-such-profile")
	_, foreign := f.ms.authorizeProfile(t.Context(), req, otherProfile)
	require.Error(t, absent)
	require.Error(t, foreign)
	assert.Equal(t, absent.Error(), foreign.Error())

	_, empty := f.ms.authorizeProfile(t.Context(), req, "")
	require.ErrorIs(t, empty, ErrProfileNotFound, "a required profile_id may not be empty")
}

// TestAuthorizeOptionalProfile_EmptyPassesThrough proves the scoring tools
// still resolve a profile from the scope ladder when no profile_id is given.
func TestAuthorizeOptionalProfile_EmptyPassesThrough(t *testing.T) {
	f := newVoiceFixture(t)
	require.NoError(t, f.ms.authorizeOptionalProfile(t.Context(), callAs(tenantUser), ""))
	require.ErrorIs(t,
		f.ms.authorizeOptionalProfile(t.Context(), callAs(tenantUser), otherProfile),
		ErrProfileNotFound)
}

// TestScoringLadder_RefusesForeignBoundProfile proves the voice-scope ladder
// cannot serve another workspace's profile either. The caller's own project
// carries a binding naming a profile in ws-b; resolving it would hand over that
// workspace's guidance without any profile_id being passed at all.
func TestScoringLadder_RefusesForeignBoundProfile(t *testing.T) {
	f := newVoiceFixture(t)
	ctx := t.Context()

	p, err := f.ms.contentStore.GetProject(ctx, f.own)
	require.NoError(t, err)
	if p.Properties == nil {
		p.Properties = map[string]string{}
	}
	p.Properties[coreprofile.PropertyProfileID] = otherProfile
	require.NoError(t, f.ms.contentStore.UpdateProject(ctx, p))

	_, _, err = f.ms.handleScoreVoiceCompliance(ctx, callAs(tenantUser), scoreVoiceComplianceInput{
		Text:            "utilize this",
		voiceScopeInput: voiceScopeInput{ProjectID: f.own},
	})
	require.ErrorIs(t, err, ErrProfileNotFound)

	// Bound to a profile in the caller's own workspace, the same call resolves.
	p.Properties[coreprofile.PropertyProfileID] = ownProfile
	require.NoError(t, f.ms.contentStore.UpdateProject(ctx, p))

	_, out, err := f.ms.handleScoreVoiceCompliance(ctx, callAs(tenantUser), scoreVoiceComplianceInput{
		Text:            "utilize this",
		voiceScopeInput: voiceScopeInput{ProjectID: f.own},
	})
	require.NoError(t, err)
	assert.Equal(t, ownProfile, out.Score.ProfileID)
}

// TestListProfiles_RefusesForeignWorkspace proves list_profiles no longer
// enumerates another workspace's profiles for the asking.
func TestListProfiles_RefusesForeignWorkspace(t *testing.T) {
	f := newVoiceFixture(t)
	req := callAs(tenantUser)

	_, _, err := f.ms.handleListProfiles(t.Context(), req, listProfilesInput{WorkspaceID: tenantOther})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized for workspace")

	_, out, err := f.ms.handleListProfiles(t.Context(), req, listProfilesInput{WorkspaceID: tenantOwn})
	require.NoError(t, err)
	require.Len(t, out.Profiles, 1)
	assert.Equal(t, ownProfile, out.Profiles[0].ID)
}

// TestVoiceResources_RefuseForeignProfile covers the resource half of the same
// surface: brand:// URIs name a profile by global id and a terminology index by
// workspace, and both were readable across the tenant boundary.
func TestVoiceResources_RefuseForeignProfile(t *testing.T) {
	f := newVoiceFixture(t)

	readers := map[string]func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error){
		"brand://profiles/%s":            f.ms.handleReadProfile,
		"brand://profiles/%s/vocabulary": f.ms.handleReadVocabulary,
		"brand://profiles/%s/examples":   f.ms.handleReadExamples,
	}
	for tmpl, read := range readers {
		t.Run(tmpl, func(t *testing.T) {
			_, err := read(t.Context(), resourceRequest(tenantUser, fmt.Sprintf(tmpl, otherProfile)))
			require.Error(t, err, "another workspace's profile must not be readable")

			res, err := read(t.Context(), resourceRequest(tenantUser, fmt.Sprintf(tmpl, ownProfile)))
			require.NoError(t, err)
			require.Len(t, res.Contents, 1)
			assert.NotEmpty(t, res.Contents[0].Text)
		})
	}

	_, err := f.ms.handleReadTerminology(t.Context(),
		resourceRequest(tenantUser, "brand://terminology/"+tenantOther))
	require.Error(t, err, "another workspace's terminology index must not be readable")

	res, err := f.ms.handleReadTerminology(t.Context(),
		resourceRequest(tenantUser, "brand://terminology/"+tenantOwn))
	require.NoError(t, err)
	require.Len(t, res.Contents, 1)
	assert.Contains(t, res.Contents[0].Text, ownProfile)
	assert.NotContains(t, res.Contents[0].Text, otherProfile)
}

// TestVoicePrompts_RefuseForeignProfile covers the prompt half: each prompt
// renders the named profile's guide into its system message.
func TestVoicePrompts_RefuseForeignProfile(t *testing.T) {
	f := newVoiceFixture(t)

	prompts := map[string]struct {
		handler func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error)
		args    map[string]string
	}{
		"write_in_voice":   {f.ms.handleWriteInVoice, map[string]string{"topic": "pricing"}},
		"rewrite_in_voice": {f.ms.handleRewriteInVoicePrompt, map[string]string{"text": "utilize this"}},
		"check_draft":      {f.ms.handleCheckDraft, map[string]string{"draft": "utilize this"}},
	}
	for name, p := range prompts {
		t.Run(name, func(t *testing.T) {
			args := map[string]string{"profile_id": otherProfile}
			for k, v := range p.args {
				args[k] = v
			}
			_, err := p.handler(t.Context(), promptRequest(tenantUser, args))
			require.ErrorIs(t, err, ErrProfileNotFound)

			args["profile_id"] = ownProfile
			res, err := p.handler(t.Context(), promptRequest(tenantUser, args))
			require.NoError(t, err)
			assert.NotEmpty(t, res.Messages)
		})
	}
}

// TestAuthorizeProfile_WithoutMembershipChecker proves a deployment with no
// auth is unaffected: there is no tenant to cross, so every profile resolves.
func TestAuthorizeProfile_WithoutMembershipChecker(t *testing.T) {
	open := &MCPServer{voiceStore: &memVoiceStore{profiles: []*coreprofile.VoiceProfile{
		{ID: ownProfile, Scope: tenantOwn},
		{ID: otherProfile, Scope: tenantOther},
		{ID: "unscoped"}, // a single-owner store leaves Scope empty
	}}}

	for _, id := range []string{ownProfile, otherProfile, "unscoped"} {
		got, err := open.authorizeProfileForUser(t.Context(), "", id)
		require.NoError(t, err)
		assert.Equal(t, id, got.ID)
	}
}

// TestAuthorizeProfile_FailsClosed proves a server that cannot read a profile's
// workspace refuses rather than guessing, once a membership checker says the
// deployment has tenants at all.
func TestAuthorizeProfile_FailsClosed(t *testing.T) {
	guarded := &fakeMembership{members: map[string]bool{tenantOwn + "/" + tenantUser: true}}

	// No voice store at all.
	noStore := &MCPServer{membership: guarded}
	_, err := noStore.authorizeProfileForUser(t.Context(), tenantUser, ownProfile)
	require.ErrorIs(t, err, ErrProfileNotFound)

	// A store that fails the read, which is indistinguishable from an absent
	// profile at this interface and must deny either way.
	failing := &MCPServer{membership: guarded, voiceStore: &memVoiceStore{}}
	_, err = failing.authorizeProfileForUser(t.Context(), tenantUser, ownProfile)
	require.ErrorIs(t, err, ErrProfileNotFound)

	// A profile carrying no workspace belongs to nobody a membership check can
	// name, so it is unreachable on a deployment that has tenants.
	unscoped := &MCPServer{membership: guarded, voiceStore: &memVoiceStore{
		profiles: []*coreprofile.VoiceProfile{{ID: ownProfile}},
	}}
	_, err = unscoped.authorizeProfileForUser(t.Context(), tenantUser, ownProfile)
	require.ErrorIs(t, err, ErrProfileNotFound)
}

// resourceRequest is the resource read an authenticated caller arrives with.
func resourceRequest(userID, uri string) *mcp.ReadResourceRequest {
	return &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: uri},
		Extra:  callAs(userID).Extra,
	}
}

// promptRequest is the prompt fetch an authenticated caller arrives with.
func promptRequest(userID string, args map[string]string) *mcp.GetPromptRequest {
	return &mcp.GetPromptRequest{
		Params: &mcp.GetPromptParams{Arguments: args},
		Extra:  callAs(userID).Extra,
	}
}
