package klf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hasKind reports whether errs contains a ValidationError of the given
// kind, and returns the matching error (or a zero value) for further
// assertions.
func hasKind(errs []ValidationError, kind ValidationErrorKind) (ValidationError, bool) {
	for _, e := range errs {
		if e.Kind == kind {
			return e, true
		}
	}
	return ValidationError{}, false
}

func countKind(errs []ValidationError, kind ValidationErrorKind) int {
	n := 0
	for _, e := range errs {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// ───────── extra-placeholder (ValidateTargetAgainstSource) ─────────

func TestValidateTargetAgainstSource_ExtraPlaceholder(t *testing.T) {
	tests := []struct {
		name        string
		src         *Block
		target      []Run
		wantExtra   []string // placeholder names expected to be flagged extra
		wantMissing []string // placeholder names expected to be flagged missing
	}{
		{
			name: "target introduces a brand-new placeholder",
			src:  filesHeading(),
			// Keep both required placeholders, but add a stray {stray}.
			target: []Run{
				{Text: &TextRun{Text: "Dateien "}},
				{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: `<span className="muted">`, Equiv: "muted"}},
				{Text: &TextRun{Text: "("}},
				{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", SubType: "number",
					Data: "{count}", Equiv: "count"}},
				{Ph: &PlaceholderRun{ID: "9", Type: "jsx:var", SubType: "string",
					Data: "{stray}", Equiv: "stray"}},
				{Text: &TextRun{Text: " passend)"}},
				{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: "</span>", Equiv: "muted"}},
			},
			wantExtra: []string{"stray"},
		},
		{
			name: "extra pivot introduced by a target plural",
			src:  filesHeading(),
			target: []Run{
				{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: `<span className="muted">`, Equiv: "muted"}},
				{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", SubType: "number",
					Data: "{count}", Equiv: "count"}},
				{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: "</span>", Equiv: "muted"}},
				{Plural: &PluralRun{Pivot: "qty", Forms: map[PluralForm][]Run{
					PluralOther: {{Text: &TextRun{Text: "x"}}},
				}}},
			},
			wantExtra: []string{"qty"},
		},
		{
			name: "target reusing only declared placeholders is clean",
			src:  filesHeading(),
			target: []Run{
				{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: `<span className="muted">`, Equiv: "muted"}},
				{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", SubType: "number",
					Data: "{count}", Equiv: "count"}},
				{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: "</span>", Equiv: "muted"}},
			},
			wantExtra: nil,
		},
		{
			name: "missing-and-extra reported together",
			src:  filesHeading(),
			// Drops {count} (missing) and adds {bogus} (extra).
			target: []Run{
				{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: `<span className="muted">`, Equiv: "muted"}},
				{Ph: &PlaceholderRun{ID: "9", Type: "jsx:var", Data: "{bogus}", Equiv: "bogus"}},
				{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span",
					Data: "</span>", Equiv: "muted"}},
			},
			wantExtra:   []string{"bogus"},
			wantMissing: []string{"count"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateTargetAgainstSource(tc.src, tc.target)

			gotExtra := map[string]bool{}
			gotMissing := map[string]bool{}
			for _, e := range errs {
				switch e.Kind {
				case ErrExtraPlaceholder:
					gotExtra[e.Placeholder] = true
				case ErrMissingPlaceholder:
					gotMissing[e.Placeholder] = true
				default:
					t.Fatalf("unexpected error kind %q: %s", e.Kind, e.Message)
				}
			}

			assert.Equal(t, len(tc.wantExtra), countKind(errs, ErrExtraPlaceholder),
				"extra-placeholder count")
			for _, name := range tc.wantExtra {
				assert.Truef(t, gotExtra[name], "expected extra-placeholder %q", name)
			}
			for _, name := range tc.wantMissing {
				assert.Truef(t, gotMissing[name], "expected missing-placeholder %q", name)
			}
		})
	}
}

// A target that re-wraps a paired code with the same equiv must not be
// flagged extra — the equiv is part of the source.
func TestValidateTargetAgainstSource_RewrappedPairedCodeIsClean(t *testing.T) {
	b := filesHeading()
	target := []Run{
		{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", SubType: "number",
			Data: "{count}", Equiv: "count"}},
		{Text: &TextRun{Text: " "}},
		{PcOpen: &PcOpenRun{ID: "7", Type: "jsx:element", SubType: "span",
			Data: `<span className="muted">`, Equiv: "muted"}},
		{Text: &TextRun{Text: "Dateien"}},
		{PcClose: &PcCloseRun{ID: "7", Type: "jsx:element", SubType: "span",
			Data: "</span>", Equiv: "muted"}},
	}
	assert.Empty(t, ValidateTargetAgainstSource(b, target))
}

