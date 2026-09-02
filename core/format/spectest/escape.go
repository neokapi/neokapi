package spectest

import (
	"fmt"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
)

// EscapeCase is one character of the modify-path escape corpus.
type EscapeCase struct {
	// Name identifies the character in subtest output.
	Name string
	// R is the character a tool puts into the translated value.
	R rune
}

// EscapeCorpus returns the characters a tool can place inside a translated
// value that a writer must either escape, transform, or refuse.
//
// Byte-exact write-back hides every one of them: an untouched value is replayed
// from the skeleton as its original bytes, so a writer that cannot re-escape
// looks correct until something modifies the value. The corpus therefore covers
// the whole C0 range, DEL, the C1 controls a UTF-8 document can carry, the
// Unicode line separators, the byte-order mark as interior text, and the
// delimiters text formats give structural meaning.
func EscapeCorpus() []EscapeCase {
	cases := make([]EscapeCase, 0, 64)
	for r := rune(0); r <= 0x1F; r++ {
		cases = append(cases, EscapeCase{Name: fmt.Sprintf("C0_%02X", r), R: r})
	}
	cases = append(cases,
		EscapeCase{Name: "DEL_7F", R: 0x7F},
		EscapeCase{Name: "NEL_85", R: 0x85},
		EscapeCase{Name: "C1_9F", R: 0x9F},
		EscapeCase{Name: "NBSP_A0", R: 0xA0},
		EscapeCase{Name: "LS_2028", R: 0x2028},
		EscapeCase{Name: "PS_2029", R: 0x2029},
		EscapeCase{Name: "BOM_FEFF", R: 0xFEFF},
		EscapeCase{Name: "quote", R: '"'},
		EscapeCase{Name: "apostrophe", R: '\''},
		EscapeCase{Name: "backslash", R: '\\'},
		EscapeCase{Name: "ampersand", R: '&'},
		EscapeCase{Name: "lt", R: '<'},
		EscapeCase{Name: "gt", R: '>'},
		EscapeCase{Name: "equals", R: '='},
		EscapeCase{Name: "colon", R: ':'},
		EscapeCase{Name: "hash", R: '#'},
		EscapeCase{Name: "comma", R: ','},
		EscapeCase{Name: "semicolon", R: ';'},
		EscapeCase{Name: "pipe", R: '|'},
		EscapeCase{Name: "brace_open", R: '{'},
		EscapeCase{Name: "bracket_open", R: '['},
		EscapeCase{Name: "percent", R: '%'},
		EscapeCase{Name: "dollar", R: '$'},
		EscapeCase{Name: "at", R: '@'},
	)
	return cases
}

// ModifyProbe drives read → modify → write → reparse for one format, which is
// the path a translated document actually takes. Skeleton replay covers the
// untouched value; this covers the modified one.
//
// The probe wires a skeleton exactly as the runners do, so a writer is exercised
// through the same code path production uses rather than through its generative
// fallback.
type ModifyProbe struct {
	// Format names the format under probe; it prefixes subtest names.
	Format string
	// NewReader and NewWriter build a fresh pair per case.
	NewReader func() format.DataFormatReader
	NewWriter func() format.DataFormatWriter
	// Source is a document carrying at least one translatable block.
	Source []byte
	// Rejects reports the characters the format cannot represent at all. For
	// those the writer is required to fail loudly; emitting a document that
	// cannot be parsed back is the failure this probe exists to catch.
	Rejects func(rune) bool
	// Skip names characters the probed position cannot carry, mapped to the
	// reason. A skipped case is reported, never silently dropped.
	Skip map[rune]string
}

// Run executes the probe over the whole corpus as one subtest per character.
func (p ModifyProbe) Run(t *testing.T) {
	t.Helper()
	for _, c := range EscapeCorpus() {
		t.Run(c.Name, func(t *testing.T) {
			if reason, ok := p.Skip[c.R]; ok {
				t.Skipf("%s: U+%04X not carried in this position: %s", p.Format, c.R, reason)
				return
			}
			p.runCase(t, c)
		})
	}
}

func (p ModifyProbe) runCase(t *testing.T, c EscapeCase) {
	t.Helper()
	// The character is surrounded by ASCII letters so a writer cannot pass the
	// probe by trimming the value.
	want := "Ax" + string(c.R) + "zB"

	reader := p.NewReader()
	writer := p.NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	if err != nil {
		t.Fatalf("%s: wire skeleton: %v", p.Format, err)
	}
	if store != nil {
		defer store.Close()
	}

	parts, err := spec.ReadParts(reader, p.Source)
	if err != nil {
		t.Fatalf("%s: read source: %v", p.Format, err)
	}
	locale := model.LocaleID("fr")
	mutated := 0
	for _, part := range parts {
		if part == nil || part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		block.SetTargetText(locale, want)
		mutated++
	}
	if mutated == 0 {
		t.Fatalf("%s: source produced no translatable block", p.Format)
	}

	writer.SetLocale(locale)
	out, writeErr := spec.WriteParts(writer, parts, p.Source)

	rejected := p.Rejects != nil && p.Rejects(c.R)
	if writeErr != nil {
		if !rejected {
			t.Fatalf("%s: write U+%04X: %v", p.Format, c.R, writeErr)
		}
		return
	}
	if rejected {
		t.Fatalf("%s: writer accepted U+%04X, which the format cannot represent: output %q",
			p.Format, c.R, string(out))
	}

	reparsed, err := spec.ReadParts(p.NewReader(), out)
	if err != nil {
		t.Fatalf("%s: reparse after writing U+%04X failed: %v\noutput: %q",
			p.Format, c.R, err, string(out))
	}
	// A monolingual format reads the value back as a block source; a bilingual
	// one (po, xliff, tmx, ts) reads it back as the target of the locale the
	// writer was given. Both count as surviving.
	var got []string
	for _, part := range reparsed {
		if part == nil || part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		// RenderRunsWithData, not RunsText: a reader may model part of the
		// value as an inline placeholder (an Android `%s`, an ICU argument),
		// and the text-only view drops that run's data. The value survived as
		// long as the writer's own rendering function reproduces it.
		got = append(got, model.RenderRunsWithData(block.Source))
		for key := range block.Targets {
			got = append(got, model.RenderRunsWithData(block.TargetVariant(key).Runs))
		}
	}
	if slices.Contains(got, want) {
		return
	}
	t.Fatalf("%s: U+%04X did not survive the round-trip\nwant a block reading %q\ngot  %q\noutput: %q",
		p.Format, c.R, want, got, string(out))
}
