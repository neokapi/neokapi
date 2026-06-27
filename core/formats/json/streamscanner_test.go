package json

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

// scanStream tokenises input via the streaming scanner into a slice (for parity
// comparison only — production streaming consumes next() one token at a time).
func scanStream(t *testing.T, input string) []token {
	t.Helper()
	s := newStreamScanner(strings.NewReader(input))
	var toks []token
	for {
		tok, err := s.next()
		if err != nil {
			t.Fatalf("stream scan error: %v", err)
		}
		toks = append(toks, tok)
		if tok.typ == tokenEOF {
			return toks
		}
	}
}

// TestStreamScannerParity asserts the streaming scanner emits the exact same
// token sequence (type, raw, value, prefix) as the buffered scanner across
// representative well-formed JSON / JSON5 — the property that lets a streaming
// JSON read stay byte-exact while driving the same forward, ancestor-only walk.
func TestStreamScannerParity(t *testing.T) {
	inputs := []string{
		`{}`,
		`[]`,
		`{"a":"b"}`,
		`{ "a" : "b" , "c" : 1 }`,
		"{\n  \"nested\": { \"x\": [1, 2, 3], \"y\": true },\n  \"z\": null\n}\n",
		`{"s":"line\nbreak\ttab \"quote\" \\ slash\/ é 😀"}`,
		`{"n":[-0,0,1.5,-3e10,2E-7,100]}`,
		`{"unicode":"café — naïve","nb":"a b"}`,
		"{\n  // line comment\n  \"a\": 1, /* block */ \"b\": 2,\n  # hash comment\n  \"c\": 3\n}",
		`{'single':'quoted','bare':ident,"mixed":'v'}`,
		"\ufeff{\"bom\":\"leading\"}",
		`{"deep":{"a":{"b":{"c":{"d":"e"}}}}}`,
		`[{"k":"v"},{"k":"w"},[1,[2,[3]]]]`,
		`<!-- html comment -->{"a":1}`,
		`{"empty":"","ws":"  spaced  "}`,
	}
	for _, in := range inputs {
		buffered, err := newScanner([]byte(in)).scan()
		if err != nil {
			t.Fatalf("buffered scan error for %q: %v", in, err)
		}
		stream := scanStream(t, in)
		if len(buffered) != len(stream) {
			t.Fatalf("token count mismatch for %q: buffered=%d stream=%d", in, len(buffered), len(stream))
		}
		for i := range buffered {
			b, s := buffered[i], stream[i]
			if b.typ != s.typ || b.raw != s.raw || b.value != s.value || b.prefix != s.prefix {
				t.Errorf("token %d mismatch for %q:\n buffered=%+v\n stream  =%+v", i, in, b, s)
			}
		}
	}
}

// genJSONReader emits a large JSON object of n entries on demand via an
// io.Pipe, so the document never exists as a whole buffer — the streaming
// scanner's peak memory therefore reflects only its own working set, not the
// document size. docBytes returns the exact byte length the generator produces.
func genJSONReader(n int) (io.Reader, int) {
	size := len("{\n") + len("\n}\n")
	for i := range n {
		if i > 0 {
			size += len(",\n")
		}
		size += len(fmt.Sprintf("  \"key_%d\": \"a translatable value number %d with some words\"", i, i))
	}
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, "{\n")
		for i := range n {
			if i > 0 {
				_, _ = io.WriteString(pw, ",\n")
			}
			fmt.Fprintf(pw, "  \"key_%d\": \"a translatable value number %d with some words\"", i, i)
		}
		_, _ = io.WriteString(pw, "\n}\n")
		pw.Close()
	}()
	return pr, size
}

// TestStreamScannerBoundedMemory tokenises a large JSON document supplied as a
// pure stream (never materialised as a buffer) and asserts peak heap is a small,
// flat window — independent of document size. The buffered scanner would hold the
// whole []byte plus a []token slice, both scaling with the document.
func TestStreamScannerBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	peakTokenizing := func(n int) (peak uint64, docSize int) {
		r, size := genJSONReader(n)
		runtime.GC()
		var base runtime.MemStats
		runtime.ReadMemStats(&base)

		s := newStreamScanner(r)
		var m runtime.MemStats
		count := 0
		for {
			tok, err := s.next()
			if err != nil {
				t.Fatalf("stream scan: %v", err)
			}
			count++
			if count%256 == 0 {
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > base.HeapAlloc && m.HeapAlloc-base.HeapAlloc > peak {
					peak = m.HeapAlloc - base.HeapAlloc
				}
			}
			if tok.typ == tokenEOF {
				break
			}
		}
		return peak, size
	}

	ps, smallSize := peakTokenizing(10_000)
	pl, largeSize := peakTokenizing(200_000) // 20x larger document (~13 MiB)
	t.Logf("streaming tokenizer peakΔ: small(%d KiB doc)=%d KiB, large(%d KiB doc)=%d KiB",
		smallSize/1024, ps/1024, largeSize/1024, pl/1024)

	// Bounded window: a 20x larger document must not give a ~20x larger peak,
	// and the peak must stay far below the document size (a buffered tokenizer
	// would hold >= the whole document).
	if pl > uint64(largeSize)/4 {
		t.Errorf("streaming tokenizer peak %d B is not bounded well below doc size %d B", pl, largeSize)
	}
	if ps > 0 && pl > ps*3 {
		t.Errorf("streaming tokenizer peak scaled with input: small=%d B large=%d B (20x doc -> %.1fx peak)", ps, pl, float64(pl)/float64(ps))
	}
}
