package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// A declared policy must be visible to every ingest path, not just to the verb
// that happened to grow it first.
//
// `defaults.redaction` had one consumer — resolveRedaction, called from
// `kapi extract`. `kapi push` read it nowhere, so a project that declared its
// content sensitive still sent originals to the server. This pins the
// resolution as recipe-only and flag-free, which is what lets any ingest point
// ask the same question.
func TestProjectRedaction_ReadsTheDeclaredPolicy(t *testing.T) {
	t.Run("declared and enabled", func(t *testing.T) {
		p := &project.KapiProject{Defaults: project.Defaults{
			Redaction: &project.RedactionSpec{Enabled: true, Rules: "rules.yaml", Detectors: []string{"rules"}},
		}}
		spec := ProjectRedaction(p)
		require.NotNil(t, spec)
		assert.Equal(t, "rules.yaml", spec.Rules)
	})

	t.Run("declared but off", func(t *testing.T) {
		p := &project.KapiProject{Defaults: project.Defaults{
			Redaction: &project.RedactionSpec{Enabled: false, Rules: "rules.yaml"},
		}}
		assert.Nil(t, ProjectRedaction(p), "a policy switched off is not a policy")
	})

	t.Run("not declared", func(t *testing.T) {
		assert.Nil(t, ProjectRedaction(&project.KapiProject{}))
		assert.Nil(t, ProjectRedaction(nil), "a nil project must not panic an ingest path")
	})

	t.Run("returns a copy", func(t *testing.T) {
		p := &project.KapiProject{Defaults: project.Defaults{
			Redaction: &project.RedactionSpec{Enabled: true, Rules: "rules.yaml"},
		}}
		got := ProjectRedaction(p)
		got.Rules = "mutated.yaml"
		assert.Equal(t, "rules.yaml", p.Defaults.Redaction.Rules,
			"an ingest path must not be able to rewrite the project's policy")
	})
}

// RedactAtIngest is called unconditionally from ingest paths, so its no-op and
// refusal behaviour is what keeps those call sites free of policy logic.
func TestRedactAtIngest_Boundaries(t *testing.T) {
	blocks := []*model.Block{{ID: "b1", Source: []model.Run{{Text: &model.TextRun{Text: "hello"}}}}}

	t.Run("no policy is a no-op", func(t *testing.T) {
		require.NoError(t, RedactAtIngest(t.Context(), blocks, nil, t.TempDir(), "", "en"))
	})

	t.Run("no blocks is a no-op", func(t *testing.T) {
		spec := &project.RedactionSpec{Enabled: true, Rules: "rules.yaml"}
		require.NoError(t, RedactAtIngest(t.Context(), nil, spec, t.TempDir(), "v.json", "en"))
	})

	// Silence is the dangerous outcome for a safety control: a project that
	// believes it is protected and is not. Both of these refuse instead.
	t.Run("enabled with no rules refuses", func(t *testing.T) {
		spec := &project.RedactionSpec{Enabled: true, Detectors: []string{"rules"}}
		err := RedactAtIngest(t.Context(), blocks, spec, t.TempDir(), "v.json", "en")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no rules file")
	})

	t.Run("enabled with no vault refuses", func(t *testing.T) {
		spec := &project.RedactionSpec{Enabled: true, Rules: "rules.yaml"}
		err := RedactAtIngest(t.Context(), blocks, spec, t.TempDir(), "", "en")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "somewhere to keep the originals")
	})
}

// The vault is not cache. Losing the cache costs CPU; losing the vault means
// redacted content can never be restored — so it lives outside cache/, where a
// "clear the cache" instruction cannot reach it.
func TestVaultLivesOutsideTheCache(t *testing.T) {
	layout := project.Layout{StateDir: filepath.Join(t.TempDir(), ".kapi")}

	vault := layout.RedactionVaultPath()
	assert.NotContains(t, vault, string(os.PathSeparator)+project.CacheDirName+string(os.PathSeparator),
		"the vault must not sit under the disposable cache root")
	assert.Equal(t, filepath.Join(layout.VaultDir(), "redaction.json"), vault)
}
