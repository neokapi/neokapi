package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// unreadProject is a project with a source-only collection in `sourcecode`, a
// format a plugin supplies and no kapi binary carries, and, when readable is
// set, a JSON collection every kapi binary reads. It returns the project root.
func unreadProject(t *testing.T, readable string) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "cask"), 0o755))
	recipe := `version: v1
name: unread-format
defaults:
  source_language: en
collections:
`
	if readable != "" {
		recipe += `  - name: content
    content:
      - path: content.json
`
		require.NoError(t, os.WriteFile(filepath.Join(root, "content.json"), []byte(readable), 0o644))
	}
	recipe += `  - name: cask
    source_only: true
    content:
      - path: "cask/*.rb"
        format:
          name: sourcecode
`
	voice := `id: unread-format
name: Unread format
constraints:
  - id: no-unsupported-assurance
    kind: prohibited_pattern
    version: 1
    source: guidance.md
    statement: Do not offer unsupported assurances.
    regex: '(?i)guaranteed safe'
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapi", "voice.yaml"), []byte(voice), 0o644))
	concepts := []terms.Concept{{ID: "service", Terms: []terms.Term{
		{Text: "Harbor", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		{Text: "ForbiddenName", Locale: model.LocaleEnglish, Status: model.TermForbidden},
	}}}
	data, err := ktb.Marshal(ktb.FromConcepts(concepts))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapi", "terms.json"), data, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cask", "kapi.rb"),
		[]byte("cask \"kapi\" do\n  desc \"Guaranteed safe content engine\"\nend\n"), 0o644))
	readProjectContext(t, root)
	return root
}

const cleanContent = `{"title":"Harbor helps you prepare.","body":"Contact our team for help."}`

// shipRun runs `kapi check --ship --json` over the project and returns the
// parsed result, stderr and the exit code.
func shipRun(t *testing.T, root string, args []string) (verifyOutput, string, int, error) {
	t.Helper()
	cmd := sourceShipCommand(t, root)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := (&App{}).RunVerify(cmd, args)
	var out verifyOutput
	code := ExitCode(cmd, err)
	if code == ExitOK || code == ExitGate || code == ExitNotRun {
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), stdout.String())
	}
	return out, stderr.String(), code, err
}

// bareRun runs a bare `kapi check --output-format json` inside the project and
// returns the parsed report, stderr and the exit code.
func bareRun(t *testing.T, root string, args []string) (check.Report, string, int, error) {
	t.Helper()
	t.Chdir(root)
	cmd := executionCommand(t)
	AddProjectFlag(cmd)
	cmd.Flags().String("output-format", "json", "")
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, "kapi.yaml")))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := (&App{}).RunCheck(cmd, args)
	var report check.Report
	code := ExitCode(cmd, err)
	if code == ExitOK || code == ExitGate || code == ExitNotRun {
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &report), stdout.String())
	}
	return report, stderr.String(), code, err
}

// noReaderCode is the warning code a program branches on. It is part of the
// kapi.check/v2 contract, so the test spells it out rather than importing it.
const noReaderCode = "format.no_reader"

// assertUnreadReported holds a run to naming the file it could not read, and
// the format and plugin that would read it, in its JSON warnings and on stderr.
func assertUnreadReported(t *testing.T, warnings []check.Warning, stderr, file string) {
	t.Helper()
	var found *check.Warning
	for i := range warnings {
		if warnings[i].Code == noReaderCode && warnings[i].Source == file {
			found = &warnings[i]
		}
	}
	require.NotNil(t, found, "a %s warning names %s: %+v", noReaderCode, file, warnings)
	assert.Contains(t, found.Message, `"sourcecode"`)
	assert.Contains(t, found.Message, "kapi plugins install sourcecode")
	assert.Contains(t, stderr, `no reader for format "sourcecode"`)
	assert.Contains(t, stderr, file)
}

// A project-wide `kapi check --ship` over a project that declares a collection
// in a plugin format this machine lacks checks the collections it can read and
// lets their gates decide the exit code. The collection it could not read is
// named, and none of its files counts as checked.
func TestCheckShipSkipsDeclaredContentWithNoReader(t *testing.T) {
	root := unreadProject(t, cleanContent)

	out, stderr, code, err := shipRun(t, root, nil)
	require.Equal(t, 0, code, "the gates over the readable content pass: %v\n%s", err, stderr)
	assert.Equal(t, check.VerdictPassed, out.Verdict)
	assertUnreadReported(t, out.Warnings, stderr, "cask/kapi.rb")

	for _, name := range []string{gateVoice, gateTerms, gateChecks} {
		gate, ok := gateByName(out, name)
		require.True(t, ok, "the %s gate ran", name)
		require.NotNil(t, gate.Coverage, name)
		assert.Equal(t, 1, gate.Coverage.Files, "%s counts only the file it read", name)
		assert.Positive(t, gate.Coverage.Blocks, "%s checked the readable content", name)
		for _, f := range gate.Findings {
			assert.NotEqual(t, "cask/kapi.rb", f.File, "%s reports no finding on content it never read", name)
		}
	}

	// The same content read wrong still fails the gates: the readable collection
	// is checked for real, not waved through with the unreadable one.
	require.NoError(t, os.WriteFile(filepath.Join(root, "content.json"),
		[]byte(`{"title":"Guaranteed safe with ForbiddenName."}`), 0o644))
	out, stderr, code, _ = shipRun(t, root, nil)
	assert.Equal(t, ExitGate, code, stderr)
	assert.Equal(t, check.VerdictFailed, out.Verdict)
	assertUnreadReported(t, out.Warnings, stderr, "cask/kapi.rb")
}

// A bare `kapi check` over the same project checks the declared content it can
// read and reports the rest by name.
func TestBareCheckSkipsDeclaredContentWithNoReader(t *testing.T) {
	root := unreadProject(t, cleanContent)

	report, stderr, code, err := bareRun(t, root, nil)
	require.Equal(t, 0, code, "the readable content passes: %v\n%s", err, stderr)
	assert.Equal(t, check.VerdictPassed, report.Verdict)
	assert.Positive(t, report.Target.Blocks, "the readable content was checked")
	assert.Equal(t, "content.json", report.Target.File, "the report names the one file it checked")
	assertUnreadReported(t, report.Warnings, stderr, filepath.Join("cask", "kapi.rb"))
	require.NotNil(t, report.Execution)
	for _, c := range report.Execution.Contexts {
		assert.NotContains(t, c.File, "kapi.rb", "an unread file records no check context")
	}
	for _, a := range report.Execution.Analyzers {
		assert.NotContains(t, a.File, "kapi.rb", "no analyzer ran on an unread file")
	}

	require.NoError(t, os.WriteFile(filepath.Join(root, "content.json"),
		[]byte(`{"title":"Guaranteed safe with ForbiddenName."}`), 0o644))
	report, stderr, code, _ = bareRun(t, root, nil)
	assert.Equal(t, ExitGate, code, stderr)
	assert.Equal(t, check.VerdictFailed, report.Verdict)
}

// A project whose only content is in a format with no reader has nothing the
// check could read, and content in its scope went unchecked. Neither check
// passes: each did not run, with the cause that says so.
func TestProjectCheckOfOnlyUnreadableContentDidNotRun(t *testing.T) {
	t.Run("bare", func(t *testing.T) {
		root := unreadProject(t, "")
		report, stderr, code, err := bareRun(t, root, nil)
		require.Equal(t, ExitNotRun, code, "%v\n%s", err, stderr)
		assert.False(t, report.Pass)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, check.CauseContentNotChecked, report.DidNotRunCause)
		assert.Contains(t, strings.Join(report.DidNotRun, "; "), filepath.Join("cask", "kapi.rb"))
		assert.Zero(t, report.Target.Blocks)
		assertUnreadReported(t, report.Warnings, stderr, filepath.Join("cask", "kapi.rb"))
	})
	t.Run("ship", func(t *testing.T) {
		root := unreadProject(t, "")
		out, stderr, code, err := shipRun(t, root, nil)
		require.Equal(t, ExitNotRun, code, "%v\n%s", err, stderr)
		assert.False(t, out.Pass)
		assert.Equal(t, check.VerdictDidNotRun, out.Verdict)
		assert.Equal(t, check.CauseContentNotChecked, out.DidNotRunCause)
		assert.Contains(t, strings.Join(out.DidNotRun, "; "), "cask/kapi.rb")
		require.NotEmpty(t, out.Gates)
		for _, g := range out.Gates {
			assert.NotEqual(t, check.VerdictPassed, g.Verdict, "the %s gate read nothing", g.Gate)
		}
		voice, ok := gateByName(out, gateVoice)
		require.True(t, ok, "a bound voice whose content is unreadable is a gate that did not run, never an absent one")
		assert.Equal(t, check.CauseContentNotChecked, voice.DidNotRunCause)
		assertUnreadReported(t, out.Warnings, stderr, "cask/kapi.rb")
	})
}

// A file the user names is a file they asked to have checked. When no reader
// for its format is installed, that is an error, as it always was.
func TestCheckOfANamedFileWithNoReaderStillFails(t *testing.T) {
	t.Run("bare", func(t *testing.T) {
		root := unreadProject(t, cleanContent)
		_, _, code, err := bareRun(t, root, []string{filepath.Join("cask", "kapi.rb")})
		require.ErrorIs(t, err, registry.ErrUnknownFormat)
		assert.NotContains(t, []int{0, ExitGate, ExitNotRun}, code)
	})
	t.Run("ship", func(t *testing.T) {
		root := unreadProject(t, cleanContent)
		_, _, code, err := shipRun(t, root, []string{filepath.Join(root, "cask", "kapi.rb")})
		require.ErrorIs(t, err, registry.ErrUnknownFormat)
		assert.NotContains(t, []int{0, ExitGate, ExitNotRun}, code)
	})
}

// Only a missing reader is skipped. A file in a format kapi reads that fails to
// parse was opened and is broken, and it still fails the project check.
func TestProjectCheckStillFailsOnABrokenFileInAKnownFormat(t *testing.T) {
	t.Run("bare", func(t *testing.T) {
		root := unreadProject(t, `{"title": "Harbor`)
		_, _, code, err := bareRun(t, root, nil)
		require.Error(t, err)
		require.NotErrorIs(t, err, registry.ErrUnknownFormat)
		assert.NotContains(t, []int{0, ExitGate, ExitNotRun}, code)
	})
	t.Run("ship", func(t *testing.T) {
		root := unreadProject(t, `{"title": "Harbor`)
		_, _, code, err := shipRun(t, root, nil)
		require.Error(t, err)
		require.NotErrorIs(t, err, registry.ErrUnknownFormat)
		assert.NotContains(t, []int{0, ExitGate, ExitNotRun}, code)
	})
}

