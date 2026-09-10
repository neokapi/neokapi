package host

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sourceShipFixture(t *testing.T) (string, string) {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	recipe := `version: v1
name: source-ship
defaults:
  source_language: en
  terms_source: .kapi/terms.json
  voice:
    profile_file: .kapi/voice.yaml
collections:
  - name: content
    content:
      - path: content.json
`
	voice := `id: source-ship
name: Source ship
constraints:
  - id: no-unsupported-assurance
    kind: prohibited_pattern
    version: 1
    source: guidance.md
    statement: Do not offer unsupported assurances.
    regex: '(?i)guaranteed safe'
  - id: explain-clearly
    kind: guidance
    version: 1
    source: guidance.md
    statement: Explain who takes each action.
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
	path := filepath.Join(root, "content.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"title":"Harbor helps you prepare.","body":"Contact our team for help."}`), 0o644))
	return root, path
}

func sourceShipCommand(t *testing.T, root string) *EnvCommand {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "check")
	AddProjectFlag(cmd)
	AddVerifyFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", filepath.Join(root, "kapi.yaml")))
	require.NoError(t, cmd.Flags().Set("ship", "true"))
	return cmd
}

func TestVerifySourceOnlyProjectAndExplicitFileAgree(t *testing.T) {
	root, path := sourceShipFixture(t)
	for _, args := range [][]string{nil, {path}} {
		app := &App{}
		out, err := app.computeVerify(sourceShipCommand(t, root), args)
		require.NoError(t, err)
		require.True(t, out.Pass, "clean source content must not be treated as its own translation: %+v", out)
		qa, ok := gateByName(out, gateChecks)
		require.True(t, ok)
		require.NotNil(t, qa.Coverage)
		assert.Equal(t, verifyCoverage{Files: 1, Blocks: 2}, *qa.Coverage)
		require.NotNil(t, qa.Execution)
		statuses := map[string]check.AnalyzerStatus{}
		for _, run := range qa.Execution.Analyzers {
			statuses[run.ID] = run.Status
		}
		assert.Equal(t, check.AnalyzerPassed, statuses["hygiene"])
		assert.Equal(t, check.AnalyzerUnsupported, statuses["voice.guidance"])
	}
}

func TestVerifySourceOnlyFindingsMatchFileChecks(t *testing.T) {
	root, path := sourceShipFixture(t)
	badContent := `{"title":"Guaranteed safe with ForbiddenName.","body":"We we can help.","empty":" "}`
	require.NoError(t, os.WriteFile(path, []byte(badContent), 0o644))
	app := &App{}
	cmd := sourceShipCommand(t, root)
	ordinary, err := app.ComputeCheck(cmd, []string{path})
	require.NoError(t, err)
	require.NotEmpty(t, ordinary.Findings)
	for _, args := range [][]string{nil, {path}} {
		out, err := (&App{}).computeVerify(sourceShipCommand(t, root), args)
		require.NoError(t, err)
		assert.False(t, out.Pass)
		qa, ok := gateByName(out, gateChecks)
		require.True(t, ok)
		assert.False(t, qa.Pass)
		messages := []string{}
		for _, finding := range qa.Findings {
			messages = append(messages, finding.Message)
			assert.NotContains(t, finding.Message, "identical")
		}
		for _, finding := range ordinary.Findings {
			assert.Contains(t, messages, finding.Message)
		}
		termsGate, ok := gateByName(out, gateTerms)
		require.True(t, ok)
		assert.False(t, termsGate.Pass)
		require.NotEmpty(t, termsGate.Findings)
		assert.Contains(t, termsGate.Findings[0].Message, "ForbiddenName")
		assert.Equal(t, 1, termsGate.Coverage.Files)
	}
}

