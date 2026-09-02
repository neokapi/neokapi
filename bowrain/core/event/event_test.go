package event_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActorRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		actorID   string
		actorName string
		wantID    string
		wantName  string
	}{
		{"id and name", "usr_1", "Ada Lovelace", "usr_1", "Ada Lovelace"},
		{"id only leaves the name unset", "usr_1", "", "usr_1", ""},
		{"a blank id is carried as blank", "", "Ada Lovelace", "", "Ada Lovelace"},
		{"both blank", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := event.WithActor(context.Background(), tt.actorID, tt.actorName)
			assert.Equal(t, tt.wantID, event.ActorFromContext(ctx))
			assert.Equal(t, tt.wantName, event.ActorNameFromContext(ctx))
		})
	}
}

// An unattributed context reads as unattributed rather than panicking, so a
// background job that emits an event without a request behind it still works.
func TestActorAbsentFromContext(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, event.ActorFromContext(ctx))
	assert.Empty(t, event.ActorNameFromContext(ctx))
	assert.Equal(t, event.RequestMeta{}, event.RequestMetaFromContext(ctx))
}

// The later actor wins, which is what a nested WithActor has to mean for
// impersonation and for a worker re-attributing work it picked up.
func TestActorIsOverwrittenByANestedCall(t *testing.T) {
	ctx := event.WithActor(context.Background(), "usr_1", "Ada")
	ctx = event.WithActor(ctx, "usr_2", "Grace")
	assert.Equal(t, "usr_2", event.ActorFromContext(ctx))
	assert.Equal(t, "Grace", event.ActorNameFromContext(ctx))
}

// Overwriting with a blank name keeps the previous one: WithActor only stores a
// non-empty name, so the earlier value stays reachable. Attribution therefore
// degrades to a stale name rather than to none.
func TestNestedActorWithoutANameKeepsTheEarlierName(t *testing.T) {
	ctx := event.WithActor(context.Background(), "usr_1", "Ada")
	ctx = event.WithActor(ctx, "usr_2", "")
	assert.Equal(t, "usr_2", event.ActorFromContext(ctx))
	assert.Equal(t, "Ada", event.ActorNameFromContext(ctx))
}

func TestRequestMetaRoundTrip(t *testing.T) {
	meta := event.RequestMeta{RequestID: "req_1", IP: "203.0.113.7", UserAgent: "kapi/1.2.0"}
	ctx := event.WithRequestMeta(context.Background(), meta)
	assert.Equal(t, meta, event.RequestMetaFromContext(ctx))
}

func TestRequestMetaAndActorAreIndependent(t *testing.T) {
	meta := event.RequestMeta{RequestID: "req_1"}
	ctx := event.WithRequestMeta(event.WithActor(context.Background(), "usr_1", "Ada"), meta)
	assert.Equal(t, "usr_1", event.ActorFromContext(ctx))
	assert.Equal(t, meta, event.RequestMetaFromContext(ctx))
}

// The context keys are unexported struct types, so no caller can plant a value
// the accessors will read — attribution comes from WithActor or from nowhere.
func TestContextKeysAreNotReachableByOtherPackages(t *testing.T) {
	type actorKeyType struct{}
	type reqMetaKeyType struct{}

	ctx := context.WithValue(context.Background(), actorKeyType{}, "impostor")
	ctx = context.WithValue(ctx, reqMetaKeyType{}, event.RequestMeta{RequestID: "impostor"})

	assert.Empty(t, event.ActorFromContext(ctx),
		"a look-alike key from another package must not be read as the actor")
	assert.Equal(t, event.RequestMeta{}, event.RequestMetaFromContext(ctx),
		"a look-alike key from another package must not be read as request metadata")
}

// The event type strings are a wire and persistence vocabulary: subscriptions
// match on them, the audit log stores them, and analytics counts them. Renaming
// one splits a stream rather than moving it, so the values are pinned here and
// a change to this table has to be a deliberate edit.
func TestEventTypeWireValues(t *testing.T) {
	tests := pinnedEventTypes()

	// A duplicated value merges two audit streams into one with nothing to
	// signal it — the failure mode a copy-pasted constant produces.
	seen := map[string]string{}
	shape := regexp.MustCompile(`^[a-z][a-z_]*(\.[a-z][a-z_]*)+$`)
	for _, tt := range tests {
		assert.Equal(t, tt.want, string(tt.got))
		assert.Regexp(t, shape, tt.want, "event types are dotted lower-case segments")
		prev, dup := seen[tt.want]
		require.False(t, dup, "%q is also the wire value of %q", tt.want, prev)
		seen[tt.want] = tt.want
	}
}

