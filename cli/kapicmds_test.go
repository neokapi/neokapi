package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/i18n"
)

// TestLocalizeCommandHelp_TranslatesShortLongExample verifies the scope
// derivation (cli.commands.<path>.*, root normalized to "kapi") and the
// in-place rewrite of Short/Long/Example through a catalog-backed Translator.
func TestLocalizeCommandHelp_TranslatesShortLongExample(t *testing.T) {
	root := &cobra.Command{Use: "kapi", Short: "A toolkit"}
	tm := &cobra.Command{Use: "tm", Short: "Manage translation memory"}
	imp := &cobra.Command{
		Use:     "import <file>",
		Short:   "Import a TMX file",
		Long:    "Line one.\nLine two.",
		Example: "  kapi tm import corpus.tmx",
	}
	tm.AddCommand(imp)
	root.AddCommand(tm)

	cat := makeMoCatalog(t, "nb",
		[3]string{"cli.commands.kapi.short", "A toolkit", "Et verktøysett"},
		[3]string{"cli.commands.kapi.tm.short", "Manage translation memory", "Administrer oversettelsesminne"},
		[3]string{"cli.commands.kapi.tm.import.short", "Import a TMX file", "Importer en TMX-fil"},
		[3]string{"cli.commands.kapi.tm.import.long", "Line one.\nLine two.", "Linje én.\nLinje to."},
	)
	LocalizeCommandHelp(root, i18n.NewTranslator("nb", cat))

	assert.Equal(t, "Et verktøysett", root.Short)
	assert.Equal(t, "Administrer oversettelsesminne", tm.Short)
	assert.Equal(t, "Importer en TMX-fil", imp.Short)
	assert.Equal(t, "Linje én.\nLinje to.", imp.Long)
	// Example has no catalog entry — falls back to the English source.
	assert.Equal(t, "  kapi tm import corpus.tmx", imp.Example)
}

// TestLocalizeCommandHelp_GuardsCollapsedMultiline verifies that a
// translation that lost the source's line structure (the project TM's
// plain-text fast path collapses whitespace) is rejected in favor of the
// English source — a one-line blob is worse than untranslated help.
func TestLocalizeCommandHelp_GuardsCollapsedMultiline(t *testing.T) {
	root := &cobra.Command{Use: "kapi"}
	c := &cobra.Command{Use: "merge", Long: "Line one.\nLine two."}
	root.AddCommand(c)

	cat := makeMoCatalog(t, "nb",
		[3]string{"cli.commands.kapi.merge.long", "Line one.\nLine two.", "Linje én. Linje to."},
	)
	LocalizeCommandHelp(root, i18n.NewTranslator("nb", cat))
	assert.Equal(t, "Line one.\nLine two.", c.Long, "collapsed translation must fall back to source")
}

// TestLocalizeCommandHelp_EnglishIsNoop verifies the fast path: an English
// (or noop) translator leaves the tree untouched.
func TestLocalizeCommandHelp_EnglishIsNoop(t *testing.T) {
	root := &cobra.Command{Use: "kapi", Short: "A toolkit"}
	LocalizeCommandHelp(root, i18n.NoopTranslator{})
	assert.Equal(t, "A toolkit", root.Short)
}

// TestOutputT_LocalizesChrome verifies the cli/output chrome table renders
// through the installed Translator under the cli.output.* scopes, and that
// unknown keys and misses degrade safely.
func TestOutputT_LocalizesChrome(t *testing.T) {
	cat := makeMoCatalog(t, "nb",
		[3]string{"cli.output.tools.available", "Available tools:", "Tilgjengelige verktøy:"},
	)
	output.SetTranslator(i18n.NewTranslator("nb", cat))
	t.Cleanup(func() { output.SetTranslator(nil) })

	assert.Equal(t, "Tilgjengelige verktøy:", output.T("tools.available"))
	// Miss: key exists in the table but not in the catalog → English source.
	assert.Equal(t, "Available formats:", output.T("formats.available"))
	// Unknown key → returned verbatim (visible programming error).
	assert.Equal(t, "no.such.key", output.T("no.such.key"))
}
