package jobs

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	voicestore "github.com/neokapi/neokapi/bowrain/voice"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
)

func lockTestEntries(t *testing.T, description, base string) json.RawMessage {
	t.Helper()
	voice, err := json.Marshal(&coreprofile.VoiceProfile{
		Name:        "Acme Voice",
		Description: description,
		Tone:        coreprofile.ToneProfile{Formality: "neutral"},
	})
	require.NoError(t, err)
	entry := contextEntry("docs", map[string]string{"product": "kapi"}, "docs", "Acme Voice", voice)
	entry.VoiceProfileRevision = base
	raw, err := json.Marshal([]*pb.SyncContextEntry{entry})
	require.NoError(t, err)
	return raw
}

// A push that changes a voice profile and then waits longer than the voice
// write's lock bound on a block row still lands: the bound covers the
// profile's lock statement alone, and the rest of the push waits as it
// always did.
func TestPush_VoiceLockBoundEndsWithTheVoiceStatement(t *testing.T) {
	e := newPartsEnv(t)
	ctx := t.Context()

	require.NoError(t, e.push(t, "job-seed", map[string]any{
		"contexts": lockTestEntries(t, "first", ""),
	}))
	voice := e.deps.VoiceStore.(*voicestore.PostgresVoiceStore)
	profiles, err := voice.ListProfiles(ctx, partsWorkspace)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	revs, err := voice.Revisions(ctx, []string{profiles[0].ID})
	require.NoError(t, err)
	base := revs[profiles[0].ID]
	require.NotEmpty(t, base)

	// Another transaction holds the project's block rows for longer than the
	// bound; the push must wait it out.
	hold, err := e.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	var held int
	require.NoError(t, hold.QueryRowContext(ctx,
		`SELECT count(*) FROM (SELECT 1 FROM blocks WHERE project_id = $1 FOR UPDATE) locked`,
		partsProject).Scan(&held))
	require.Equal(t, 2, held)
	const holdFor = 6 * time.Second
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(holdFor)
		_ = hold.Commit()
	}()

	start := time.Now()
	err = e.pushBlocks(t, "job-voice", "Hello again", map[string]any{
		"contexts": lockTestEntries(t, "second", base),
	})
	<-released
	require.NoError(t, err, "a block wait after the voice write is not bounded by it")
	assert.GreaterOrEqual(t, time.Since(start), holdFor, "the push waited for the held row")

	got, err := voice.GetProfile(ctx, profiles[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Description, "the voice change landed")
	blocks, err := e.cs.GetBlocks(ctx, store.BlockQuery{ProjectID: partsProject, Stream: "main", ItemName: "en.json", Limit: 10})
	require.NoError(t, err)
	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Contains(t, texts, "Hello again", "the content landed")
}

// A push answers with the revision each voice profile it carried has once its
// transaction committed. A client that keeps it can change the profile again
// in its next push without first pulling: its own write is not a change it
// has not seen.
func TestPush_AnswersWithTheVoiceRevisionItWrote(t *testing.T) {
	e := newPartsEnv(t)
	ctx := t.Context()
	voice := e.deps.VoiceStore.(*voicestore.PostgresVoiceStore)
	revisionAfter := func(t *testing.T, jobID string) string {
		t.Helper()
		raw, err := e.deps.JobStore.PushParts(ctx, "push-"+jobID)
		require.NoError(t, err)
		var parts venue.PushParts
		require.NoError(t, json.Unmarshal([]byte(raw), &parts))
		profiles, err := voice.ListProfiles(ctx, partsWorkspace)
		require.NoError(t, err)
		require.Len(t, profiles, 1)
		revs, err := voice.Revisions(ctx, []string{profiles[0].ID})
		require.NoError(t, err)
		require.Equal(t, revs[profiles[0].ID], parts.VoiceRevisions["acme voice"], "the answer names the stored revision")
		return parts.VoiceRevisions["acme voice"]
	}

	require.NoError(t, e.push(t, "job-create", map[string]any{"contexts": lockTestEntries(t, "first", "")}))
	created := revisionAfter(t, "job-create")

	require.NoError(t, e.push(t, "job-change", map[string]any{"contexts": lockTestEntries(t, "second", created)}))
	changed := revisionAfter(t, "job-change")
	assert.NotEqual(t, created, changed)

	require.NoError(t, e.push(t, "job-again", map[string]any{"contexts": lockTestEntries(t, "third", changed)}),
		"a second change names the revision the first one wrote")
	profiles, err := voice.ListProfiles(ctx, partsWorkspace)
	require.NoError(t, err)
	assert.Equal(t, "third", profiles[0].Description)
}

// A push takes the voice profile's row lock at the end of its transaction.
// While the push waits on its content, a person saving the same profile is
// not held up; the push then finds the profile moved and is refused, so the
// person's save stands.
func TestPush_TakesTheVoiceLockAfterItsContent(t *testing.T) {
	e := newPartsEnv(t)
	ctx := t.Context()

	require.NoError(t, e.push(t, "job-seed", map[string]any{
		"contexts": lockTestEntries(t, "first", ""),
	}))
	voice := e.deps.VoiceStore.(*voicestore.PostgresVoiceStore)
	profiles, err := voice.ListProfiles(ctx, partsWorkspace)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	revs, err := voice.Revisions(ctx, []string{profiles[0].ID})
	require.NoError(t, err)
	base := revs[profiles[0].ID]

	hold, err := e.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = hold.ExecContext(ctx, "SELECT 1 FROM blocks WHERE project_id = $1 FOR UPDATE", partsProject)
	require.NoError(t, err)

	run := e.stagePush(t, "job-voice", "Hello again", map[string]any{
		"contexts": lockTestEntries(t, "from the push", base),
	})
	pushed := make(chan error, 1)
	go func() { pushed <- run() }()
	// The push is past everything before its content once its backend waits
	// on the held rows.
	require.Eventually(t, func() bool {
		var waiting int
		err := e.db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting)
		return err == nil && waiting > 0
	}, 30*time.Second, 20*time.Millisecond, "the push reached the held rows")

	person := *profiles[0]
	person.Description = "from a person"
	start := time.Now()
	require.NoError(t, voice.UpdateProfileAt(ctx, &person, base))
	assert.Less(t, time.Since(start), 2*time.Second, "the person's save did not wait for the push")

	require.NoError(t, hold.Commit())
	err = <-pushed
	var refusal *VoiceRefusal
	require.ErrorAs(t, err, &refusal, "the push is refused on the profile it would overwrite")

	got, err := voice.GetProfile(ctx, profiles[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "from a person", got.Description)
}
