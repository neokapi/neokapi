package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The redaction policy is a property of the content, not of a verb. A project
// that declares defaults.redaction must have originals withheld on the route to
// the server — the push scan — the same way `kapi extract` withholds them.
// This checks the store contents the scan produces, not any command output: the
// blocks the connector hands the sync path carry placeholders, and the original
// survives only in the local vault.
func TestScanLocalBlocks_RedactsAtIngest(t *testing.T) {
	const secret = "Project Nightfall"

	root := t.TempDir()

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	rulesPath := filepath.Join(root, "redact-rules.yaml")
	rules := &redaction.RulesFile{
		Detectors: []string{"rules"},
		Rules: []redaction.Rule{
			{Term: secret, Category: "product"},
		},
	}
	require.NoError(t, rules.Save(rulesPath))

	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Redaction: &coreproj.RedactionSpec{
				Enabled:   true,
				Rules:     "redact-rules.yaml",
				Detectors: []string{"rules"},
			},
		},
		Collections: []coreproj.Collection{
			{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}},
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	abs := filepath.Join(root, "locales", "en.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs,
		[]byte(`{"greeting":"Welcome to `+secret+`","farewell":"Goodbye"}`), 0o644))

	conn := NewLocalConnector(&host.App{}, proj, reg)
	defer conn.Close()

	_, blockMap, err := conn.scanLocalBlocks(context.Background(), nil)
	require.NoError(t, err)

	blocks := blockMap["locales/en.json"]
	require.NotEmpty(t, blocks, "scan produced no blocks")

	// The placeholder is a protected run typed under the redaction namespace,
	// not merely a string the writer could re-expand. A redacted span
	// contributes nothing to the flattened source text, so the secret is gone
	// from every block the scan would push.
	var sawPlaceholderRun bool
	for _, b := range blocks {
		for _, r := range b.SourceRuns() {
			if r.Ph != nil && strings.HasPrefix(r.Ph.Type, redaction.CategoryPrefix) {
				sawPlaceholderRun = true
			}
			if r.Ph != nil {
				assert.NotContains(t, r.Ph.Data, secret, "an original leaked into a placeholder run")
				assert.NotContains(t, r.Ph.Equiv, secret, "an original leaked into a placeholder run")
			}
		}
		assert.NotContains(t, b.SourceText(), secret,
			"an original leaked into a block the scan would push")
	}
	require.True(t, sawPlaceholderRun, "no block carried a redaction placeholder run")

	// The original survives only in the local vault, outside the cache.
	vaultPath := proj.Layout.RedactionVaultPath()
	vaultBytes, err := os.ReadFile(vaultPath)
	require.NoError(t, err, "the vault must hold the withheld original")
	assert.Contains(t, string(vaultBytes), secret,
		"the original must be recoverable from the vault")
}

// Two files whose blocks share an ID, each withholding a different name. The
// scan's blocks for one file hold no value of the other's, and the project
// vault keeps each name under its file, so restoring a file's blocks from the
// vault gives that file's name back.
func TestScanLocalBlocks_VaultKeepsEachFilesOriginal(t *testing.T) {
	names := map[string]string{"locales/a.json": "Alice Archer", "locales/b.json": "Bruno Black"}
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	rules := &redaction.RulesFile{Rules: []redaction.Rule{
		{Term: "Alice Archer", Category: "person"},
		{Term: "Bruno Black", Category: "person"},
	}}
	require.NoError(t, rules.Save(filepath.Join(root, "redact-rules.yaml")))
	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Redaction:       &coreproj.RedactionSpec{Enabled: true, Rules: "redact-rules.yaml", Detectors: []string{"rules"}},
		},
		Collections: []coreproj.Collection{
			{Path: "locales/*.json", Format: &coreproj.FormatSpec{Name: "json"}},
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)
	for rel, name := range names {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(`{"greeting":"Hello `+name+`"}`), 0o644))
	}

	conn := NewLocalConnector(&host.App{}, proj, reg)
	defer conn.Close()
	_, blockMap, err := conn.scanLocalBlocks(context.Background(), nil)
	require.NoError(t, err)

	vault, err := redaction.OpenFileVault(proj.Layout.RedactionVaultPath())
	require.NoError(t, err)
	assert.Len(t, vault.All(), len(names), "the vault keeps one name per file")

	for rel, name := range names {
		blocks := blockMap[rel]
		require.Len(t, blocks, 1, rel)
		b := blocks[0]
		for _, other := range names {
			assert.NotContains(t, b.SourceText(), other, "%s holds no withheld name", rel)
		}
		get := func(token string) (string, bool) {
			rv, ok := vault.Get(rel, b.ID, token)
			return rv.Original, ok
		}
		restored, n, _ := redaction.RestoreTree(b.SourceRuns(), get, redaction.ValuesForBlock(vault, rel, b.ID))
		assert.Equal(t, 1, n, rel)
		assert.Equal(t, "Hello "+name, redaction.TextOf(restored), rel)
	}
	require.Equal(t, blockMap["locales/a.json"][0].ID, blockMap["locales/b.json"][0].ID,
		"the two files' blocks share an ID, which is what the vault must tell apart")
}

// A collection's redaction governs the files it holds in place of
// defaults.redaction, on the route to the server as on every other: the block
// index a push sends carries no value either policy withholds from the files
// it governs.
func TestScanLocalBlocks_RedactsEachFileUnderItsCollectionsPolicy(t *testing.T) {
	const person, product = "Ada Lovelace", "Project Nightfall"

	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	save := func(name, term, category string) {
		t.Helper()
		rules := &redaction.RulesFile{Detectors: []string{"rules"}, Rules: []redaction.Rule{{Term: term, Category: category}}}
		require.NoError(t, rules.Save(filepath.Join(root, name)))
	}
	save("default-rules.yaml", person, "person")
	save("legal-rules.yaml", product, "product")

	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Redaction:       &coreproj.RedactionSpec{Enabled: true, Rules: "default-rules.yaml", Detectors: []string{"rules"}},
		},
		Collections: []coreproj.Collection{
			{
				Name:      "legal",
				Redaction: &coreproj.RedactionSpec{Enabled: true, Rules: "legal-rules.yaml", Detectors: []string{"rules"}},
				Content:   []coreproj.ContentItem{{Path: "legal/*.json", Format: &coreproj.FormatSpec{Name: "json"}}},
			},
			{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}},
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)
	body := []byte(`{"line":"` + person + ` leads ` + product + `."}`)
	for _, rel := range []string{"legal/memo.json", "locales/en.json"} {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, body, 0o644))
	}

	conn := NewLocalConnector(&host.App{}, proj, reg)
	defer conn.Close()
	_, blockMap, err := conn.scanLocalBlocks(context.Background(), nil)
	require.NoError(t, err)

	text := func(rel string) string {
		t.Helper()
		blocks := blockMap[rel]
		require.NotEmpty(t, blocks, "the scan read %s", rel)
		var sb strings.Builder
		for _, b := range blocks {
			sb.WriteString(b.SourceText())
			for _, r := range b.SourceRuns() {
				if r.Ph != nil {
					sb.WriteString(r.Ph.Data + r.Ph.Equiv)
				}
			}
		}
		return sb.String()
	}
	legal := text("legal/memo.json")
	assert.NotContains(t, legal, product, "the collection's rules withhold its value from the push")
	assert.Contains(t, legal, person, "the collection's policy replaces the defaults for its files")
	other := text("locales/en.json")
	assert.NotContains(t, other, person, "the defaults withhold their value from a file outside the collection")
	assert.Contains(t, other, product)
}