// pinnedEventTypes is the recorded wire vocabulary: every EventType the package
// declares, beside the string it serialises to.
func pinnedEventTypes() []struct {
	got  event.EventType
	want string
} {
	return []struct {
		got  event.EventType
		want string
	}{
		{event.EventBlockCreated, "block.created"},
		{event.EventBlockUpdated, "block.updated"},
		{event.EventBlockDeleted, "block.deleted"},
		{event.EventProjectCreated, "project.created"},
		{event.EventProjectUpdated, "project.updated"},
		{event.EventProjectDeleted, "project.deleted"},
		{event.EventVersionCreated, "version.created"},
		{event.EventPullCompleted, "connector.pull.completed"},
		{event.EventPushCompleted, "connector.push.completed"},
		{event.EventConvergenceRunCompleted, "convergence.run.completed"},
		{event.EventSyncCompleted, "connector.sync.completed"},
		{event.EventFlowStarted, "flow.started"},
		{event.EventFlowCompleted, "flow.completed"},
		{event.EventFlowFailed, "flow.failed"},
		{event.EventExtractionCompleted, "extraction.completed"},
		{event.EventQualityGatePass, "quality.gate.pass"},
		{event.EventQualityGateFail, "quality.gate.fail"},
		{event.EventStreamCreated, "stream.created"},
		{event.EventStreamMerged, "stream.merged"},
		{event.EventStreamDeleted, "stream.deleted"},
		{event.EventStreamLocked, "stream.locked"},
		{event.EventStreamUnlocked, "stream.unlocked"},
		{event.EventStreamTagged, "stream.tagged"},
		{event.EventCollectionCreated, "collection.created"},
		{event.EventCollectionUpdated, "collection.updated"},
		{event.EventCollectionDeleted, "collection.deleted"},
		{event.EventItemCreated, "item.created"},
		{event.EventItemDeleted, "item.deleted"},
		{event.EventVoiceCheckStarted, "voice.check.started"},
		{event.EventVoiceCheckCompleted, "voice.check.completed"},
		{event.EventVoiceGateFailed, "voice.gate.failed"},
		{event.EventVoiceGatePassed, "voice.gate.passed"},
		{event.EventVoiceDrift, "voice.drift"},
		{event.EventVoiceCorrected, "voice.corrected"},
		{event.EventVoiceProfileUpdated, "voice.profile.updated"},
		{event.EventPushAutomationsCompleted, "push.automations.completed"},
		{event.EventSourceReviewCompleted, "source.review.completed"},
		{event.EventReviewCompleted, "review.completed"},
		{event.EventReviewDecided, "review.decided"},
		{event.EventReviewBulkApproved, "review.bulk_approved"},
		{event.EventAgentConversationCreated, "agent.conversation.created"},
		{event.EventAgentMessageSent, "agent.message.sent"},
		{event.EventAgentToolExecuted, "agent.tool.executed"},
		{event.EventAgentToolApproved, "agent.tool.approved"},
		{event.EventAgentToolDenied, "agent.tool.denied"},
		{event.EventAgentCodeExecuted, "agent.code.executed"},
		{event.EventMemberAdded, "member.added"},
		{event.EventMemberRemoved, "member.removed"},
		{event.EventMemberRoleChanged, "member.role_changed"},
		{event.EventRoleTemplateCreated, "role.template.created"},
		{event.EventRoleTemplateUpdated, "role.template.updated"},
		{event.EventRoleTemplateDeleted, "role.template.deleted"},
		{event.EventInviteCreated, "invite.created"},
		{event.EventInviteAccepted, "invite.accepted"},
		{event.EventInviteRevoked, "invite.revoked"},
		{event.EventTokenCreated, "token.created"},
		{event.EventTokenRevoked, "token.revoked"},
		{event.EventAuthLogin, "auth.login"},
		{event.EventAuthLogout, "auth.logout"},
		{event.EventAuthFailed, "auth.failed"},
		{event.EventSessionGrantCreated, "session.grant.created"},
		{event.EventAuthzDenied, "authz.denied"},
		{event.EventRollbackPerformed, "rollback.performed"},
		{event.EventPlatformConfigChanged, "platform_config.changed"},
	}
}

// Every EventType the package declares is pinned above, so a new event cannot
// reach the wire without its value being written down. Without this the table
// only guards the events someone remembered to add to it.
func TestEveryDeclaredEventTypeIsPinned(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "event.go", nil, 0)
	require.NoError(t, err)

	var declared []string
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "EventType" {
				continue
			}
			for _, v := range vs.Values {
				lit, ok := v.(*ast.BasicLit)
				require.True(t, ok, "an EventType constant must be a string literal")
				declared = append(declared, strings.Trim(lit.Value, `"`))
			}
		}
	}
	require.NotEmpty(t, declared, "no EventType constants found in event.go")

	pinned := map[string]bool{}
	for _, p := range pinnedEventTypes() {
		pinned[p.want] = true
	}
	for _, want := range declared {
		assert.True(t, pinned[want],
			"EventType %q is declared but absent from pinnedEventTypes", want)
	}
	assert.Len(t, pinned, len(declared),
		"pinnedEventTypes records a value event.go no longer declares")
}
