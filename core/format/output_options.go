package format

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

// BOMMode controls the byte-order-mark policy for writer output.
type BOMMode string

// BOM policy values for OutputOptions.BOM.
const (
	BOMKeep   BOMMode = "keep"   // leave the stream as the writer produced it (default)
	BOMAdd    BOMMode = "add"    // ensure the output starts with a BOM
	BOMRemove BOMMode = "remove" // strip a leading BOM if the writer emitted one
)

// NewlineMode controls line-break normalization for writer output.
type NewlineMode string

// Newline policy values for OutputOptions.Newline.
const (
	NewlineKeep NewlineMode = "keep" // leave line breaks as the writer produced them (default)
	NewlineLF   NewlineMode = "lf"   // normalize CRLF / lone CR to LF
	NewlineCRLF NewlineMode = "crlf" // normalize LF / lone CR to CRLF
)

// OutputOptions are the byte-level output options shared by every format
// writer (AD-005). Readers already normalize BOM / charset / newlines at
// parse time; these options control the corresponding *output* style, applied
// as a post-encode step on the writer's byte stream. They are set via the
// reserved "output" key of the ordinary format-config mechanism
// (`defaults.formats[<id>].config` in a .kapi recipe):
//
//	defaults:
//	  formats:
//	    plaintext:
//	      config:
//	        output:
//	          bom: remove      # add|remove|keep (default keep)
//	          newline: crlf    # lf|crlf|keep (default keep)
//	          encoding: windows-1252  # any core/encoding charset (default utf-8)
//
// Dotted keys ("output.bom": "add") are accepted as an alias for the nested
// map. The zero value (or keep/keep/utf-8) is a full passthrough.
type OutputOptions struct {
	BOM      BOMMode
	Newline  NewlineMode
	Encoding string
}

// IsZero reports whether the options are a full passthrough (nothing to do).
func (o OutputOptions) IsZero() bool {
	return (o.BOM == "" || o.BOM == BOMKeep) &&
		(o.Newline == "" || o.Newline == NewlineKeep) &&
		isUTF8Name(o.Encoding)
}

// isUTF8Name reports whether name is empty or an alias of UTF-8.
func isUTF8Name(name string) bool {
	n := strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(name), "-", ""), "_", "")
	return n == "" || n == "utf8"
}

// OutputConfigurable is implemented by writers that accept the shared
// byte-level output options. format.BaseFormatWriter implements it, so every
// writer that embeds the base inherits the behavior without per-format code.
type OutputConfigurable interface {
	SetOutputOptions(opts OutputOptions) error
}

// EncodingResolver resolves a charset name to an x/text encoding. It is
// registered by core/encoding at init time — core/format cannot import
// core/encoding directly (core/encoding depends on core/format for
// diagnostics), so the charset registry is injected here instead.
var EncodingResolver func(name string) (encoding.Encoding, error)

// OutputConfigKey is the reserved format-config key that carries the shared
// writer output options ("output"). SplitOutputConfig consumes it (and its
// dotted "output.*" aliases) before per-format config is applied, so format
// reader/writer configs never see it as an unknown key.
const OutputConfigKey = "output"

// SplitOutputConfig extracts the shared writer output options from a format
// config map and returns them together with the remaining format-specific
// keys. It accepts both the nested map form ("output": {bom, newline,
// encoding}) and dotted keys ("output.bom", "output.newline",
// "output.encoding"). Unknown output sub-keys and invalid enum values return
// an error. A nil map yields passthrough options and a nil remainder.
func SplitOutputConfig(values map[string]any) (OutputOptions, map[string]any, error) {
	opts := OutputOptions{BOM: BOMKeep, Newline: NewlineKeep}
	if values == nil {
		return opts, nil, nil
	}
	rest := make(map[string]any, len(values))
	for k, v := range values {
		switch {
		case k == OutputConfigKey:
			m, ok := v.(map[string]any)
			if !ok {
				return opts, nil, fmt.Errorf("config key %q: expected a map of output options, got %T", k, v)
			}
			for sub, sv := range m {
				if err := opts.setOption(sub, sv); err != nil {
					return opts, nil, err
				}
			}
		case strings.HasPrefix(k, OutputConfigKey+"."):
			if err := opts.setOption(strings.TrimPrefix(k, OutputConfigKey+"."), v); err != nil {
				return opts, nil, err
			}
		default:
			rest[k] = v
		}
	}
	return opts, rest, nil
}

