package main

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/safeio"
	"github.com/stretchr/testify/require"
)

func newRegistry() *registry.FormatRegistry {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return reg
}

// TestClassifyGoodFile: a well-formed file reads (and where a writer exists,
// round-trips) cleanly — OK or OK_ROUNDTRIP, never a reject or crash.
func TestClassifyGoodFile(t *testing.T) {
	reg := newRegistry()
	budget := safeio.DefaultBudget()

	cases := []struct {
		id   string
		data string
	}{
		{"plaintext", "Hello world\nSecond line\n"},
		{"json", "{\n  \"title\": \"Hello World\",\n  \"desc\": \"A simple test\"\n}\n"},
		{"properties", "greeting = Hello World\nfarewell = Goodbye\n"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			cls, detail := classify(reg, c.id, []byte(c.data), budget)
			require.Contains(t, []Classification{OK, OKRoundTrip}, cls,
				"format %s should read/round-trip cleanly; detail=%s", c.id, detail)
		})
	}
}

// TestClassifyGarbageGraceful: truncated/garbage input is handled without
// escaping a panic — the result is a graceful reject (EXPECTED_REJECT), a
// drift, or a recovered CRASH, but the harness never falls over.
func TestClassifyGarbageGraceful(t *testing.T) {
	reg := newRegistry()
	budget := safeio.DefaultBudget()

	cases := []struct {
		name string
		id   string
		data string
	}{
		{"truncated-json", "json", `{"truncated": `},
		{"binary-as-json", "json", "\x00\x01\x02\xff\xfe not json at all"},
		{"malformed-xml", "xml", "<a><b></a>"},
		{"binary-as-xml", "xml", "\x00\x01\x02\x03\x04not xml"},
	}
	ok := []Classification{OK, OKRoundTrip, ExpectedReject, RoundtripDrift, Crash}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cls, _ := classify(reg, c.id, []byte(c.data), budget)
			require.Contains(t, ok, cls)
		})
	}
}

// TestClassifyUnknownFormat: an unknown format id is surfaced as CRASH (a
// harness/config error made visible), not silently miscounted.
func TestClassifyUnknownFormat(t *testing.T) {
	reg := newRegistry()
	cls, detail := classify(reg, "no-such-format", []byte("x"), safeio.DefaultBudget())
	require.Equal(t, Crash, cls)
	require.Contains(t, detail, "new reader")
}

// TestClassifyByteBudgetGuard: the byte budget bounds the read; an input larger
// than the cap is rejected on the channel rather than fully buffered.
func TestClassifyByteBudgetGuard(t *testing.T) {
	reg := newRegistry()
	// 64 KiB of plain text under a tiny 1 KiB byte budget.
	big := make([]byte, 64*1024)
	for i := range big {
		big[i] = 'a'
	}
	budget := safeio.DefaultBudget().WithMaxBytes(1024)
	cls, detail := classify(reg, "plaintext", big, budget)
	require.Equal(t, ExpectedReject, cls, "over-budget input should reject cleanly; detail=%s", detail)
}
