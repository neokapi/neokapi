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
		for _, r := range b.Source {
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
