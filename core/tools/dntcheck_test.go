package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runDNT(t *testing.T, src, tgt string, terms []string, caseInsensitive bool) []check.Finding {
	t.Helper()
	loc := model.LocaleID("de")
	b := &model.Block{ID: "b", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: src}}}}
	tool.NewVariantView(b).SetTargetText(loc, tgt)

	cfg := NewDNTCheckConfig(loc)
	cfg.Terms = terms
	cfg.CaseInsensitive = caseInsensitive
	tl := NewDNTCheckTool(cfg)
	require.NoError(t, tl.Annotate(tool.NewBlockView(b)))

	ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey)
	if !ok {
		return nil
	}
	return ann.Findings
}

func TestDNTCheck_PreservedTermPasses(t *testing.T) {
	f := runDNT(t,
		"Open the Acme Cloud dashboard",
		"Öffnen Sie das Acme Cloud Dashboard",
		[]string{"Acme Cloud"}, false)
	assert.Empty(t, f, "verbatim term should not be flagged")
}

func TestDNTCheck_TranslatedTermFlaggedCritical(t *testing.T) {
	f := runDNT(t,
		"Open the Acme Cloud dashboard",
		"Öffnen Sie das Akme-Wolke Dashboard",
		[]string{"Acme Cloud"}, false)
	require.Len(t, f, 1)
	assert.Equal(t, "do-not-translate", f[0].Category)
	assert.Equal(t, check.SeverityCritical, f[0].Severity)
	assert.Equal(t, "Acme Cloud", f[0].OriginalText)
}

func TestDNTCheck_TermNotInSourceIgnored(t *testing.T) {
	f := runDNT(t,
		"Open the dashboard",
		"Öffnen Sie das Dashboard",
		[]string{"Acme Cloud"}, false)
	assert.Empty(t, f, "term absent from source is nothing to preserve")
}

func TestDNTCheck_WordBoundaryNoFalsePositive(t *testing.T) {
	// "Go" must not match inside "going" in the source, so nothing is checked.
	f := runDNT(t,
		"We are going home",
		"Wir gehen nach Hause",
		[]string{"Go"}, false)
	assert.Empty(t, f, `"Go" should not match inside "going"`)
}

func TestDNTCheck_CaseSensitivity(t *testing.T) {
	// Exact-case required by default: "iPhone" → "Iphone" is a violation.
	strict := runDNT(t, "Use iPhone now", "Benutze Iphone jetzt", []string{"iPhone"}, false)
	require.Len(t, strict, 1)
	assert.Equal(t, check.SeverityCritical, strict[0].Severity)

	// With case-insensitive preservation, a case-folded match is accepted.
	relaxed := runDNT(t, "Use iPhone now", "Benutze Iphone jetzt", []string{"iPhone"}, true)
	assert.Empty(t, relaxed)
}