// setOption applies one output.* sub-key.
func (o *OutputOptions) setOption(key string, value any) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("output.%s: expected a string, got %T", key, value)
	}
	switch key {
	case "bom":
		switch BOMMode(s) {
		case BOMKeep, BOMAdd, BOMRemove:
			o.BOM = BOMMode(s)
		default:
			return fmt.Errorf("output.bom: %q is not one of add|remove|keep", s)
		}
	case "newline":
		switch NewlineMode(s) {
		case NewlineKeep, NewlineLF, NewlineCRLF:
			o.Newline = NewlineMode(s)
		default:
			return fmt.Errorf("output.newline: %q is not one of lf|crlf|keep", s)
		}
	case "encoding":
		o.Encoding = s
	default:
		return fmt.Errorf("output.%s: unknown output option (want bom, newline, or encoding)", key)
	}
	return nil
}

// Wrap layers the configured post-encode steps over w. The writer produces
// UTF-8 bytes; the chain applies, in order: newline normalization, the BOM
// policy (in UTF-8 space, so a kept/added U+FEFF is converted with the rest of
// the stream), then charset conversion via the core/encoding registry. A
// byte-order mark the charset cannot carry keeps the output UTF-8: the mark is
// the evidence the next read settles the encoding by, so it wins over the
// configured charset on this side as it does on the reading side. Close
// flushes the chain but does not close w. Returns an error for an unknown
// charset or when no encoding resolver is registered.
func (o OutputOptions) Wrap(w io.Writer) (io.WriteCloser, error) {
	pipe := &outputPipeline{Writer: w}
	if !isUTF8Name(o.Encoding) {
		if EncodingResolver == nil {
			return nil, errors.New("output.encoding: no charset registry available (core/encoding is not linked)")
		}
		enc, err := EncodingResolver(o.Encoding)
		if err != nil {
			return nil, fmt.Errorf("output.encoding: %w", err)
		}
		cw := &charsetWriter{next: pipe.Writer, enc: enc, name: o.Encoding}
		pipe.Writer = cw
		pipe.closers = append(pipe.closers, cw)
	}
	if o.BOM == BOMAdd || o.BOM == BOMRemove {
		bw := &bomWriter{next: pipe.Writer, add: o.BOM == BOMAdd}
		pipe.Writer = bw
		pipe.closers = append(pipe.closers, bw)
	}
	if o.Newline == NewlineLF || o.Newline == NewlineCRLF {
		nw := &newlineWriter{next: pipe.Writer, crlf: o.Newline == NewlineCRLF}
		pipe.Writer = nw
		pipe.closers = append(pipe.closers, nw)
	}
	return pipe, nil
}

// outputPipeline is the assembled post-encode chain. Close flushes each stage
// outermost-first; the underlying destination is left open.
type outputPipeline struct {
	io.Writer
	closers []io.Closer // innermost first
}

