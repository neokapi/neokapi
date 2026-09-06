package host_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The locale leg of the face parity contract: a project declaring its locales
// in POSIX style answers in BCP-47, and answers the same at every face.
//
// The CLI leg is cli/faceparity_locale_test.go and the desktop leg is in the
// desktop backend, for the module reasons host/facetest describes.

func TestFaceParity_HostCanonicalizesDeclaredLocales(t *testing.T) {
	p := facetest.WritePosix(t)

	proj, err := project.Load(p.Recipe)
	require.NoError(t, err)

	// The recipe keeps what its author wrote.
	assert.Equal(t, "en_US", string(proj.Defaults.SourceLanguage))

	// The resolved runtime every face reads through does not.
	ctx := project.NewProjectContext(proj, p.Recipe)
	assert.Equal(t, facetest.PosixSourceLocale, ctx.SourceLocale)
	assert.Equal(t, []model.LocaleID{facetest.PosixTargetLocale}, ctx.TargetLocales)
}

func TestFaceParity_StatusReportsCanonicalLocales(t *testing.T) {
	p := facetest.WritePosix(t)

	a := &host.App{}
	a.InitRegistries()

	cmd := host.NewEnvCommand(t.Context(), "status")
	host.AddProjectFlag(cmd)
	host.AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", p.Recipe))
	require.NoError(t, cmd.Flags().Set("json", "true"))

	var buf capturingWriter
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, a.RunStatus(cmd, nil))

	var out host.StatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	require.NotEmpty(t, out.Locales, "the fixture declares a target")
	for _, lc := range out.Locales {
		assert.Equal(t, string(facetest.PosixTargetLocale), lc.Locale,
			"status reports the canonical tag for a POSIX-declared target")
	}
}

// The store leg: every store under the project keys its rows by the canonical
// locale, so a row written under the recipe's POSIX spelling is the row a
// canonical lookup finds, and the tally the desktop reads from the block store
// counts a target written under "nb_NO" for "nb-NO".
func TestFaceParity_StoresKeyCanonicalLocales(t *testing.T) {
	p := facetest.WritePosix(t)
	facetest.ExtractToStore(t, p)
	ctx := t.Context()

	a := &host.App{}
	a.InitRegistries()
	defer a.Shutdown()
	db, err := a.ProjectDB(ctx, p.Root)
	require.NoError(t, err)

	// Terms: written as the recipe spells its source, found as every face asks.
	require.NoError(t, db.Terms().AddConcept(ctx, terms.Concept{
		ID:    "c-berth",
		Terms: []terms.Term{{Text: "berth", Locale: "en_US", Status: model.TermPreferred}},
	}))
	hits, err := db.Terms().Lookup(ctx, "berth", terms.LookupOptions{SourceLocale: facetest.PosixSourceLocale})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, facetest.PosixSourceLocale, hits[0].Term.Locale)

	// Content memory: the same, for both locales of a pair.
	require.NoError(t, db.Memory().Add(ctx, memory.Entry{
		ID: "m1",
		Variants: map[model.LocaleID][]model.Run{
			"en_US": {model.TextR("Departing")},
			"nb_NO": {model.TextR("Avgang")},
		},
	}))
	matches, err := db.Memory().LookupText(ctx, "Departing", facetest.PosixSourceLocale, facetest.PosixTargetLocale, memory.LookupOptions{})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Avgang", matches[0].Entry.VariantText(facetest.PosixTargetLocale))

	// Block store: a target a producer writes under the POSIX spelling is the
	// target the coverage tally counts under the canonical one.
	sess, err := db.BlocksAutocommit().Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()
	label := project.CollectionLabel("App")
	n := 0
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Collection: label}) {
		require.NoError(t, berr)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind:      blockstore.TargetOverlayKind("nb_NO"),
			BlockHash: b.Hash,
			Payload:   []byte(`{"text":"x","status":"translated"}`),
		}))
		n++
	}
	require.Positive(t, n, "the fixture has blocks")

	tally, totals, err := convergence.TallyBlockStore(sess, []convergence.BlockStoreScope{{
		Collection: "App", Label: label, Locales: []string{string(facetest.PosixTargetLocale)},
	}})
	require.NoError(t, err)
	cov, ok := tally.Coverage(convergence.Scope{Collection: "App", Locale: string(facetest.PosixTargetLocale)})
	require.True(t, ok)
	assert.Equal(t, totals["App"], cov.Counts[string(model.TargetStatusTranslated)],
		"every block's stored target counts for the canonical locale")
}
