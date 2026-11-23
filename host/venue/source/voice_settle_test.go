package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	coreproj "github.com/neokapi/neokapi/core/project"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
)

// newVoicedProject is a project whose "docs" collection resolves the stored
// voice profile "Acme Voice", read in the way `kapi store import` reads it.
func newVoicedProject(t *testing.T) *BowrainSourceConnector {
	t.Helper()
	app := &host.App{}
	root := t.TempDir()
	voiceFile := filepath.Join(root, coreproj.RelStatePath(coreproj.ProfilesDirName, "kapi", "voice.yaml"))
	require.NoError(t, os.MkdirAll(filepath.Dir(voiceFile), 0o755))
	require.NoError(t, os.WriteFile(voiceFile, []byte(
		"name: Acme Voice\ndescription: How Acme sounds.\ntone:\n  formality: neutral\n"), 0o644))

	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Profiles: map[string]coreproj.Profile{
			"kapi": {
				Channels: []coreproj.Channel{{ID: "docs"}},
				Voice:    &coreproj.VoiceBinding{Profile: "acme-voice"},
			},
		},
		Collections: []coreproj.Collection{{
			Name: "docs", Channel: "kapi/docs",
			Content: []coreproj.ContentItem{{Path: "docs/**/*.json"}},
		}},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)
	_, err = app.ImportProjectContext(t.Context(), proj.Layout.RecipePath, host.ContextImportRequest{Force: true})
	require.NoError(t, err)

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return mustLocalConnector(t, app, proj, reg)
}

// serverVoiceEntry is the "docs" collection as a pull carries it, bound to
// the server's version of Acme Voice with the given description.
func serverVoiceEntry(t *testing.T, description string) *pb.SyncContextEntry {
	t.Helper()
	raw, err := json.Marshal(&coreprofile.VoiceProfile{
		Name:        "Acme Voice",
		Description: description,
		Tone:        coreprofile.ToneProfile{Formality: "neutral"},
		UpdatedBy:   "Ana Lima",
		UpdatedAt:   time.Date(2026, 11, 23, 10, 4, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	return &pb.SyncContextEntry{
		Name:                 "docs",
		Channel:              "docs",
		VoiceProfile:         "Acme Voice",
		VoiceProfileRevision: "rev-server",
		VoiceProfileJson:     raw,
		Owner:                venue.ContextOwnerRecipe,
	}
}

func localDescription(t *testing.T, c *BowrainSourceConnector) string {
	t.Helper()
	local, _ := c.localVoice(t.Context(), "docs")
	require.NotNil(t, local)
	return local.Description
}

// A pull that finds the server's profile equal to the local one marks the
// server's revision seen, so the next push names it.
func TestSettleVoice_AgreementMarksTheRevisionSeen(t *testing.T) {
	c := newVoicedProject(t)
	res, err := c.applyPulledContext(t.Context(), []*pb.SyncContextEntry{serverVoiceEntry(t, "How Acme sounds.")}, bowrainconn.PullOptions{})
	require.NoError(t, err)
	assert.Empty(t, res.Differences)
	assert.Equal(t, map[string]string{"acme voice": "rev-server"}, c.cache.VoiceSeen)
}

// A pull that finds someone else's edit on the server lists it field by field
// with who made it, and leaves the revision unseen: a push of the local
// version is refused until the person chooses.
func TestSettleVoice_ADifferenceIsShownAndNotMarkedSeen(t *testing.T) {
	c := newVoicedProject(t)
	res, err := c.applyPulledContext(t.Context(), []*pb.SyncContextEntry{serverVoiceEntry(t, "How Acme sounds now.")}, bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.Len(t, res.Differences, 1)
	d := res.Differences[0]
	assert.Equal(t, bowrainconn.DifferenceUnresolved, d.Outcome)
	assert.Equal(t, "Ana Lima", d.ChangedBy)
	assert.Equal(t, []bowrainconn.FieldDifference{{
		Field: "description", Server: "How Acme sounds now.", Local: "How Acme sounds.",
	}}, d.Fields)
	assert.Empty(t, c.cache.VoiceSeen, "nothing marks the server's version seen")
	assert.Equal(t, "How Acme sounds.", localDescription(t, c), "and nothing local changed")
}

// --keep-local is the person's word that they read the difference and mean
// their version to replace the server's: the revision is marked seen and the
// local profile stays as it was.
func TestSettleVoice_KeepLocalMarksTheRevisionSeen(t *testing.T) {
	c := newVoicedProject(t)
	res, err := c.applyPulledContext(t.Context(), []*pb.SyncContextEntry{serverVoiceEntry(t, "How Acme sounds now.")},
		bowrainconn.PullOptions{KeepLocal: true})
	require.NoError(t, err)
	require.Len(t, res.Differences, 1)
	assert.Equal(t, bowrainconn.DifferenceKept, res.Differences[0].Outcome)
	assert.Equal(t, "rev-server", c.cache.VoiceSeen["acme voice"])
	assert.Equal(t, "How Acme sounds.", localDescription(t, c))
}

// --take-server writes the server's version into the project, where the
// person's own context record shows it, and marks it seen.
func TestSettleVoice_TakeServerWritesTheServersVersion(t *testing.T) {
	c := newVoicedProject(t)
	res, err := c.applyPulledContext(t.Context(), []*pb.SyncContextEntry{serverVoiceEntry(t, "How Acme sounds now.")},
		bowrainconn.PullOptions{TakeServer: true})
	require.NoError(t, err)
	require.Len(t, res.Differences, 1)
	assert.Equal(t, bowrainconn.DifferenceTaken, res.Differences[0].Outcome)
	assert.Equal(t, "rev-server", c.cache.VoiceSeen["acme voice"])
	assert.Equal(t, "How Acme sounds now.", localDescription(t, c), "the server's version is the project's now")

	// A second pull finds the two equal.
	res, err = c.applyPulledContext(t.Context(), []*pb.SyncContextEntry{serverVoiceEntry(t, "How Acme sounds now.")}, bowrainconn.PullOptions{})
	require.NoError(t, err)
	assert.Empty(t, res.Differences)
}
