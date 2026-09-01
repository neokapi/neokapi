package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	neokapiconfig "github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format/schema"
)

// appWithFormats builds an App carrying the real built-in format registry —
// which is what makes "sourcecode" genuinely absent rather than absent because
// nothing at all was registered.
func appWithFormats() *App {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg, formats.RegisterOptions{
		SchemaReg: schema.NewSchemaRegistry(),
		ConfigReg: neokapiconfig.DefaultRegistry,
	})
	return &App{FormatReg: reg}
}

// A format supplied by a PLUGIN is only readable on a machine that installed
// it, so the same recipe reads here and does not there. A project-wide rollup
// must survive that — aborting would make every unrelated collection
// unreportable because one optional dependency is missing — and must never
// survive it silently, because coverage measured over files that were never
// opened reads as progress.
//
// This is not hypothetical: declaring the Homebrew cask as a `sourcecode`
// collection made `kapi status` in this very repo exit 1 for every contributor
// who had not built the plugin, with "no reader for \"sourcecode\"" and nothing
// else — no coverage for the twenty collections that read perfectly well.
func TestSettleSourceStatesSurvivesAMissingReader(t *testing.T) {
	dir := t.TempDir()

	readable := filepath.Join(dir, "readable.md")
	require.NoError(t, os.WriteFile(readable, []byte("# Title\n\nA sentence.\n"), 0o644))

	// A real file whose declared format nothing has registered — exactly what a
	// plugin format looks like before its plugin is installed.
	pluginOnly := filepath.Join(dir, "cask.rb")
	require.NoError(t, os.WriteFile(pluginOnly, []byte("cask \"x\" do\n  desc \"A thing\"\nend\n"), 0o644))

	a := appWithFormats()
	units := []VerifyUnit{
		{SourcePath: readable, SourceFormat: "markdown", Locale: "nb"},
		{SourcePath: pluginOnly, SourceFormat: "sourcecode", Locale: "nb"},
	}

	states, _, unreadable, err := a.settleSourceStates(context.Background(), "", "en", model.SourceGateNone, units)
	require.NoError(t, err, "a missing reader is a missing optional dependency, not a failure")
	assert.Equal(t, []string{"sourcecode"}, unreadable, "the skip is reported by name")
	assert.NotEmpty(t, states, "the collections that DO read are still measured")
}

// Every other read error still fails the rollup. The distinction is the whole
// point of the sentinel: an unknown format means the file was never opened, a
// read error means it was opened and is broken, and only the first is somebody
// else's missing install.
func TestSettleSourceStatesStillFailsOnARealReadError(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone.md")

	a := appWithFormats()
	units := []VerifyUnit{{SourcePath: missing, SourceFormat: "markdown", Locale: "nb"}}

	_, _, _, err := a.settleSourceStates(context.Background(), "", "en", model.SourceGateNone, units)
	require.Error(t, err, "a file that does not exist is not a missing plugin")
}