func TestVerifyMixedProjectKeepsRealTargetChecks(t *testing.T) {
	root, path := sourceShipFixture(t)
	recipePath := filepath.Join(root, "kapi.yaml")
	data, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	data = append(data, []byte(`  - name: bilingual
    content:
      - path: english.json
        target: '{lang}.json'
        target_languages: [fr]
`)...)
	require.NoError(t, os.WriteFile(recipePath, data, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "english.json"), []byte(`{"hello":"Hello {name}"}`), 0o644))
	target := filepath.Join(root, "fr.json")
	require.NoError(t, os.WriteFile(target, []byte(`{"hello":"Bonjour"}`), 0o644))
	app := &App{}
	app.InitRegistries()
	app.SourceLang = "en"
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	targets, err := app.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	out, err := app.computeVerify(sourceShipCommand(t, root), nil)
	require.NoError(t, err)
	qa, ok := gateByName(out, gateChecks)
	require.True(t, ok)
	assert.False(t, qa.Pass)
	assert.Equal(t, verifyCoverage{Files: 2, Blocks: 3}, *qa.Coverage)
	missingPlaceholder := false
	for _, f := range qa.Findings {
		if f.File == "fr.json" && f.Severity == "error" {
			missingPlaceholder = true
		}
	}
	assert.True(t, missingPlaceholder, "real target must retain placeholder checks")
	for _, body := range []string{`{"hello":"Hello {name}"}`, `{}`} {
		require.NoError(t, os.WriteFile(target, []byte(body), 0o644))
		out, err := (&App{}).computeVerify(sourceShipCommand(t, root), []string{target})
		require.NoError(t, err)
		qa, ok := gateByName(out, gateChecks)
		require.True(t, ok)
		assert.False(t, qa.Pass, "identical or empty target still fails")
	}
	out, err = (&App{}).computeVerify(sourceShipCommand(t, root), []string{path})
	require.NoError(t, err)
	assert.True(t, out.Pass, "explicit source does not inherit first target locale")
}

func TestVerifySourceOnlyHonorsReaderConfig(t *testing.T) {
	root, path := sourceShipFixture(t)
	recipePath := filepath.Join(root, "kapi.yaml")
	data, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	data = append(data, []byte(`        format:
          name: json
          config:
            extractionRules: '^title$'
`)...)
	require.NoError(t, os.WriteFile(recipePath, data, 0o644))
	require.NoError(t, os.WriteFile(path, []byte(`{"title":"Harbor is ready.","excluded":"Guaranteed safe."}`), 0o644))
	for _, args := range [][]string{nil, {path}} {
		out, err := (&App{}).computeVerify(sourceShipCommand(t, root), args)
		require.NoError(t, err)
		assert.True(t, out.Pass)
		qa, ok := gateByName(out, gateChecks)
		require.True(t, ok)
		assert.Equal(t, verifyCoverage{Files: 1, Blocks: 1}, *qa.Coverage)
	}
}

func TestVerifyEmptyScopeReportsNoContent(t *testing.T) {
	out := buildVerifyOutput([]verifyGateResult{{Gate: gateChecks, Pass: true, Coverage: &verifyCoverage{}}})
	var buf bytes.Buffer
	require.NoError(t, out.FormatText(&buf))
	assert.Contains(t, buf.String(), "NO CONTENT")
}

func TestVerifySourceTerminologyUsesCollectionScope(t *testing.T) {
	root, path := sourceShipFixture(t)
	recipePath := filepath.Join(root, "kapi.yaml")
	recipe, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	scoped := strings.Replace(string(recipe), "collections:", `profiles:
  help:
    channels: [web]
    voice: .kapi/voice.yaml
collections:`, 1)
	scoped = strings.Replace(scoped, "  - name: content", "  - name: content\n    channel: help/web", 1)
	require.NoError(t, os.WriteFile(recipePath, []byte(scoped), 0o644))
	scopeDir := filepath.Join(root, ".kapi", "profiles", "help")
	require.NoError(t, os.MkdirAll(scopeDir, 0o755))
	concepts := []terms.Concept{{ID: "scoped-service", Terms: []terms.Term{
		{Text: "ForbiddenName", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		{Text: "ScopedName", Locale: model.LocaleEnglish, Status: model.TermForbidden},
	}}}
	data, err := ktb.Marshal(ktb.FromConcepts(concepts))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(scopeDir, "terms.json"), data, 0o644))
	require.NoError(t, os.WriteFile(path, []byte(`{"title":"ForbiddenName and ScopedName help you."}`), 0o644))
	for _, args := range [][]string{nil, {path}} {
		out, err := (&App{}).computeVerify(sourceShipCommand(t, root), args)
		require.NoError(t, err)
		gate, ok := gateByName(out, gateTerms)
		require.True(t, ok)
		require.Len(t, gate.Findings, 1)
		assert.Contains(t, gate.Findings[0].Message, "ScopedName")
		assert.Contains(t, gate.Findings[0].Suggestion, "ForbiddenName")
	}
}
