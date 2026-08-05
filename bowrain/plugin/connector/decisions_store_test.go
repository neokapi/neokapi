package connector

import (
	"testing"

	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDecisionsConnector scaffolds a bare project and a local connector bound to
// the given App, which is what reaches its store.
func newDecisionsConnector(t *testing.T, a *host.App) *BowrainSourceConnector {
	t.Helper()
	proj, err := bproject.InitProject(t.TempDir(), &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{SourceLanguage: "en"},
		},
	})
	require.NoError(t, err)
	return NewLocalConnector(a, proj, registry.NewFormatRegistry())
}

// Staging a pulled decision writes through the App's handle — the same one every
// other caller under that App holds. Not a second opener: the working set is a
// schema of the project's one store, so a connector that opened its own would be
// a second connection pool on the file, and the in-process write gate could no
// longer order a pull's staging against whatever else is writing.
func TestStagePulledDecisions_WritesThroughTheAppsHandle(t *testing.T) {
	a := &host.App{}
	defer a.Shutdown()
	c := newDecisionsConnector(t, a)

	staged, err := c.stagePulledDecisions(t.Context(), []platstore.UnitDecision{{
		ItemName:    "locales/en.json",
		Unit:        "greeting",
		Variant:     "fr",
		Status:      string(model.TargetStatusReviewed),
		ReviewState: "approved",
		Updated:     "2026-08-05T10:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, staged)

	// The decision is readable through the App's own accessor, which proves both
	// reached one store rather than two.
	st, err := a.OpenProjectState(t.Context(), c.project.Root)
	require.NoError(t, err)
	us, found := st.Get(t.Context(), state.Key{Unit: "greeting", Variant: model.Variant("fr")})
	require.True(t, found)
	assert.Equal(t, "approved", us.Decision.ReviewState)
	assert.Equal(t, "locales/en.json", us.Scope)
}

// A newer local record wins: staging reconciles by the Updated stamp rather than
// overwriting whatever it finds.
func TestStagePulledDecisions_LeavesANewerLocalRecord(t *testing.T) {
	a := &host.App{}
	defer a.Shutdown()
	c := newDecisionsConnector(t, a)

	st, err := a.OpenProjectState(t.Context(), c.project.Root)
	require.NoError(t, err)
	require.NoError(t, st.Put(t.Context(), state.UnitState{
		Unit:     "greeting",
		Variant:  model.Variant("fr"),
		Decision: state.Decision{ReviewState: "rejected"},
		Updated:  "2026-08-05T12:00:00Z",
	}))

	staged, err := c.stagePulledDecisions(t.Context(), []platstore.UnitDecision{{
		Unit:        "greeting",
		Variant:     "fr",
		ReviewState: "approved",
		Updated:     "2026-08-05T10:00:00Z",
	}})
	require.NoError(t, err)
	assert.Zero(t, staged, "an older server record must not replace a newer local one")

	us, found := st.Get(t.Context(), state.Key{Unit: "greeting", Variant: model.Variant("fr")})
	require.True(t, found)
	assert.Equal(t, "rejected", us.Decision.ReviewState)
}

// A connector built without an App cannot reach the project store, and says so
// rather than silently staging nothing — the failure mode a nil-tolerant lookup
// would have hidden.
func TestStagePulledDecisions_WithoutAnAppIsAnError(t *testing.T) {
	c := newDecisionsConnector(t, nil)
	_, err := c.stagePulledDecisions(t.Context(), []platstore.UnitDecision{{
		Unit: "greeting", Variant: "fr", ReviewState: "approved",
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no host app")
}
