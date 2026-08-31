package yaml

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	yamlv3 "gopkg.in/yaml.v3"
)

// Writer implements DataFormatWriter for YAML files.
type Writer struct {
	format.BaseFormatWriter
	// blocks holds every block in arrival (= source document) order.
	//
	// It is a slice rather than a key-path-keyed map because a key path does
	// not identify a block: the reader emits one block per key occurrence, and
	// YAML documents in the wild repeat keys. Keying by path collapsed the
	// repeats and dropped a translatable string the reader had already handed
	// to the pipeline — translated, and written into content memory — with
	// nothing logged (#1597).
	blocks        []*model.Block
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new YAML writer.
func NewWriter() *Writer {
	return &Writer{
		FormatName: "yaml",
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes YAML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block) // block.ID → block (for skeleton store)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					w.blocks = append(w.blocks, block)
					blocksByID[block.ID] = block
				}
			}
		}
	}
done:
	// Mode 1: Skeleton store (byte-exact, streaming-friendly).
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("yaml writer: flush skeleton: %w", err)
		}
		return w.writeFromSkeleton(w.skeletonStore, blocksByID)
	}

	// Mode 2: Rebuild from blocks (lossy formatting).
	return w.flush()
}

// writeFromSkeleton reads skeleton entries and fills in block content.
// This produces byte-exact output — only translated text differs from the original.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("yaml writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				// Re-emit the scalar's original bytes while they still spell
				// exactly the text being written — byte-exact output for a block
				// nothing touched. The test is against the text the reader
				// recorded with those bytes, NOT against block.SourceText():
				// after a source edit the block's source IS the new text, so
				// that comparison always held and the pre-edit bytes won,
				// discarding the edit (#1473).
				if raw, ok := format.VerbatimFor(block, "yaml.raw", text); ok {
					if _, err := io.WriteString(w.Output, raw); err != nil {
						return err
					}
				} else {
					style := block.Properties["yaml.style"]
					indicator := block.Properties["yaml.indicator"]
					indent := block.Properties["yaml.indent"]
					encoded := encodeYAMLScalarWithIndicatorIndent(text, style, indicator, indent)
					// The scalar encoders emit bare LF for multi-line
					// bodies (block scalars, multi-line quoted strings).
					// When the source uses CRLF the surrounding skeleton
					// already carries CRLF, so a re-encoded scalar emitting
					// LF would mix conventions within one document. Rewrite
					// to the source's dominant line ending — mirroring
					// Okapi's YamlSkeletonWriter, which normalises to LF then
					// replays getLineBreak() on every break. Empty / "\n"
					// eol leaves the LF-source common case untouched.
					if eol := block.Properties["yaml.eol"]; eol == "\r\n" {
						encoded = applyEOL(encoded, eol)
					}
					if _, err := io.WriteString(w.Output, encoded); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// encodeYAMLScalarWithIndicatorIndent encodes a string value using the
// specified YAML scalar style. The caller passes the original block-scalar
// indicator (`|`, `|-`, `|+`, `|2`, `>-`, …) so the chomp / explicit-indent
// modifier carries through on round-trip; empty indicator falls back to the
// bare `|` / `>` defaults. The original block-scalar content indent
// (decimal string, e.g. "12") captured by the reader as `yaml.indent` is
// also threaded through; empty indent falls back to a compact 2-space
// default suitable for fresh emission.
func encodeYAMLScalarWithIndicatorIndent(text, style, indicator, indent string) string {
	// A plain, single-quoted, literal or folded scalar can only carry the
	// characters of YAML 1.2's printable production. Anything else has to be
	// written as a double-quoted scalar with an escape, whatever style the
	// source used — the alternative is emitting a raw control byte, which
	// yaml.v3 (and every other parser) rejects on the way back in.
	if needsDoubleQuoting(text) {
		return encodeDoubleQuoted(text)
	}
	switch style {
	case "double-quoted":
		return encodeDoubleQuoted(text)
	case "single-quoted":
		return encodeSingleQuoted(text)
	case "literal":
		return encodeLiteralBlockWithIndicatorIndent(text, indicator, indent)
	case "folded":
		return encodeFoldedBlockWithIndicatorIndent(text, indicator, indent)
	default:
		if !plainScalarSafe(text) {
			return encodeDoubleQuoted(text)
		}
		return encodePlain(text)
	}
}

// plainScalarSafe reports whether s can be emitted as a plain scalar and read
// back as itself.
//
// A plain scalar has no delimiters, so the value's own bytes decide where it
// ends and what it means: a line break ends it, leading or trailing space is
// stripped, ` #` starts a comment, `: ` reads as a mapping, and a leading
// indicator character opens a different node type. A translated value hits all
// of these — the source scalar was plain because the ORIGINAL text was safe,
// which says nothing about the replacement.
func plainScalarSafe(s string) bool {
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, "\n\r") {
		return false
	}
	if strings.TrimSpace(s) != s {
		return false
	}
	if strings.ContainsAny(s, ",[]{}") {
		return false
	}
	if strings.Contains(s, ": ") || strings.HasSuffix(s, ":") {
		return false
	}
	if strings.Contains(s, " #") {
		return false
	}
	return !strings.ContainsRune("-?:#&*!|>'\"%@`", rune(s[0]))
}

