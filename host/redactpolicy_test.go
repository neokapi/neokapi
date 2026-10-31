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

// Every ingest path asks RedactionFor for the policy of the file it reads,
// recipe-only and flag-free, so each route answers the same question the
// same way.
func TestRedactionFor_ReadsTheDeclaredPolicy(t *testing.T) {
	t.Run("declared and enabled", func(t *testing.T) {
		p := &project.KapiProject{Defaults: project.Defaults{
			Redaction: &project.RedactionSpec{Enabled: true, Rules: "rules.yaml", Detectors: []string{"rules"}},
		}}
		spec := RedactionFor(p, "")
		require.NotNil(t, spec)
		assert.Equal(t, "rules.yaml", spec.Rules)
		assert.True(t, RedactionDeclared(p))
	})

	t.Run("declared but off", func(t *testing.T) {
		p := &project.KapiProject{Defaults: project.Defaults{
			Redaction: &project.RedactionSpec{Enabled: false, Rules: "rules.yaml"},
		}}
		assert.Nil(t, RedactionFor(p, ""), "a policy switched off is not a policy")
		assert.False(t, RedactionDeclared(p))
	})

	t.Run("not declared", func(t *testing.T) {
		assert.Nil(t, RedactionFor(&project.KapiProject{}, "docs/a.md"))
		assert.Nil(t, RedactionFor(nil, "docs/a.md"), "a nil project must not panic an ingest path")
		assert.False(t, RedactionDeclared(nil))
	})

	t.Run("returns a copy", func(t *testing.T) {
		p := &project.KapiProject{Defaults: project.Defaults{
			Redaction: &project.RedactionSpec{Enabled: true, Rules: "rules.yaml"},
		}}
		got := RedactionFor(p, "")
		got.Rules = "mutated.yaml"
		assert.Equal(t, "rules.yaml", p.Defaults.Redaction.Rules,
			"an ingest path must not be able to rewrite the project's policy")
	})

	t.Run("a collection's policy governs its files", func(t *testing.T) {
		p := &project.KapiProject{
			Defaults: project.Defaults{Redaction: &project.RedactionSpec{Enabled: true, Rules: "default.yaml"}},
			Collections: []project.Collection{
				{Name: "legal", Redaction: &project.RedactionSpec{Enabled: true, Rules: "legal.yaml"},
					Content: []project.ContentItem{{Path: "legal/*.md"}}},
				{Name: "docs", Content: []project.ContentItem{{Path: "docs/*.md"}}},
			},
		}
		assert.Equal(t, "legal.yaml", RedactionFor(p, "legal/terms.md").Rules)
		assert.Equal(t, "default.yaml", RedactionFor(p, "docs/a.md").Rules)
		assert.Equal(t, "default.yaml", RedactionFor(p, "").Rules)
	})
}

// A route that names a file by an absolute or a working-directory path reads
// it relative to the project root, and a file outside the root under the
// defaults.
func TestRedactionPath(t *testing.T) {
	root := t.TempDir()
	assert.Equal(t, "docs/a.md", redactionPath(root, filepath.Join(root, "docs", "a.md")))
	assert.Empty(t, redactionPath(root, filepath.Join(filepath.Dir(root), "elsewhere.md")))
	assert.Empty(t, redactionPath(root, ""))
	assert.Empty(t, redactionPath("", filepath.Join(root, "docs", "a.md")))
}

// RedactAtIngest is called unconditionally from ingest paths, so its no-op and
// refusal behaviour is what keeps those call sites free of policy logic.
func TestRedactAtIngest_Boundaries(t *testing.T) {
	b1 := &model.Block{ID: "b1"}
	b1.SetSourceText("hello")
	blocks := []*model.Block{b1}

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