func (p *outputPipeline) Close() error {
	var firstErr error
	for _, v := range slices.Backward(p.closers) {
		if err := v.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	p.closers = nil
	return firstErr
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// charsetWriter encodes the UTF-8 stream in the output charset. It holds the
// first bytes until it can tell whether the writer emitted a byte-order mark:
// a mark the charset cannot carry (any single-byte or multi-byte charset that
// is not a Unicode form) keeps the output UTF-8, mark included.
type charsetWriter struct {
	next    io.Writer
	enc     encoding.Encoding
	name    string    // the charset, for the error a text it cannot carry raises
	out     io.Writer // the destination once decided: the encoder or next
	closer  io.Closer // the encoder, when one is in use
	head    []byte    // buffered head bytes (< 3) while undecided
	decided bool
}

func (c *charsetWriter) Write(p []byte) (int, error) {
	if c.decided {
		n, err := c.out.Write(p)
		return n, c.wrap(err)
	}
	total := len(p)
	c.head = append(c.head, p...)
	if len(c.head) < len(utf8BOM) && strings.HasPrefix(string(utf8BOM), string(c.head)) {
		return total, nil
	}
	if err := c.decide(); err != nil {
		return 0, c.wrap(err)
	}
	return total, nil
}

// wrap names the charset in an error the encoder raised, so a text the charset
// cannot carry is reported as such rather than as a bare transform failure.
func (c *charsetWriter) wrap(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("write in %s: %w", c.name, err)
}

// decide picks the destination from the buffered head and forwards it.
func (c *charsetWriter) decide() error {
	c.decided = true
	head := c.head
	c.head = nil
	if bytes.HasPrefix(head, utf8BOM) && !canEncode(c.enc, UTF8BOM) {
		c.out = c.next
	} else {
		tw := transform.NewWriter(c.next, c.enc.NewEncoder())
		c.out, c.closer = tw, tw
	}
	if len(head) == 0 {
		return nil
	}
	_, err := c.out.Write(head)
	return err
}

// Close decides a still-buffered head (streams shorter than three bytes) and
// flushes the encoder.
func (c *charsetWriter) Close() error {
	if !c.decided {
		if err := c.decide(); err != nil {
			return err
		}
	}
	if c.closer == nil {
		return nil
	}
	return c.wrap(c.closer.Close())
}

// canEncode reports whether enc can represent s.
func canEncode(enc encoding.Encoding, s string) bool {
	_, err := enc.NewEncoder().String(s)
	return err == nil
}

// bomWriter enforces the BOM policy on a UTF-8 stream. It buffers up to the
// first three bytes to detect a writer-emitted BOM, then either guarantees
// exactly one leading BOM (add) or none (remove) and passes the rest through.
type bomWriter struct {
	next    io.Writer
	add     bool
	started bool   // leading-BOM decision made
	prefix  []byte // buffered head bytes (< 3) while undecided
}

func (b *bomWriter) Write(p []byte) (int, error) {
	if b.started {
		return b.next.Write(p)
	}
	total := len(p)
	b.prefix = append(b.prefix, p...)
	if len(b.prefix) < len(utf8BOM) {
		// Not enough bytes yet to decide; keep buffering.
		if strings.HasPrefix(string(utf8BOM), string(b.prefix)) {
			return total, nil
		}
		// Head can no longer be a BOM: decide now.
	}
	if err := b.flushHead(); err != nil {
		return 0, err
	}
	return total, nil
}

// flushHead makes the leading-BOM decision and forwards the buffered head.
func (b *bomWriter) flushHead() error {
	head := b.prefix
	b.prefix = nil
	b.started = true
	hasBOM := len(head) >= len(utf8BOM) && string(head[:len(utf8BOM)]) == string(utf8BOM)
	if hasBOM {
		head = head[len(utf8BOM):]
	}
	if b.add {
		if _, err := b.next.Write(utf8BOM); err != nil {
			return err
		}
	}
	if len(head) == 0 {
		return nil
	}
	_, err := b.next.Write(head)
	return err
}

// Close flushes a still-buffered head (streams shorter than three bytes).
func (b *bomWriter) Close() error {
	if b.started {
		return nil
	}
	return b.flushHead()
}

// newlineWriter converts line breaks (LF, CRLF, lone CR) to the configured
// style. It is chunk-boundary safe: a trailing CR is held until the next
// write (or Close) decides whether it belongs to a CRLF pair.
type newlineWriter struct {
	next      io.Writer
	crlf      bool
	pendingCR bool
}

func (n *newlineWriter) Write(p []byte) (int, error) {
	total := len(p)
	out := make([]byte, 0, len(p)+len(p)/8)
	emit := func() {
		if n.crlf {
			out = append(out, '\r', '\n')
		} else {
			out = append(out, '\n')
		}
	}
	for _, c := range p {
		if n.pendingCR {
			n.pendingCR = false
			emit() // the held CR is a break (CRLF collapses with the LF below)
			if c == '\n' {
				continue
			}
		}
		switch c {
		case '\r':
			n.pendingCR = true
		case '\n':
			emit()
		default:
			out = append(out, c)
		}
	}
	if len(out) > 0 {
		if _, err := n.next.Write(out); err != nil {
			return 0, err
		}
	}
	return total, nil
}

// Close flushes a held trailing CR as one final line break.
func (n *newlineWriter) Close() error {
	if !n.pendingCR {
		return nil
	}
	n.pendingCR = false
	br := []byte{'\n'}
	if n.crlf {
		br = []byte{'\r', '\n'}
	}
	_, err := n.next.Write(br)
	return err
}