// needsDoubleQuoting reports whether s carries a character no plain,
// single-quoted or block scalar can hold: the C0 controls other than tab and
// line feed, DEL, and the C1 controls. Only a double-quoted scalar can escape
// those.
//
// Tab and line feed are excluded because every style carries them — tab
// literally, line feed as the style's own line structure. NEL (U+0085) is
// included even though YAML 1.2 §5.1 lists it as printable: yaml.v3 folds it
// as a line break, so a value carrying it literally comes back split.
func needsDoubleQuoting(s string) bool {
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n':
		case r < 0x20, r == 0x7F:
			return true
		case r >= 0x80 && r <= 0x9F:
			return true
		}
	}
	return false
}

// encodeDoubleQuoted encodes a string as a YAML double-quoted scalar, escaping
// every character the production does not admit literally (YAML 1.2 §7.3.1).
// Characters with a named escape get it; the rest take the `\xNN` / `\uNNNN`
// numeric forms.
func encodeDoubleQuoted(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\x00':
			b.WriteString(`\0`)
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\v':
			b.WriteString(`\v`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		case '\x1b':
			b.WriteString(`\e`)
		case 0x85:
			b.WriteString(`\N`)
		default:
			switch {
			case r < 0x20 || r == 0x7F:
				fmt.Fprintf(&b, `\x%02x`, r)
			case r >= 0x80 && r <= 0x9F:
				fmt.Fprintf(&b, `\x%02x`, r)
			default:
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// encodeSingleQuoted encodes a string as a YAML single-quoted scalar.
// Single-line bodies are wrapped with `'...'` and any embedded `'`
// doubled per YAML 1.2 §7.3.2. Multi-line bodies must round-trip
// through YAML's single-quoted line-folding rules: a parsed value
// containing N consecutive line breaks comes from N+1 source line
// breaks (a single break folds to a space; N>=2 breaks preserve
// N-1 literal breaks).
//
// Okapi's snakeyaml writer encodes paragraph breaks as
// `<trailing-space>` + (N+1) source newlines between the surrounding
// content, with continuation paragraphs flush to column 0. The
// trailing space is purely cosmetic — it is stripped during folding —
// but matching it here is the only way to land byte-equal with the
// okapi reference on multi-line single-quoted fixtures like
// `en (2).yml` and `en (5).yml`.
func encodeSingleQuoted(s string) string {
	escaped := strings.ReplaceAll(s, "'", "''")
	if !strings.Contains(escaped, "\n") {
		var b strings.Builder
		b.WriteByte('\'')
		b.WriteString(escaped)
		b.WriteByte('\'')
		return b.String()
	}
	var b strings.Builder
	b.WriteByte('\'')
	// Walk the value, splitting on runs of `\n`. Between two non-empty
	// content paragraphs separated by N value newlines, emit one
	// trailing space + (N+1) source newlines so the round-trip recovers
	// N value newlines after fold (snakeyaml-style output).
	i := 0
	for i < len(escaped) {
		// Collect a content run (non-newline bytes).
		j := i
		for j < len(escaped) && escaped[j] != '\n' {
			j++
		}
		if j > i {
			b.WriteString(escaped[i:j])
		}
		i = j
		if i >= len(escaped) {
			break
		}
		// Count consecutive newlines.
		nlStart := i
		for i < len(escaped) && escaped[i] == '\n' {
			i++
		}
		n := i - nlStart // number of value newlines in this run
		// Trailing space on the preceding content line, then N+1
		// source newlines so the run folds back to N value newlines.
		// Skip the trailing space when the preceding content already
		// ended with one or when there is no preceding content (run at
		// the very start of the value).
		if nlStart > 0 && escaped[nlStart-1] != ' ' && escaped[nlStart-1] != '\t' {
			b.WriteByte(' ')
		}
		for range n + 1 {
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// encodePlain returns the text as a plain scalar.
func encodePlain(s string) string {
	return s
}

// encodeLiteralBlockWithIndicatorIndent emits a literal block scalar
// (| style) using the given indicator line and explicit content indent
// (decimal string). Empty indicator falls back to bare `|`; empty indent
// falls back to "  " (2 spaces).
func encodeLiteralBlockWithIndicatorIndent(s, indicator, indent string) string {
	if indicator == "" {
		indicator = "|"
	}
	pad := indentPad(indent)
	if !strings.Contains(s, "\n") {
		return indicator + "\n" + pad + s + "\n"
	}
	var b strings.Builder
	b.WriteString(indicator)
	b.WriteByte('\n')
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			continue
		}
		// Blank lines stay blank — emitting `pad + "\n"` would leave
		// trailing whitespace, which okapi's writer (and a strict YAML
		// linter) avoid. The block-scalar indent still applies to
		// content lines but is omitted on truly empty lines.
		if line == "" {
			b.WriteByte('\n')
			continue
		}
		b.WriteString(pad)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// encodeFoldedBlockWithIndicatorIndent emits a folded block scalar
// (> style) using the given indicator and explicit content indent
// (decimal string). Empty indicator falls back to bare `>`; empty indent
// falls back to "  " (2 spaces).
//
// Folded scalars (`>`) collapse single line breaks between content
// lines into spaces and preserve blank-line gaps as single `\n`s in
// the parsed value. Re-emitting therefore can NOT simply mirror value
// lines back as source lines: a value of `"para1\npara2"` must come
// out as two source lines separated by a blank line, otherwise it
// re-folds to `"para1 para2"` — which is the divergence the
// folded_indented and folded_literal_examples parity fixtures hit.
//
// Encoding rule (per YAML 1.2 §8.1.3 folding semantics):
//   - For each `\n` in the value between two non-empty value lines,
//     source needs one blank line. We have already terminated the
//     previous source line with `\n`, so for N `\n`s in value we add
//     N more `\n`s before the next content line.
//   - More-indented value lines (lines starting with whitespace, which
//     yaml.v3 returns at the relative indent past the block's content
//     indent) absorb one `\n` via the indentation itself — the
//     leading newline before a more-indented run is preserved without
//     a blank line. So we add (N - 1) blank `\n`s instead.
//   - Clip-chomp `>` (and the bare `>` default) preserves a single
//     trailing newline if and only if the source body ended with one.
//     If the parsed value lacks a trailing `\n`, suppress the
//     terminator after the last content line so the chomp shape
//     round-trips.
func encodeFoldedBlockWithIndicatorIndent(s, indicator, indent string) string {
	if indicator == "" {
		indicator = ">"
	}
	pad := indentPad(indent)
	endsWithNewline := strings.HasSuffix(s, "\n")
	if !strings.Contains(s, "\n") {
		// Single-line bodies always need a final terminator so the
		// next sibling key (or EOF) lands on its own line.
		return indicator + "\n" + pad + s + "\n"
	}
	var b strings.Builder
	b.WriteString(indicator)
	b.WriteByte('\n')
	lines := strings.Split(s, "\n")
	// Drop a trailing empty entry from a value ending in `\n` — that
	// trailing newline is encoded by the final content line's
	// terminator, not by an additional blank source line.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	// Find the index of the last non-empty entry so we can suppress the
	// trailing `\n` after it when the value didn't end with one.
	lastNonEmptyIdx := -1
	for i, line := range lines {
		if line != "" {
			lastNonEmptyIdx = i
		}
	}
	prevNonEmptyIdx := -1
	for i, line := range lines {
		if line == "" {
			continue
		}
		isMoreIndented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		if prevNonEmptyIdx >= 0 {
			n := i - prevNonEmptyIdx // # of \n's in value between previous non-empty and this one
			gap := n
			if isMoreIndented {
				gap--
			}
			for range gap {
				b.WriteByte('\n')
			}
		}
		b.WriteString(pad)
		b.WriteString(line)
		// Suppress the final terminator when the value didn't end with
		// a newline, so re-parsing yields the same chomp shape.
		if i != lastNonEmptyIdx || endsWithNewline {
			b.WriteByte('\n')
		}
		prevNonEmptyIdx = i
	}
	return b.String()
}

// applyEOL rewrites every line break in s to the given line-ending
// convention. It first normalises any CR / CRLF to LF, then replaces
// each LF with eol — the same ordered normalise-then-replay sequence
// Okapi's YamlSkeletonWriter uses (replaceAll "\r\n"→"\n", "\r"→"\n",
// "\n"→getLineBreak()). The pre-normalisation makes it idempotent: a
// scalar that already carries the target EOL is left unchanged.
func applyEOL(s, eol string) string {
	if eol == "" || eol == "\n" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", eol)
}

// indentPad converts a decimal indent string (e.g. "12") to a space
// string. Empty / unparseable returns the legacy 2-space default.
func indentPad(indent string) string {
	if indent == "" {
		return "  "
	}
	n, err := strconv.Atoi(indent)
	if err != nil || n <= 0 {
		return "  "
	}
	return strings.Repeat(" ", n)
}

func (w *Writer) flush() error {
	if w.Output == nil || len(w.blocks) == 0 {
		return nil
	}

	// Build a yaml.Node tree directly so that mapping key order matches
	// the source document's order. yaml.v3 preserves the slice order of
	// MappingNode.Content (alternating key/value pairs); the previous
	// `map[string]any` approach lost order to Go's randomized map
	// iteration. w.blocks holds them in the order they arrived from the
	// reader, which mirrors the source document.
	//
	// One entry per block, not per distinct key path. A document that
	// repeats a key is malformed per YAML 1.2 and every real consumer is
	// last-wins, but it is the document kapi was handed: reproducing it is
	// this path's job, and collapsing the repeat here would silently drop a
	// string that had already been translated.
	root := &yamlv3.Node{Kind: yamlv3.MappingNode, Tag: "!!map"}
	for _, block := range w.blocks {
		value := &yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!str", Value: w.blockText(block)}
		// A value that begins with a newline drifts through the default
		// (auto) style: the stock yaml.v3 encoder picks a literal block
		// scalar (`|2-`, `|2`) whose first content line is emitted
		// immediately, without the leading blank line, so the leading
		// "\n" is silently dropped on re-read. That corrupts the
		// content-derived block identity (AD-036) and breaks content-memory
		// matches after a conversion. A double-quoted scalar escapes the
		// newline (`"\nx"`) and round-trips the leading "\n" byte-for-byte.
		// Only the leading-newline case is remapped — interior and trailing
		// newlines and plain values already round-trip under the default
		// style and stay byte-stable.
		if strings.HasPrefix(value.Value, "\n") {
			value.Style = yamlv3.DoubleQuotedStyle
		}
		root.Content = append(root.Content,
			&yamlv3.Node{Kind: yamlv3.ScalarNode, Tag: "!!str", Value: block.Name},
			value,
		)
	}

	encoder := yamlv3.NewEncoder(w.Output)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return fmt.Errorf("yaml writer: encoding: %w", err)
	}
	return encoder.Close()
}

// A non-translatable block is the source's own bytes, whatever target it is
// carrying. `translatable: false` is the reader's answer to "may this be
// rewritten", derived from the collection's extraction rules, and a target on
// such a block is leftover state — written when the rules were broader, or by a
// pass that did not consult them — not a translation someone decided on.
//
// Writing it out is how `tools.case-transform.category: "text-processing"`
// became `"tekstbehandling"` in a committed catalog: an identifier that a
// group-by splits on and an overlay matches its master by, rewritten into
// something that matches nothing. The extraction rule was already right and the
// blocks were already marked correctly; nothing asked them on the way out.
func (w *Writer) blockText(block *model.Block) string {
	// RenderRunsWithData splices inline-code Data back into the text
	// stream — required when the reader's codeFinder split the value
	// into TextRun + Ph runs. plain SourceText/TargetText drops Ph
	// runs so the placeholders would vanish on round-trip.
	if !w.Locale.IsEmpty() && block.Translatable && block.HasTarget(w.Locale) {
		return model.RenderRunsWithData(block.TargetRuns(w.Locale))
	}
	return model.RenderRunsWithData(block.Source)
}