// ───────── unknown-placeholder (ValidateBlock) ─────────

func TestValidateBlock_UnknownPlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Block)
		wantName string
	}{
		{
			name: "ph equiv not declared",
			mutate: func(b *Block) {
				// Drop the "count" placeholder declaration; the {count}
				// ph run now references an undeclared name.
				b.Placeholders = b.Placeholders[:1] // keep only "muted"
			},
			wantName: "count",
		},
		{
			name: "pcOpen equiv not declared",
			mutate: func(b *Block) {
				b.Placeholders = b.Placeholders[1:] // keep only "count"
			},
			wantName: "muted",
		},
		{
			name: "plural pivot not declared",
			mutate: func(b *Block) {
				*b = *shoppingCart()
				b.Placeholders = nil // pivot "count" is now undeclared
			},
			wantName: "count",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := filesHeading()
			tc.mutate(b)
			errs := ValidateBlock(b)
			e, ok := hasKind(errs, ErrUnknownPlaceholder)
			require.True(t, ok, "expected an unknown-placeholder error, got %v", errs)
			assert.Equal(t, tc.wantName, e.Placeholder)
		})
	}
}

// A fully-declared block emits no unknown-placeholder error.
func TestValidateBlock_AllDeclared_NoUnknown(t *testing.T) {
	for _, b := range []*Block{filesHeading(), tagChip(), shoppingCart()} {
		errs := ValidateBlock(b)
		_, ok := hasKind(errs, ErrUnknownPlaceholder)
		assert.Falsef(t, ok, "block %q should have no unknown-placeholder error: %v", b.ID, errs)
	}
}

// unknown-placeholder dedupes repeated references to the same
// undeclared name.
func TestValidateBlock_UnknownPlaceholderDeduped(t *testing.T) {
	b := &Block{
		ID:           "dup",
		Translatable: true,
		Type:         BlockTypeJSXElement,
		Source: []Run{
			{Ph: &PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{x}", Equiv: "x"}},
			{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", Data: "{x}", Equiv: "x"}},
		},
		// No placeholders declared.
	}
	errs := ValidateBlock(b)
	assert.Equal(t, 1, countKind(errs, ErrUnknownPlaceholder),
		"repeated references to the same undeclared name collapse to one error")
}

// ───────── malformed-runs (ValidateBlock) ─────────

func TestValidateBlock_MalformedRuns(t *testing.T) {
	tests := []struct {
		name      string
		block     *Block
		wantCount int
	}{
		{
			name: "run with no discriminator",
			block: &Block{
				ID: "empty-run", Translatable: true, Type: BlockTypeJSXElement,
				Source: []Run{
					{Text: &TextRun{Text: "ok"}},
					{}, // zero discriminators — malformed
				},
			},
			wantCount: 1,
		},
		{
			name: "run with two discriminators",
			block: &Block{
				ID: "double-run", Translatable: true, Type: BlockTypeJSXElement,
				Source: []Run{
					{Text: &TextRun{Text: "hi"}, Ph: &PlaceholderRun{ID: "1", Type: "t", Data: "d", Equiv: "e"}},
				},
				Placeholders: []Placeholder{{Name: "e", Kind: PlaceholderVariable}},
			},
			wantCount: 1,
		},
		{
			name: "malformed run nested inside a plural form",
			block: &Block{
				ID: "nested", Translatable: true, Type: BlockTypeJSXElement,
				Source: []Run{
					{Plural: &PluralRun{Pivot: "n", Forms: map[PluralForm][]Run{
						PluralOther: {{}}, // malformed inside the form
					}}},
				},
				Placeholders: []Placeholder{{Name: "n", Kind: PlaceholderICUPivot}},
			},
			wantCount: 1,
		},
		{
			name: "malformed run inside a target",
			block: &Block{
				ID: "target-bad", Translatable: true, Type: BlockTypeJSXElement,
				Source:  []Run{{Text: &TextRun{Text: "ok"}}},
				Targets: map[LocaleID][]Run{"fr": {{}}},
			},
			wantCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateBlock(tc.block)
			assert.Equalf(t, tc.wantCount, countKind(errs, ErrMalformedRuns),
				"malformed-runs count; got errs %v", errs)
		})
	}
}

// Well-formed blocks emit no malformed-runs error.
func TestValidateBlock_WellFormed_NoMalformed(t *testing.T) {
	for _, b := range []*Block{filesHeading(), tagChip(), shoppingCart()} {
		errs := ValidateBlock(b)
		assert.Equalf(t, 0, countKind(errs, ErrMalformedRuns),
			"block %q should have no malformed-runs error: %v", b.ID, errs)
	}
}
