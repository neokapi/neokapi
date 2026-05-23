package redaction

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeMatches(t *testing.T) {
	tests := []struct {
		name string
		in   []Match
		want []Match
	}{
		{
			name: "sorts by start",
			in:   []Match{{Start: 5, End: 8}, {Start: 0, End: 3}},
			want: []Match{{Start: 0, End: 3}, {Start: 5, End: 8}},
		},
		{
			name: "drops overlap, earlier start wins",
			in:   []Match{{Start: 0, End: 5}, {Start: 3, End: 9}},
			want: []Match{{Start: 0, End: 5}},
		},
		{
			name: "equal start keeps longer",
			in:   []Match{{Start: 0, End: 3}, {Start: 0, End: 7}},
			want: []Match{{Start: 0, End: 7}},
		},
		{
			name: "drops zero-width",
			in:   []Match{{Start: 2, End: 2}, {Start: 4, End: 6}},
			want: []Match{{Start: 4, End: 6}},
		},
		{
			name: "adjacent non-overlapping kept",
			in:   []Match{{Start: 0, End: 3}, {Start: 3, End: 6}},
			want: []Match{{Start: 0, End: 3}, {Start: 3, End: 6}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeMatches(tt.in))
		})
	}
}

func TestCategoryHelpers(t *testing.T) {
	assert.Equal(t, "redaction:person", PlaceholderType("person"))
	cat, ok := CategoryOf("redaction:role")
	assert.True(t, ok)
	assert.Equal(t, "role", cat)
	_, ok = CategoryOf("fmt:bold")
	assert.False(t, ok)
}

func TestRuleDetector_Literal(t *testing.T) {
	d, err := NewRuleDetector([]Rule{
		{Term: "Mr Bean", Category: "person"},
		{Term: "King of England", Category: "role"},
	})
	require.NoError(t, err)

	text := "Mr Bean is the new King of England"
	got, err := d.Detect(context.Background(), text, "en")
	require.NoError(t, err)

	require.Len(t, got, 2)
	assert.Equal(t, Match{Start: 0, End: 7, Category: "person", Original: "Mr Bean"}, got[0])
	assert.Equal(t, "King of England", got[1].Original)
	assert.Equal(t, "role", got[1].Category)
	// Offsets must slice back to the original.
	assert.Equal(t, "King of England", text[got[1].Start:got[1].End])
}

func TestRuleDetector_IgnoreCaseAndWholeWord(t *testing.T) {
	d, err := NewRuleDetector([]Rule{
		{Term: "acme", Category: "org", Flags: []string{FlagIgnoreCase, FlagWholeWord}},
	})
	require.NoError(t, err)

	got, err := d.Detect(context.Background(), "ACME ships acmewidgets and Acme.", "en")
	require.NoError(t, err)
	// "ACME" and "Acme" match; "acmewidgets" does not (whole word).
	require.Len(t, got, 2)
	assert.Equal(t, "ACME", got[0].Original)
	assert.Equal(t, "Acme", got[1].Original)
}

func TestRuleDetector_Regex(t *testing.T) {
	d, err := NewRuleDetector([]Rule{
		{Pattern: `PROJECT-[A-Z]+`, Category: "product"},
	})
	require.NoError(t, err)
	got, err := d.Detect(context.Background(), "ship PROJECT-ZEUS now", "en")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "PROJECT-ZEUS", got[0].Original)
}

func TestRuleDetector_Validation(t *testing.T) {
	_, err := NewRuleDetector([]Rule{{Category: "x"}})
	require.Error(t, err, "neither term nor pattern")

	_, err = NewRuleDetector([]Rule{{Term: "a", Pattern: "b", Category: "x"}})
	require.Error(t, err, "both term and pattern")

	_, err = NewRuleDetector([]Rule{{Term: "a"}})
	require.Error(t, err, "missing category")

	_, err = NewRuleDetector([]Rule{{Pattern: "[", Category: "x"}})
	require.Error(t, err, "bad regex")
}

