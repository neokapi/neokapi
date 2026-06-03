package mif_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/formats/mif"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readMIF drains a reader for the given input and reports how many blocks and
// errors surfaced on the channel. It never calls t.Fatal on a result.Error (as
// testutil.CollectBlocks would), so it can observe the error path of malformed
// input directly. Both Open and the channel drain run inside require.NotPanics
// so a parser crash fails the test loudly instead of taking down the suite.
func readMIF(t *testing.T, ctx context.Context, input string) (blocks, errs int) {
	t.Helper()
	reader := mif.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
		require.NoError(t, err)
	})
	defer reader.Close()

	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			switch {
			case result.Error != nil:
				errs++
			case result.Part != nil && result.Part.Type == model.PartBlock:
				blocks++
			}
		}
	})
	return blocks, errs
}

// TestReadMalformedNoPanic feeds a battery of broken, truncated, and garbage
// MIF inputs and asserts the reader degrades gracefully: it must never panic
// (parse a partial tree, drop unreadable content, or surface a clean error on
// the channel — but not crash). This is the L1->L2 malformed-input contract.
//
// The MIF parser (parseMIF) is deliberately forgiving — an unclosed statement
// is flushed defensively rather than rejected — so truncated/unterminated input
// yields a best-effort partial extraction with no error. Version validation
// (validateMIFVersion, mirroring okapi Document.Version) is the one hard gate:
// a <MIFFile> header with a missing or sub-8.0 version is rejected with a
// PartResult.Error before any Part is emitted. Garbage that carries no
// <MIFFile> header skips the version gate and simply extracts nothing.
func TestReadMalformedNoPanic(t *testing.T) {
	t.Parallel()

	bt := "`" // backtick: opens a MIF string literal.

	tests := []struct {
		name string
		// input is the raw MIF document fed to the reader.
		input string
		// wantErr is true when a PartResult.Error must surface; false when a
		// graceful empty/partial extraction (no error) is the contract.
		wantErr bool
	}{
		{
			// Truncated mid-document: the <String> value never closes and the
			// trailing containers are never popped. parseMIF flushes the open
			// stack, so a best-effort partial extraction comes back with no
			// error and no panic.
			name: "truncated document",
			input: "<MIFFile 2015>\n<TextFlow\n <Para\n  <ParaLine\n   <String " +
				bt + "Hello wor",
			wantErr: false,
		},
		{
			// Unterminated <String literal: the backtick opens a value that
			// has no closing quote before end-of-input. The parser keeps it
			// as literal text rather than crashing.
			name: "unterminated String literal",
			input: "<MIFFile 2015>\n<TextFlow\n <Para\n  <ParaLine\n   <String " +
				bt + "Hello world.\n  >\n >\n>\n",
			wantErr: false,
		},
		{
			// <MIFFile> with no version token. mifFileVersion reports an empty
			// version, which validateMIFVersion rejects ("Unsupported document
			// version: ") before emitting any Part.
			name: "MIFFile without version",
			input: "<MIFFile>\n<TextFlow\n <Para\n  <ParaLine\n   <String " +
				bt + "Hi.'>\n  >\n >\n>\n",
			wantErr: true,
		},
		{
			// <MIFFile > with a trailing space but still no version number:
			// also rejected by the version gate.
			name:    "MIFFile with empty version",
			input:   "<MIFFile >\n",
			wantErr: true,
		},
		{
			// Garbage non-MIF bytes (NUL/control bytes + random brackets).
			// There is no <MIFFile> header, so the version gate is skipped and
			// nothing is extractable: an empty extraction with no error.
			name:    "garbage non-MIF bytes",
			input:   "\x00\x01\x02 this is not a mif file at all }{][><",
			wantErr: false,
		},
		{
			// Sub-8.0 version: FrameMaker 7 and earlier use an unsupported
			// structure/encoding. validateMIFVersion rejects 7.00 (< 8.0) with
			// a PartResult.Error, mirroring okapi's OkapiBadFilterInputException.
			name: "sub-8.0 version",
			input: "<MIFFile 7.00>\n<TextFlow\n <Para\n  <ParaLine\n   <String " +
				bt + "Hi.'>\n  >\n >\n>\n",
			wantErr: true,
		},
		{
			// An even older 6.00 version, also below the 8.0 floor.
			name:    "very old version header only",
			input:   "<MIFFile 6.00>\n",
			wantErr: true,
		},
		{
			// Only the version header, supported version, no content: a valid
			// but empty document. No error, no blocks.
			name:    "supported version no content",
			input:   "<MIFFile 2015>\n",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			blocks, errs := readMIF(t, ctx, tt.input)
			if tt.wantErr {
				assert.Positive(t, errs,
					"expected a clean PartResult.Error for malformed input")
			} else {
				assert.Zero(t, errs,
					"forgiving parse must not surface a spurious error")
				_ = blocks // partial/empty extraction both acceptable; key is no panic.
			}
		})
	}
}

// TestReadMalformedNilInputs verifies Open rejects a nil document and a
// document with a nil reader without panicking — the parse pipeline never gets
// a chance to run, so the rejection must happen at Open.
func TestReadMalformedNilInputs(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("nil document", func(t *testing.T) {
		t.Parallel()
		reader := mif.NewReader()
		require.NotPanics(t, func() {
			err := reader.Open(ctx, nil)
			require.Error(t, err)
		})
	})

	t.Run("nil reader", func(t *testing.T) {
		t.Parallel()
		reader := mif.NewReader()
		require.NotPanics(t, func() {
			err := reader.Open(ctx, &model.RawDocument{Reader: nil})
			require.Error(t, err)
		})
	})
}
