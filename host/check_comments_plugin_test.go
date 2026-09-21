package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/registry"
)

// The hint table is what lets a check name the plugin to install with no
// plugin present. It must list exactly the comment languages the plugin's
// manifest declares, with the same names and extensions.
func TestCommentPluginHintsMatchTheManifest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "plugins", "sourcecode", "manifest.json"))
	require.NoError(t, err)
	m, err := manifest.Parse(data)
	require.NoError(t, err)
	require.NotEmpty(t, m.Capabilities.Comments)
	var declared []commentPluginHint
	for _, c := range m.Capabilities.Comments {
		declared = append(declared, commentPluginHint{Plugin: m.Plugin, Language: c.Language, DisplayName: c.DisplayName, Extensions: c.Extensions})
	}
	assert.ElementsMatch(t, declared, commentPluginHints)
}

// commentsOnlyProject is a project whose only content is TypeScript and TSX
// source declared for its comments. It returns the project root.
func commentsOnlyProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("kapi.yaml", `version: v1
name: ts-comments
defaults:
  source_language: en
  voice:
    profile_file: .kapi/voice.yaml
collections:
  - name: code
    source_only: true
    content:
      - path: "src/*.ts"
        comments: true
      - path: "ui/*.tsx"
        comments: true
`)
	write(".kapi/voice.yaml", `id: ts-comments
name: Service
constraints:
  - id: service/plain-words
    version: 1
    source: service-guide.md
    statement: Say use rather than utilize.
    kind: prohibited_pattern
    regex: '(?i)\butilize\b'
`)
	write("src/parse.ts", "/** Parses the input. */\nexport function parse(src: string): string {\n  return src;\n}\n")
	write("ui/Greeting.tsx", "/** Renders the greeting. */\nexport function Greeting() {\n  return <p>Hello</p>;\n}\n")
	readProjectContext(t, root)
	return root
}

// assertCommentsUnread holds a run to naming a file whose comments it could not
// read, the language and the plugin to install, in its warnings and on stderr.
func assertCommentsUnread(t *testing.T, warnings []check.Warning, stderr, file, language string) {
	t.Helper()
	var found *check.Warning
	for i := range warnings {
		if warnings[i].Code == noReaderCode && warnings[i].Source == file {
			found = &warnings[i]
		}
	}
	require.NotNil(t, found, "a %s warning names %s: %+v", noReaderCode, file, warnings)
	assert.Contains(t, found.Message, "no reader for "+language+" comments is installed")
	assert.Contains(t, found.Message, "kapi plugins install sourcecode")
	assert.Contains(t, stderr, "no reader for "+language+" comments")
	assert.Contains(t, stderr, file)
}

// A project whose declared comments are in languages no installed plugin reads
// has nothing a check could read. Neither check passes: each did not run, the
// cause says content was not checked, and each file names the plugin to install.
func TestProjectCheckOfCommentsNoPluginReadsDidNotRun(t *testing.T) {
	t.Run("bare", func(t *testing.T) {
		isolateCheckExecution(t)
		root := commentsOnlyProject(t)
		report, stderr, code, err := bareRun(t, root, nil)
		require.Equal(t, ExitNotRun, code, "%v\n%s", err, stderr)
		assert.False(t, report.Pass)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, check.CauseContentNotChecked, report.DidNotRunCause)
		assert.Contains(t, strings.Join(report.DidNotRun, "; "), filepath.Join("src", "parse.ts"))
		assert.Zero(t, report.Target.Blocks)
		assertCommentsUnread(t, report.Warnings, stderr, filepath.Join("src", "parse.ts"), "TypeScript")
		assertCommentsUnread(t, report.Warnings, stderr, filepath.Join("ui", "Greeting.tsx"), "TSX")
		require.NotNil(t, report.Execution)
		for _, a := range report.Execution.Analyzers {
			assert.False(t, strings.HasPrefix(a.ID, "comments."), "no comment extraction ran: %s", a.ID)
		}
	})
	t.Run("ship", func(t *testing.T) {
		isolateCheckExecution(t)
		root := commentsOnlyProject(t)
		out, stderr, code, err := shipRun(t, root, nil)
		require.Equal(t, ExitNotRun, code, "%v\n%s", err, stderr)
		assert.False(t, out.Pass)
		assert.Equal(t, check.VerdictDidNotRun, out.Verdict)
		assert.Equal(t, check.CauseContentNotChecked, out.DidNotRunCause)
		require.NotEmpty(t, out.Gates)
		for _, g := range out.Gates {
			assert.NotEqual(t, check.VerdictPassed, g.Verdict, "the %s gate read nothing", g.Gate)
		}
		assertCommentsUnread(t, out.Warnings, stderr, "src/parse.ts", "TypeScript")
		assertCommentsUnread(t, out.Warnings, stderr, "ui/Greeting.tsx", "TSX")
	})
	t.Run("must fail: with the plugin installed the same project is read", func(t *testing.T) {
		a := sourcecodeApp(t, nil)
		root := commentsOnlyProject(t)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		report, err := a.ComputeCheck(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)
		assert.Equal(t, 2, report.Target.Blocks, "one doc comment in each file")
		assert.Empty(t, report.Warnings)
	})
}

// A file the user names is a file they asked to have checked. With no plugin to
// read its comments that is an error, and the error names the plugin.
func TestCheckOfANamedTypeScriptFileWithNoPluginNamesThePlugin(t *testing.T) {
	isolateCheckExecution(t)
	root := commentsOnlyProject(t)
	_, _, code, err := bareRun(t, root, []string{filepath.Join("src", "parse.ts")})
	require.ErrorIs(t, err, registry.ErrUnknownFormat)
	assert.Contains(t, err.Error(), "TypeScript comments")
	assert.Contains(t, err.Error(), "kapi plugins install sourcecode")
	assert.NotContains(t, []int{0, ExitGate, ExitNotRun}, code)
}

// A diff over declared TypeScript comments with no plugin lists the file as
// having no reader, names the plugin, and did not run.
func TestDiffCheckNamesThePluginForUnreadComments(t *testing.T) {
	isolateCheckExecution(t)
	root := commentsOnlyProject(t)
	patch := "--- a/src/parse.ts\n+++ b/src/parse.ts\n@@ -1 +1 @@\n-/** Parses input. */\n+/** Parses the input. */\n"
	writeCheckInput(t, root, "change.diff", patch)
	t.Chdir(root)

	cmd := diffCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
	require.NoError(t, cmd.Flags().Set("diff-file", "change.diff"))
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)

	entry := scopeEntry(t, report, filepath.FromSlash("src/parse.ts"))
	assert.Equal(t, check.ScopeNoReader, entry.Status)
	assert.Contains(t, entry.Reason, "no reader for TypeScript comments is installed")
	assert.Contains(t, entry.Reason, "kapi plugins install sourcecode")
	assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	assert.Equal(t, check.CauseContentNotChecked, report.DidNotRunCause)
}
