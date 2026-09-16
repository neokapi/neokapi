package server

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server's check pass and the CLI's disagreed about placeholders, and the
// nightly convergence uses the server. A catalogue leaf carries its placeholder
// in the text rather than as an inline run, so the run-based checks step over
// it and nothing looked at the target at all: a Norwegian help string shipped
// without {ext}, {lang}, {name} or {path}, and an ICU plural was flattened to a
// sentence with no count in it.
//
// These measure the text spelling specifically. The inline-run spelling is
// covered by the ship-state and dashboard fixtures, which build their blocks
// from model.PlaceholderRun.

// placeholderIssues returns the placeholder findings runChecksOnBlock reports
// for a source/target pair given as text, which is how a JSON or gettext
// catalogue carries one.
func placeholderIssues(t *testing.T, source, target string) []CheckIssueResponse {
	t.Helper()
	block := &model.Block{
		ID:           "b1",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: source}}},
		Properties:   map[string]string{},
	}
	block.SetTargetText(model.LocaleFrench, target)

	issues, err := runChecksOnBlock(t.Context(), block, pointChecks{TargetLocale: model.LocaleFrench})
	require.NoError(t, err)

	var out []CheckIssueResponse
	for _, iss := range issues {
		if iss.Type == "placeholder" {
			out = append(out, iss)
		}
	}
	return out
}

// TestRunChecksOnBlock_TextPlaceholderDropped is the CLI help string that
// shipped with its holes gone.
func TestRunChecksOnBlock_TextPlaceholderDropped(t *testing.T) {
	issues := placeholderIssues(t, "Add {name} from {path}", "Ajouter")
	require.NotEmpty(t, issues,
		"a target that dropped its source's placeholders must be reported")
	for _, iss := range issues {
		assert.Equal(t, "error", iss.Severity,
			"a dropped placeholder is release-blocking, not cosmetic")
	}
}

// TestRunChecksOnBlock_ICUPluralFlattened is the defect that failed the
// nightly: the target kept the sentence and lost the picker, so the reader gets
// no count.
func TestRunChecksOnBlock_ICUPluralFlattened(t *testing.T) {
	issues := placeholderIssues(t,
		"{judged, plural, one {The one pair has been judged.} other {Every pair has been judged.}}",
		"Toutes les paires ont été jugées.")
	require.NotEmpty(t, issues, "a flattened ICU plural must be reported")
	for _, iss := range issues {
		assert.Equal(t, "error", iss.Severity, "a lost picker is release-blocking")
	}
}

// errorIssues returns every error-severity finding, whatever its category, so a
// case whose correct outcome is silence can assert silence rather than a
// different complaint.
func errorIssues(t *testing.T, source, target string) []CheckIssueResponse {
	t.Helper()
	block := &model.Block{
		ID:           "b1",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: source}}},
		Properties:   map[string]string{},
	}
	block.SetTargetText(model.LocaleFrench, target)

	issues, err := runChecksOnBlock(t.Context(), block, pointChecks{TargetLocale: model.LocaleFrench})
	require.NoError(t, err)

	var out []CheckIssueResponse
	for _, iss := range issues {
		if iss.Severity == "error" {
			out = append(out, iss)
		}
	}
	return out
}

// TestRunChecksOnBlock_FewerPluralCategoriesIsNotRefused pins the half that must
// never move.
//
// How many plural categories a language needs is a property of that language:
// English writes two where Japanese writes one and Polish writes four. A target
// that writes fewer than its source is correct, and refusing one is the defect
// this check exists to prevent rather than a stricter version of it.
//
// A corpus measurement cannot show this. Norwegian takes the same two categories
// as English, so every measured pair happens to match, and a regression here
// stays invisible until someone adds a language that needs fewer.
//
// The ICU-aware comparison is what decides these cases. Forcing that path off
// makes the literal comparison refuse them, which is what this test catches.
func TestRunChecksOnBlock_FewerPluralCategoriesIsNotRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		target string
	}{
		{
			name:   "one category where the source writes two",
			source: "{n, plural, one {# item} other {# items}}",
			target: "{n, plural, other {# articles}}",
		},
		{
			name:   "one category, with the argument repeated inside the branch",
			source: "{count, plural, one {1 file} other {{count} files}}",
			target: "{count, plural, other {{count} fichiers}}",
		},
		{
			name:   "one category around a sentence carrying another placeholder",
			source: "{n, plural, one {{name} has one match} other {{name} has # matches}}",
			target: "{n, plural, other {{name} a # correspondances}}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Empty(t, placeholderIssues(t, tc.source, tc.target),
				"a language needing fewer plural categories must not be refused")
			require.Empty(t, errorIssues(t, tc.source, tc.target),
				"and nothing else may refuse it either")
		})
	}
}

// TestRunChecksOnBlock_SoundTargetsAreNotRefused is the half that must not
// move. How many times a placeholder occurs inside a picker is a property of
// the language, so a target with fewer plural categories than its source is
// correct rather than lossy, and refusing one would be the defect this check is
// meant to prevent.
func TestRunChecksOnBlock_SoundTargetsAreNotRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		target string
	}{
		{
			name:   "the target carries both placeholders",
			source: "Add {name} from {path}",
			target: "Ajouter {name} depuis {path}",
		},
		{
			name:   "the target writes fewer plural categories than the source",
			source: "{n, plural, one {# issue} other {# issues}}",
			target: "{n, plural, other {# problèmes}}",
		},
		{
			name:   "the target has no placeholder to carry",
			source: "Delete this profile",
			target: "Supprimer ce profil",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, placeholderIssues(t, tc.source, tc.target),
				"a sound target must not be reported")
		})
	}
}
