package host

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
)

// TestVoiceWrite_RefusesAnAgentBeforeTheStore: writing a project's voice
// profile directly is a person's decision. Every path that writes one asks the
// policy before the store is touched, so a refused agent leaves the profile,
// and the operations a sync would share, as they were.
func TestVoiceWrite_RefusesAnAgentBeforeTheStore(t *testing.T) {
	writes := map[string]func(t *testing.T, a *App, cmd Command, root string) error{
		"voice edit": func(t *testing.T, a *App, cmd Command, _ string) error {
			t.Setenv("VISUAL", "")
			t.Setenv("EDITOR", writingEditor(t, "name: Agent Voice\ntone:\n  formality: casual\n"))
			_, err := a.EditVoiceProfile(context.Background(), cmd, VoiceEditRequest{})
			return err
		},
		"voice save": func(_ *testing.T, a *App, _ Command, root string) error {
			_, err := a.SaveVoiceProfileAt(context.Background(), root, project.GovernancePoint{}, contextop.Actor{},
				&coreprofile.VoiceProfile{Name: "Agent Voice", Tone: coreprofile.ToneProfile{Formality: "casual"}})
			return err
		},
		"voice import": func(_ *testing.T, a *App, cmd Command, _ string) error {
			return a.SaveProfileToStore(cmd, &coreprofile.VoiceProfile{ID: "agent-voice", Name: "Agent Voice"}, "")
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			a, cmd, root, recipe := newApplyAssetProject(t)
			ownWorkspace(t, a)
			ctx := context.Background()
			seedVoiceProfile(t, a, cmd)
			store, release, err := a.ProjectVoiceStore(ctx, root)
			require.NoError(t, err)
			defer release()
			before, err := store.ListProfiles(ctx, "")
			require.NoError(t, err)
			held := len(recordedOps(t, a, recipe))

			t.Setenv(EnvActor, "agent")
			t.Setenv(EnvAgentName, "claude")
			t.Setenv(EnvAgentSession, "s9")
			err = write(t, a, cmd, root)
			require.ErrorIs(t, err, contextop.ErrRefused)
			assert.Contains(t, err.Error(), "agent claude/s9 may not edit")

			after, err := store.ListProfiles(ctx, "")
			require.NoError(t, err)
			assert.Equal(t, before, after, "a refused write leaves the store as it was")
			assert.Len(t, recordedOps(t, a, recipe), held)
		})
	}
}
