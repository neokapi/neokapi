package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// A check entry is built from the code and documented by its dossier, and the
// two are held together in both directions: the dossier names every rule id
// the checker reports and no other, with what each reports and what fixes it.

const checkDossier = `displayName: Hygiene
description: Reports the shape defects of a text.
overview: |
  The check reads each block on its own.
rules:
  - id: hygiene.empty
    severity: major
    reports: The block holds nothing.
    fix: Give it content.
  - id: hygiene.doubled-word
    severity: minor
    reports: A word repeated.
    fix: Remove the repeat.
`

var testChecks = []check.SourceCheck{
	{ID: "content-lint", Family: "hygiene", Categories: []string{"empty", "doubled-word"}},
}

func TestCollectChecks_BuildsAnEntryFromTheCodeAndTheDossier(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "checks/content-lint.yaml", checkDossier)

	entries, err := collectChecks(dir, testChecks)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	e := entries[0]
	assert.Equal(t, "content-lint", e.ID)
	assert.Equal(t, KindCheck, e.Kind)
	assert.Equal(t, SourceBuiltIn, e.Source)
	assert.Equal(t, "hygiene", e.RuleFamily, "the family a finding carries, not the checker id")
	assert.Equal(t, "Hygiene", e.DisplayName)
	assert.Equal(t, "Reports the shape defects of a text.", e.Description)
	require.NotNil(t, e.Doc)
	assert.Contains(t, e.Doc.Overview, "reads each block")
	require.Len(t, e.Rules, 2)
	assert.Equal(t, CheckRule{ID: "hygiene.empty", Severity: "major", Reports: "The block holds nothing.", Fix: "Give it content."}, e.Rules[0])
	assert.Equal(t, "hygiene.doubled-word", e.Rules[1].ID, "the code's order is kept")
}

func TestCollectChecks_HoldsTheDossierToTheRulesTheCodeReports(t *testing.T) {
	tests := []struct {
		name    string
		dossier string
		wantErr []string
	}{
		{
			name: "a rule the checker reports is not documented",
			dossier: `displayName: Hygiene
description: Reports the shape defects of a text.
rules:
  - id: hygiene.empty
    reports: The block holds nothing.
    fix: Give it content.
`,
			wantErr: []string{"documents no rule", `"hygiene.doubled-word"`, "content-lint"},
		},
		{
			name: "a rule the checker never reports is documented",
			dossier: checkDossier + `  - id: hygiene.tabs
    reports: A tab.
    fix: Remove it.
`,
			wantErr: []string{`"hygiene.tabs"`, "never reports"},
		},
		{
			name: "a rule says nothing about what fixes it",
			dossier: `displayName: Hygiene
description: Reports the shape defects of a text.
rules:
  - id: hygiene.empty
    reports: The block holds nothing.
  - id: hygiene.doubled-word
    reports: A word repeated.
    fix: Remove the repeat.
`,
			wantErr: []string{`"hygiene.empty"`, "fix"},
		},
		{
			name:    "a dossier with no name",
			dossier: "description: Reports the shape defects of a text.\n",
			wantErr: []string{"displayName"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "checks/content-lint.yaml", tt.dossier)
			_, err := collectChecks(dir, testChecks)
			require.Error(t, err)
			for _, want := range tt.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

// The repository's own dossiers satisfy the binding, so the guard is a live
// statement about the tree rather than a rule only its unit tests obey.
func TestCollectChecks_TheRepositoryIsDocumented(t *testing.T) {
	entries, err := collectChecks("nativedocs", check.SourceChecks())
	require.NoError(t, err)
	require.Len(t, entries, len(check.SourceChecks()))
	for _, e := range entries {
		assert.NotEmpty(t, e.RuleFamily, "%s names its rule family", e.ID)
		assert.NotEmpty(t, e.Rules, "%s documents its rules", e.ID)
	}
}