func TestDetectors_Merge(t *testing.T) {
	a, err := NewRuleDetector([]Rule{{Term: "Mr Bean", Category: "person"}})
	require.NoError(t, err)
	b, err := NewRuleDetector([]Rule{{Term: "England", Category: "location"}})
	require.NoError(t, err)

	ds := Detectors{a, b}
	assert.Equal(t, "rules+rules", ds.Name())
	got, err := ds.Detect(context.Background(), "Mr Bean lives in England", "en")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "person", got[0].Category)
	assert.Equal(t, "location", got[1].Category)
}

// --- Redact / Restore -------------------------------------------------------

func TestRedact_PlainText(t *testing.T) {
	runs := []model.Run{{Text: &model.TextRun{Text: "Mr Bean is the new King of England"}}}
	matches := []Match{
		{Start: 0, End: 7, Category: "person", Original: "Mr Bean"},
		{Start: 19, End: 34, Category: "role", Original: "King of England"},
	}
	out, recs, _ := redactWithDefaults(runs, matches)

	require.Len(t, recs, 2)
	assert.Equal(t, "person", recs[0].Category)
	assert.Equal(t, "Mr Bean", recs[0].Original)
	assert.Equal(t, "rdx1", recs[0].Token)
	assert.Equal(t, "rdx2", recs[1].Token)

	// The flattened text must contain the placeholders, not the secrets.
	flat := model.FlattenRuns(out)
	assert.Contains(t, flat, "[REDACTED:Person]")
	assert.Contains(t, flat, "[REDACTED:Role]")
	assert.NotContains(t, flat, "Mr Bean")
	assert.NotContains(t, flat, "King of England")

	// The first match starts at offset 0, so there is no leading text run:
	// structure is ph, text, ph.
	require.Len(t, out, 3)
	require.NotNil(t, out[0].Ph)
	assert.Equal(t, "redaction:person", out[0].Ph.Type)
	assert.Equal(t, "rdx1", out[0].Ph.ID)
	require.NotNil(t, out[0].Ph.Constraints)
	assert.False(t, out[0].Ph.Constraints.Deletable)
	assert.Equal(t, " is the new ", out[1].Text.Text)
	assert.Equal(t, "redaction:role", out[2].Ph.Type)
	// The original is absent from every placeholder field.
	assert.NotContains(t, out[0].Ph.Data, "Mr Bean")
	assert.NotContains(t, out[0].Ph.Equiv, "Mr Bean")
}

func TestRedact_PreservesInlineCodes(t *testing.T) {
	// "Hello <b>Mr Bean</b>!" — bold codes around the name.
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold"}},
		{Text: &model.TextRun{Text: "Mr Bean"}},
		{PcClose: &model.PcCloseRun{ID: "1"}},
		{Text: &model.TextRun{Text: "!"}},
	}
	// Flattened text is "Hello Mr Bean!" — "Mr Bean" at [6,13).
	flat := TextOf(runs)
	assert.Equal(t, "Hello Mr Bean!", flat)
	matches := []Match{{Start: 6, End: 13, Category: "person", Original: "Mr Bean"}}

	out, recs, _ := redactWithDefaults(runs, matches)
	require.Len(t, recs, 1)

	// Inline codes survive; the name is now a placeholder.
	var kinds []model.RunKind
	for _, r := range out {
		kinds = append(kinds, r.Kind())
	}
	assert.Equal(t, []model.RunKind{
		model.RunKindText,   // "Hello "
		model.RunKindPcOpen, // <b>
		model.RunKindPh,     // [REDACTED:Person]
		model.RunKindPcClose,
		model.RunKindText, // "!"
	}, kinds)
}

func TestRedact_SkipsSpanCrossingInlineCode(t *testing.T) {
	// A match spanning across the inline code (positions 5..8 in flattened
	// "ab cd" where an inline sits between) must be skipped, not applied.
	runs := []model.Run{
		{Text: &model.TextRun{Text: "ab"}},
		{Ph: &model.PlaceholderRun{ID: "v1", Type: "var", Equiv: "X"}},
		{Text: &model.TextRun{Text: "cd"}},
	}
	// Flattened text "abcd"; a match [1,3) would cross the inline boundary.
	matches := []Match{{Start: 1, End: 3, Category: "custom", Original: "bc"}}
	out, recs, _ := redactWithDefaults(runs, matches)
	assert.Empty(t, recs, "cross-inline match must be skipped")
	assert.Equal(t, runs, out)
}