// A diff-scoped check inside the project reaches a declared file in a format no
// installed reader handles through the diff rather than through a name. It
// lists that file as no_reader and checks the rest of the change.
func TestDiffCheckSkipsDeclaredContentWithNoReader(t *testing.T) {
	added := func(path, content string) string {
		lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
		var b strings.Builder
		b.WriteString("diff --git a/" + path + " b/" + path + "\nnew file mode 100644\n--- /dev/null\n+++ b/" + path + "\n")
		b.WriteString("@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@\n")
		for _, l := range lines {
			b.WriteString("+" + l + "\n")
		}
		return b.String()
	}
	run := func(t *testing.T, root, patch string) (check.Report, string, int) {
		t.Helper()
		t.Chdir(root)
		cmd := diffCommand(t)
		AddProjectFlag(cmd)
		cmd.Flags().String("output-format", "json", "")
		require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, "kapi.yaml")))
		require.NoError(t, cmd.Flags().Set("diff-file", writeCheckInput(t, t.TempDir(), "change.diff", patch)))
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		err := (&App{}).RunCheck(cmd, nil)
		code := ExitCode(cmd, err)
		require.Contains(t, []int{0, ExitGate, ExitNotRun}, code, "%v\n%s", err, stderr.String())
		var report check.Report
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &report), stdout.String())
		return report, stderr.String(), code
	}
	cask, err := os.ReadFile(filepath.Join(unreadProject(t, ""), "cask", "kapi.rb"))
	require.NoError(t, err)

	t.Run("beside readable content", func(t *testing.T) {
		root := unreadProject(t, cleanContent)
		report, stderr, code := run(t, root, added("content.json", cleanContent)+added("cask/kapi.rb", string(cask)))
		assert.Equal(t, 0, code, stderr)
		assert.Equal(t, check.VerdictPassed, report.Verdict)
		assert.Positive(t, report.Target.Blocks)
		assert.Equal(t, check.ScopeChecked, scopeEntry(t, report, "content.json").Status)
		entry := scopeEntry(t, report, "cask/kapi.rb")
		assert.Equal(t, check.ScopeNoReader, entry.Status)
		assert.Contains(t, entry.Reason, `"sourcecode"`)
		assertUnreadReported(t, report.Warnings, stderr, "cask/kapi.rb")
	})
	t.Run("alone", func(t *testing.T) {
		root := unreadProject(t, cleanContent)
		report, stderr, code := run(t, root, added("cask/kapi.rb", string(cask)))
		assert.Equal(t, ExitNotRun, code, stderr)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, check.CauseContentNotChecked, report.DidNotRunCause)
		assertUnreadReported(t, report.Warnings, stderr, "cask/kapi.rb")
	})
}

