package projector_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
)

// recordVoiceWrite records a voice.write another writer made in the
// workspace log and lets the projector apply it.
func recordVoiceWrite(t *testing.T, p *projector.Projector, ws *workspace.Workspace, put []*coreprofile.VoiceProfile, remove []string) {
	t.Helper()
	op, err := projector.VoiceWrite(key, time.Now(), projector.Origin{By: "server"}, put, remove)
	require.NoError(t, err)
	_, err = ws.Record(t.Context(), op)
	require.NoError(t, err)
	require.NoError(t, p.CatchUp(t.Context()))
}

func TestVoiceWritePutsProfileIntoEmptyStore(t *testing.T) {
	p, ws, _ := open(t)
	recordVoiceWrite(t, p, ws, []*coreprofile.VoiceProfile{{
		ID: "house-voice", Name: "House voice", Description: "Plain and direct.",
	}}, nil)

	got, err := p.Voice().GetProfile(t.Context(), "house-voice")
	require.NoError(t, err)
	assert.Equal(t, "Plain and direct.", got.Description)
	assert.Equal(t, 1, got.Version)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestVoiceWriteUpdatesHeldProfile(t *testing.T) {
	p, ws, _ := open(t)
	ctx := t.Context()
	require.NoError(t, p.Voice().CreateProfile(ctx, &coreprofile.VoiceProfile{
		ID: "house-voice", Name: "House voice", Description: "Old.",
	}))

	recordVoiceWrite(t, p, ws, []*coreprofile.VoiceProfile{{
		ID: "house-voice", Name: "House voice", Description: "New.",
	}}, nil)
	got, err := p.Voice().GetProfile(ctx, "house-voice")
	require.NoError(t, err)
	assert.Equal(t, "New.", got.Description)
	assert.Equal(t, 2, got.Version)
	old, err := p.Voice().GetProfileVersion(ctx, "house-voice", 1)
	require.NoError(t, err)
	assert.Equal(t, "Old.", old.Snapshot.Description)

	// The same content again leaves the profile at the version it holds.
	recordVoiceWrite(t, p, ws, []*coreprofile.VoiceProfile{{
		ID: "house-voice", Name: "House voice", Description: "New.",
	}}, nil)
	got, err = p.Voice().GetProfile(ctx, "house-voice")
	require.NoError(t, err)
	assert.Equal(t, 2, got.Version)
}

func TestVoiceWriteRemovesProfile(t *testing.T) {
	p, ws, _ := open(t)
	ctx := t.Context()
	require.NoError(t, p.Voice().CreateProfile(ctx, &coreprofile.VoiceProfile{ID: "house-voice", Name: "House voice"}))

	recordVoiceWrite(t, p, ws, nil, []string{"house-voice", "never-held"})
	_, err := p.Voice().GetProfile(ctx, "house-voice")
	require.Error(t, err)
}

func TestVoiceWriteSurvivesRebuild(t *testing.T) {
	p, ws, _ := open(t)
	ctx := t.Context()
	recordVoiceWrite(t, p, ws, []*coreprofile.VoiceProfile{{ID: "house-voice", Name: "House voice", Description: "A."}}, nil)
	recordVoiceWrite(t, p, ws, []*coreprofile.VoiceProfile{{ID: "house-voice", Name: "House voice", Description: "B."}}, nil)

	_, err := p.Rebuild(ctx)
	require.NoError(t, err)
	got, err := p.Voice().GetProfile(ctx, "house-voice")
	require.NoError(t, err)
	assert.Equal(t, "B.", got.Description)
	assert.Equal(t, 2, got.Version)
}

func TestVoiceWriteRefusesProfileWithoutID(t *testing.T) {
	_, err := projector.VoiceWrite(key, time.Now(), projector.Origin{}, []*coreprofile.VoiceProfile{{Name: "House voice"}}, nil)
	require.Error(t, err)
	_, err = projector.VoiceWrite(key, time.Now(), projector.Origin{}, nil, nil)
	require.Error(t, err)
}
