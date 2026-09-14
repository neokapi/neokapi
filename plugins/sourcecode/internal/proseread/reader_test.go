package proseread_test

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/plugins/sourcecode/internal/proseread"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blocks reduces the part stream to name → text, which is what a recipe and a
// check both see.
func blocks(t *testing.T, parts []*model.Part) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		require.True(t, ok)
		out[b.Name] = b.SourceText()
	}
	return out
}

// texts is the same read as blocks, flattened — for the nodes whose path is
// shared or empty, where a map would lose all but one.
func texts(t *testing.T, path string, opts proseread.Options) []string {
	t.Helper()
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	parts, err := proseread.ReadParts(src, model.LocaleEnglish, path, opts)
	require.NoError(t, err)
	var out []string
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		require.True(t, ok)
		out = append(out, b.SourceText())
	}
	return out
}

func read(t *testing.T, path string, opts proseread.Options) map[string]string {
	t.Helper()
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	parts, err := proseread.ReadParts(src, model.LocaleEnglish, path, opts)
	require.NoError(t, err)
	return blocks(t, parts)
}

// TestCaskProseIsSeparatedFromIdentifiers is the claim the grammar exists to
// make: in a file where prose and identifiers are both string literals, the
// tree tells them apart and no allowlist is involved.
//
// The fixture is the real Homebrew cask this repo ships. Its two prose sites —
// the `desc` one-liner `brew info` prints and the `caveats` block Homebrew
// prints after install — sit among a dozen paths, flags, URLs and bundle ids.
func TestCaskProseIsSeparatedFromIdentifiers(t *testing.T) {
	got := read(t, "testdata/kapi-desktop.rb", proseread.Options{
		NodePathPatterns: []string{"desc", "caveats"},
	})

	assert.Equal(t, "Desktop workbench for a project's content context", got["desc"])
	assert.Contains(t, got["caveats"], "The kapi CLI is provided by the kapi-cli formula")
	assert.Len(t, got, 2, "only the two declared node paths are extracted")
}

// Without an include list the reader is honest about everything it can see —
// which is how someone finds out what a file holds before narrowing it.
func TestUnfilteredReadSeesEveryStringButNoStructure(t *testing.T) {
	got := read(t, "testdata/kapi-desktop.rb", proseread.Options{})

	// Prose and identifiers alike, each under the call that owns it.
	assert.Equal(t, "Desktop workbench for a project's content context", got["desc"])
	assert.Equal(t, "Kapi.app", got["app"])
	assert.Equal(t, "https://github.com/neokapi/neokapi", got["homepage"])

	// Never the syntax around them.
	for name := range got {
		assert.NotContains(t, name, "cask ", "a block is named for its call, not its source line")
	}
}

// The heredoc pairing is the subtle one. `caveats <<~EOS` puts the OPENER under
// the call and parks the BODY at the top level, so a parent walk alone credits
// the text to `cask` — the enclosing block rather than the field a recipe names.
func TestHeredocIsAttributedToItsCallNotItsEnclosingBlock(t *testing.T) {
	got := read(t, "testdata/kapi-desktop.rb", proseread.Options{})

	assert.Contains(t, got["caveats"], "Getting started")
	assert.NotContains(t, got["cask"], "Getting started",
		"the heredoc body belongs to caveats, not to the cask block that encloses it")
}

// Comments are off unless asked for: a comment is written for the next person
// reading the code, which is a different register from published prose.
//
// Read as a LIST rather than a map: a top-level comment has no enclosing call,
// so several share the empty node path and a map kee­ps only the last.
func TestCommentsAreOptIn(t *testing.T) {
	const header = "Homebrew Cask formula for Kapi.\nCopy this file to homebrew-tap/Casks/kapi-desktop.rb after release."
	assert.NotContains(t, texts(t, "testdata/kapi-desktop.rb", proseread.Options{}), header)

	assert.Contains(t, texts(t, "testdata/kapi-desktop.rb", proseread.Options{Comments: true}), header,
		"opting in surfaces the file's comments, grouped and without their markers")
}

// A file the build has no grammar for is an error, not an empty read. Reporting
// nothing would read as "checked, clean" for a collection that in fact governs
// nothing.
func TestUnknownExtensionIsRefused(t *testing.T) {
	_, err := proseread.ReadParts([]byte("x = 1"), model.LocaleEnglish, "tools/thing.py", proseread.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tools/thing.py", "the error names the file")
	assert.Contains(t, err.Error(), "ruby", "and the languages the plugin reads")
}

// A file that arrives with no name, from a host that sends none, is read only
// under a declared language.
func TestAFileWithNoNameNeedsADeclaredLanguage(t *testing.T) {
	_, err := proseread.ReadParts([]byte("x = 1"), model.LocaleEnglish, "", proseread.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no language is declared")
	assert.Contains(t, err.Error(), "ruby")
}

func TestDeclaredLanguageWinsOverTheExtension(t *testing.T) {
	src := []byte("cask \"kapi\" do\n  desc \"Desktop workbench\"\nend\n")
	parts, err := proseread.ReadParts(src, model.LocaleEnglish, "Casks/kapi.cask", proseread.Options{Language: "ruby"})
	require.NoError(t, err)
	assert.Equal(t, "Desktop workbench", blocks(t, parts)["desc"])

	_, err = proseread.ReadParts(src, model.LocaleEnglish, "Casks/kapi.rb", proseread.Options{Language: "python"})
	require.Error(t, err, "the declaration is read, though the extension names a grammar")
	assert.Contains(t, err.Error(), `"python"`)
	assert.Contains(t, err.Error(), "Casks/kapi.rb")
	assert.Contains(t, err.Error(), "ruby")
}

func TestGrammarsAreReported(t *testing.T) {
	assert.Equal(t, []string{"ruby"}, proseread.Grammars())
}