// A collection in a format with no reader can carry translations. The project
// check skips its source and its targets alike and checks the collections it can
// read, including their targets.
func TestCheckShipSkipsTranslatedContentWithNoReader(t *testing.T) {
	root := unreadProject(t, cleanContent)
	recipePath := filepath.Join(root, "kapi.yaml")
	recipe, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	recipe = append(recipe, []byte(`  - name: app
    content:
      - path: english.json
        target: '{lang}.json'
        target_languages: [fr]
  - name: layout
    content:
      - path: pkg/doc.idml
        target: 'pkg/doc.{lang}.idml'
        target_languages: [fr]
        format:
          name: okf_idml
ship_gate: { translated: 100 }
`)...)
	require.NoError(t, os.WriteFile(recipePath, recipe, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "english.json"), []byte(`{"hello":"Hello {name}"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "fr.json"), []byte(`{"hello":"Bonjour {name}"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pkg", "doc.idml"), []byte("<doc>Hello</doc>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pkg", "doc.fr.idml"), []byte("<doc>Bonjour</doc>\n"), 0o644))

	out, stderr, code, err := shipRun(t, root, nil)
	require.Equal(t, 0, code, "%v\n%s", err, stderr)
	assert.Equal(t, check.VerdictPassed, out.Verdict)
	qa, ok := gateByName(out, gateChecks)
	require.True(t, ok)
	assert.Equal(t, verifyCoverage{Files: 2, Blocks: 3}, *qa.Coverage, "content.json and the fr.json target, never the layout")
	var layout *check.Warning
	for i := range out.Warnings {
		if out.Warnings[i].Source == "pkg/doc.idml" {
			layout = &out.Warnings[i]
		}
	}
	require.NotNil(t, layout, "the translated collection it could not read is named: %+v", out.Warnings)
	assert.Equal(t, noReaderCode, layout.Code)
	assert.Contains(t, layout.Message, `"okf_idml"`)
	assert.Contains(t, stderr, `no reader for format "okf_idml"`)
	ship, ok := gateByName(out, gateShip)
	require.True(t, ok, "the ship gate ran over the readable translations")
	assert.Equal(t, check.VerdictPassed, ship.Verdict)

	// `kapi status` publishes the same ship verdict through the same checks and
	// coverage, so it reports the project rather than failing on the layout.
	cmd := NewEnvCommand(t.Context(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipePath))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	var statusErr bytes.Buffer
	cmd.SetErr(&statusErr)
	status, err := captureStdout(t, func() error { return (&App{}).RunStatus(cmd, nil) })
	require.NoError(t, err, statusErr.String())
	var parsed StatusOutput
	require.NoError(t, json.Unmarshal([]byte(status), &parsed), status)
	fr, ok := localeCoverage(parsed, "fr")
	require.True(t, ok)
	assert.Equal(t, 1, fr.Total, "the readable translation is measured and the layout is not")
	require.NotNil(t, parsed.Source)
	assert.Contains(t, parsed.Source.Unreadable, "okf_idml")
	assert.Contains(t, statusErr.String(), `no reader for format "okf_idml"`)
}