func TestRedactRestore_Roundtrip(t *testing.T) {
	runs := []model.Run{{Text: &model.TextRun{Text: "Mr Bean is the new King of England"}}}
	matches := []Match{
		{Start: 0, End: 7, Category: "person", Original: "Mr Bean"},
		{Start: 19, End: 34, Category: "role", Original: "King of England"},
	}
	redacted, recs, _ := redactWithDefaults(runs, matches)

	vault := NewMemoryVault()
	for _, r := range recs {
		require.NoError(t, vault.Put(RedactedValue{Token: r.Token, Category: r.Category, Original: r.Original, BlockID: "b1"}))
	}

	restored, n := Restore(redacted, func(token string) (string, bool) {
		v, ok := vault.Get("b1", token)
		return v.Original, ok
	})
	assert.Equal(t, 2, n)
	assert.Equal(t, "Mr Bean is the new King of England", TextOf(restored))
}

func TestRestore_UnknownTokenLeftIntact(t *testing.T) {
	runs := []model.Run{
		{Text: &model.TextRun{Text: "x "}},
		{Ph: &model.PlaceholderRun{ID: "rdx9", Type: "redaction:person", Equiv: "[REDACTED:Person]"}},
	}
	out, n := Restore(runs, func(string) (string, bool) { return "", false })
	assert.Equal(t, 0, n)
	assert.Equal(t, runs, out)
}

func TestRenderPlaceholder_Slots(t *testing.T) {
	assert.Equal(t, "[REDACTED:Person]", renderPlaceholder("[REDACTED:{category}]", "person", 1))
	assert.Equal(t, "<<Role#2>>", renderPlaceholder("<<{category}#{n}>>", "role", 2))
}

func redactWithDefaults(runs []model.Run, matches []Match) ([]model.Run, []Redacted, int) {
	out, recs := Redact(runs, matches, RedactOptions{})
	return out, recs, len(recs)
}

// --- Vaults -----------------------------------------------------------------

func TestMemoryVault(t *testing.T) {
	v := NewMemoryVault()
	require.NoError(t, v.Put(RedactedValue{Token: "rdx1", Original: "Secret", BlockID: "b1", Category: "person"}))
	got, ok := v.Get("b1", "rdx1")
	require.True(t, ok)
	assert.Equal(t, "Secret", got.Original)
	_, ok = v.Get("b1", "missing")
	assert.False(t, ok)
	assert.Len(t, v.All(), 1)
}

func TestFileVault_Roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redaction", "batch1.json")
	v, err := OpenFileVault(path)
	require.NoError(t, err)
	require.NoError(t, v.Put(RedactedValue{Token: "rdx1", Original: "Mr Bean", BlockID: "b1", Category: "person"}))
	require.NoError(t, v.Flush())

	// Reopen and confirm persistence.
	v2, err := OpenFileVault(path)
	require.NoError(t, err)
	got, ok := v2.Get("b1", "rdx1")
	require.True(t, ok)
	assert.Equal(t, "Mr Bean", got.Original)
}

func TestOpenFileVault_Missing(t *testing.T) {
	v, err := OpenFileVault(filepath.Join(t.TempDir(), "nope.json"))
	require.NoError(t, err)
	assert.Empty(t, v.All())
}

func TestRulesFile_Roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".kapi", "redaction.yaml")
	rf := &RulesFile{
		Placeholder: "[REDACTED:{category}]",
		Detectors:   []string{"rules"},
		Rules: []Rule{
			{Term: "Mr Bean", Category: "person"},
			{Pattern: "PROJECT-[A-Z]+", Category: "product", Flags: []string{"ignorecase"}},
		},
	}
	require.NoError(t, rf.Save(path))

	loaded, err := LoadRulesFile(path)
	require.NoError(t, err)
	assert.Equal(t, "v1", loaded.Version)
	require.Len(t, loaded.Rules, 2)
	assert.Equal(t, "Mr Bean", loaded.Rules[0].Term)

	det, err := loaded.Detector()
	require.NoError(t, err)
	got, err := det.Detect(context.Background(), "Mr Bean owns PROJECT-X", "en")
	require.NoError(t, err)
	assert.Len(t, got, 2)
}
