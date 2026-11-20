package yaml

import (
	"context"
	"fmt"
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
	return format.BufferedSkeletonWrite(store, blocks, w.Output, w.renderRef, nil)
}

// renderRef returns the bytes a SkeletonRef contributes for the given block.
// A nil block contributes nothing, matching a map miss.
func (w *Writer) renderRef(block *model.Block) ([]byte, error) {
	if block == nil {
		return nil, nil
	}
	text := w.blockText(block)
	// Re-emit the scalar's original bytes while they still spell
	// exactly the text being written — byte-exact output for a block
	// nothing touched. The test is against the text the reader
	// recorded with those bytes, NOT against block.SourceText():
	// after a source edit the block's source IS the new text, so
	// that comparison always held and the pre-edit bytes won,
	// discarding the edit (#1473).
	if raw, ok := format.VerbatimFor(block, "yaml.raw", text); ok {
		return []byte(raw), nil
	}
	style := block.Properties["yaml.style"]
	indicator := block.Properties["yaml.indicator"]
	indent := block.Properties["yaml.indent"]
	encoded := encodeYAMLScalarWithIndicatorIndent(text, style, indicator, indent)
	// The scalar encoders emit bare LF for multi-line bodies (block
	// scalars, multi-line quoted strings). When the source uses CRLF the
	// surrounding skeleton already carries CRLF, so a re-encoded scalar
	// emitting LF would mix conventions within one document. Rewrite to
	// the source's dominant line ending — mirroring Okapi's
	// YamlSkeletonWriter, which normalises to LF then replays
	// getLineBreak() on every break. Empty / "\n" eol leaves the
	// LF-source common case untouched.
	if eol := block.Properties["yaml.eol"]; eol == "\r\n" {
		encoded = applyEOL(encoded, eol)
	}
	return []byte(encoded), nil
}

// encodeYAMLScalarWithIndicatorIndent uses the source style and indentation
// when they can represent the edited text.
func encodeYAMLScalarWithIndicatorIndent(text, style, indicator, indent string) string {
	if style == "literal" || style == "folded" {
		return encodeBlockScalar(text, style, indicator, indent)
	}
	if needsDoubleQuoting(text) {
		return encodeDoubleQuoted(text)
	}
	switch style {
	case "double-quoted":
		return encodeDoubleQuoted(text)
	case "single-quoted":
		return encodeSingleQuoted(text)
	default:
		if !plainScalarSafe(text) || !plainReadsAsString(text) {
			return encodeDoubleQuoted(text)
		}
		return encodePlain(text)
	}
}

// plainReadsAsString reports whether s written plain reads back as the
// string s, where the scalar's style was plain only because the text it held
// was: a translation such as 10, true or no, written plain, reads as a number
// or a boolean, which no reader takes as text. It holds under YAML 1.2, as
// kapi reads a file, and for the booleans and null a YAML 1.1 reader (Ruby's
// Psych, PyYAML) takes, yes, no, on and off among them. A value that does not
// look like a number is text; one that does is parsed to decide.
func plainReadsAsString(s string) bool {
	switch strings.ToLower(s) {
	case "y", "n", "yes", "no", "on", "off", "true", "false", "null", "~":
		return false
	}
	return !yaml11NonString(s) || plainIsString(s)
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

// encodeBlockScalar keeps the indicator comment and the line boundary before
// the next YAML node, including when the value requires double quotes.
func encodeBlockScalar(text, style, indicator, indent string) string {
	if indicator == "" {
		indicator = "|"
		if style == "folded" {
			indicator = ">"
		}
	}
	end := strings.IndexAny(indicator, " \t\r")
	if end < 0 {
		end = len(indicator)
	}
	header, comment := indicator[:end], indicator[end:]
	first := strings.TrimLeft(text, "\n")
	if needsDoubleQuoting(text) || (isMoreIndentedLine(first) && !strings.ContainsAny(header, "123456789")) {
		return encodeDoubleQuoted(text) + comment + "\n"
	}

	body := strings.TrimRight(text, "\n")
	trailing := len(text) - len(body)
	chomp := ""
	switch {
	case trailing == 0:
		chomp = "-"
	case trailing > 1 || body == "" || strings.Contains(header, "+"):
		chomp = "+"
	}
	if chomp == "" || !strings.Contains(header, chomp) {
		header = strings.ReplaceAll(strings.ReplaceAll(header, "-", ""), "+", "") + chomp
	}

	var out strings.Builder
	out.WriteString(header)
	out.WriteString(comment)
	out.WriteByte('\n')
	pad := indentPad(indent)
	var previous string
	if body != "" {
		for line := range strings.SplitSeq(body, "\n") {
			if line != "" {
				// YAML folds a break between two ordinary lines to a space.
				if style == "folded" && previous != "" && !isMoreIndentedLine(previous) && !isMoreIndentedLine(line) {
					out.WriteByte('\n')
				}
				out.WriteString(pad)
				out.WriteString(line)
				previous = line
			}
			out.WriteByte('\n')
		}
		trailing--
	}
	for range trailing {
		out.WriteByte('\n')
	}
	return out.String()
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
	return model.RenderRunsWithData(format.AuthoritativeRuns(block))
}
