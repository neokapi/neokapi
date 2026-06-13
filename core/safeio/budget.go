package safeio

import "io"

// Default limits. These are the single source of truth applied identically
// across the CLI, server, and WASM contexts (see the package doc). They are
// deliberately generous — large enough never to reject a legitimate
// localization document — while still bounding pathological input.
const (
	// DefaultMaxBytes caps the total bytes a single document read may consume.
	// 1 GiB is far above any real localization source file while still
	// bounding an unbounded stream.
	DefaultMaxBytes int64 = 1 << 30 // 1 GiB

	// DefaultMaxDepth caps recursion depth for recursive-descent parsers.
	// The go-yaml hardening (PR #515) showed a depth cap of 10,000 keeps
	// pathological documents sub-second; 1000 is comfortably above any real
	// document's nesting while preventing stack-exhaustion.
	DefaultMaxDepth = 1000

	// DefaultMaxEntrySize caps the uncompressed size of a single zip entry.
	// POI's ZipSecureFile defaults this to 4 GiB (the 32-bit zip maximum);
	// neokapi tightens it to 1 GiB, well above any real OOXML/EPUB/IDML part.
	DefaultMaxEntrySize int64 = 1 << 30 // 1 GiB

	// DefaultMaxTotalSize caps the cumulative uncompressed size of all entries
	// in a single archive.
	DefaultMaxTotalSize int64 = 4 << 30 // 4 GiB

	// DefaultMinInflateRatio is the minimum allowed ratio of compressed to
	// uncompressed bytes for a zip entry. POI's ZipSecureFile default is 0.01,
	// i.e. an entry that inflates more than 100:1 is treated as a zip bomb.
	DefaultMinInflateRatio = 0.01

	// DefaultMaxEntries caps the number of entries in a zip archive (an
	// entry-count flood guard; POI has no default here).
	DefaultMaxEntries = 100_000

	// graceEntrySize is the per-entry byte threshold below which the inflate-
	// ratio check is skipped, to avoid false positives on small,
	// legitimately-compressible entries. Mirrors POI's GRACE_ENTRY_SIZE
	// (100 KiB).
	graceEntrySize int64 = 100 << 10 // 100 KiB
)

// Budget bundles the byte, depth, and zip limits applied while parsing one
// document. The zero value is not useful; obtain a populated Budget from
// [DefaultBudget] and adjust with the With* methods. A Budget is small and
// copied by value.
type Budget struct {
	// MaxBytes is the total byte budget for a streaming read (0 disables the
	// byte cap).
	MaxBytes int64
	// MaxDepth is the maximum recursion depth (0 disables the depth cap).
	MaxDepth int
	// Zip holds the per-archive limits for zip-container formats.
	Zip ZipLimits
}

// DefaultBudget returns the canonical Budget. This is the single source of
// truth that CLI, server, and WASM readers share.
func DefaultBudget() Budget {
	return Budget{
		MaxBytes: DefaultMaxBytes,
		MaxDepth: DefaultMaxDepth,
		Zip:      DefaultZipLimits,
	}
}

// WithMaxBytes returns a copy of b with MaxBytes set to n.
func (b Budget) WithMaxBytes(n int64) Budget { b.MaxBytes = n; return b }

// WithMaxDepth returns a copy of b with MaxDepth set to n.
func (b Budget) WithMaxDepth(n int) Budget { b.MaxDepth = n; return b }

// WithZip returns a copy of b with Zip set to z.
func (b Budget) WithZip(z ZipLimits) Budget { b.Zip = z; return b }

// Reader wraps r so that reads beyond b.MaxBytes fail with a typed
// [LimitError]. When b.MaxBytes is 0, r is returned through a LimitedReader
// with no effective cap.
func (b Budget) Reader(r io.Reader) *LimitedReader { return NewLimitedReader(r, b.MaxBytes) }

// Writer wraps w so that writes beyond b.MaxBytes fail with a typed
// [LimitError].
func (b Budget) Writer(w io.Writer) *LimitedWriter { return NewLimitedWriter(w, b.MaxBytes) }

// DepthGuard returns a depth guard bounded by b.MaxDepth.
func (b Budget) DepthGuard() *DepthGuard { return NewDepthGuard(b.MaxDepth) }

// ZipGuard returns a stateful zip guard bounded by b.Zip, suitable for callers
// that iterate and stream every entry through one guard (so the cumulative
// total-size and entry-count caps apply across entries).
func (b Budget) ZipGuard() *ZipGuard { return b.Zip.NewGuard() }

// LimitedReader wraps an io.Reader and returns a typed [LimitError] (wrapping
// [ErrByteBudget]) once more than Max bytes have been read. Unlike
// io.LimitedReader — which returns io.EOF and silently truncates — this makes
// "input too large" distinguishable from "input ended", which matters when the
// truncated bytes would otherwise be parsed as a complete (wrong) document.
//
// A Max of 0 (or negative) disables the cap, so the reader is a transparent
// pass-through. The implementation reads at most one byte past the budget to
// detect an over-budget source without consuming it.
type LimitedReader struct {
	r   io.Reader
	max int64
	n   int64 // bytes read so far
	err error // sticky error once tripped
}

// NewLimitedReader returns a LimitedReader bounding r to max bytes.
func NewLimitedReader(r io.Reader, max int64) *LimitedReader {
	return &LimitedReader{r: r, max: max}
}

// Read implements io.Reader.
func (l *LimitedReader) Read(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	if l.max <= 0 {
		// No cap configured: pass through.
		return l.r.Read(p)
	}
	if len(p) == 0 {
		return 0, nil
	}
	// Read at most (remaining budget)+1 bytes, so a source with exactly max
	// bytes succeeds while an over-budget source is detected on the read that
	// would push us past max.
	remaining := l.max - l.n
	if int64(len(p)) > remaining+1 {
		p = p[:remaining+1]
	}
	n, err := l.r.Read(p)
	l.n += int64(n)
	if l.n > l.max {
		l.err = newLimitError(ErrByteBudget, l.max, l.n, "")
		// Discard the over-budget bytes; the error aborts the read.
		return 0, l.err
	}
	return n, err
}

// LimitedWriter wraps an io.Writer and returns a typed [LimitError] (wrapping
// [ErrByteBudget]) once more than Max bytes have been written. A Max of 0 (or
// negative) disables the cap.
type LimitedWriter struct {
	w   io.Writer
	max int64
	n   int64
	err error
}

// NewLimitedWriter returns a LimitedWriter bounding w to max bytes.
func NewLimitedWriter(w io.Writer, max int64) *LimitedWriter {
	return &LimitedWriter{w: w, max: max}
}

// Write implements io.Writer. If a write would push the running total past
// max, the bytes that fit are written, then a [LimitError] is returned.
func (l *LimitedWriter) Write(p []byte) (int, error) {
	if l.err != nil {
		return 0, l.err
	}
	if l.max <= 0 {
		return l.w.Write(p)
	}
	remaining := l.max - l.n
	if int64(len(p)) <= remaining {
		n, err := l.w.Write(p)
		l.n += int64(n)
		return n, err
	}
	// Write the portion that fits within budget, then trip.
	var written int
	if remaining > 0 {
		n, err := l.w.Write(p[:remaining])
		l.n += int64(n)
		written = n
		if err != nil {
			l.err = err
			return written, err
		}
	}
	l.err = newLimitError(ErrByteBudget, l.max, l.n+int64(len(p))-int64(written), "")
	return written, l.err
}
