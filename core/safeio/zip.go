package safeio

import (
	"archive/zip"
	"io"
	"math"
)

// ZipLimits bounds a zip-container parse against the four classic archive
// attack classes: an oversized single entry, a decompression (zip) bomb, an
// oversized total payload, and an entry-count flood. The semantics mirror
// Apache POI's ZipSecureFile, which is the canonical Java implementation:
//
//   - MaxEntrySize    — POI setMaxEntrySize (per-entry uncompressed cap).
//   - MinInflateRatio — POI setMinInflateRatio (compressed/uncompressed; below
//     this an entry is treated as a bomb). POI's default is 0.01 (100:1).
//   - MaxTotalSize    — cumulative uncompressed cap across the archive.
//   - MaxEntries      — entry-count cap.
//
// The zero value is not useful; use [DefaultZipLimits], compose from a
// [Budget], or fill the fields and let withDefaults backfill zero fields.
type ZipLimits struct {
	// MaxEntrySize caps a single entry's uncompressed bytes.
	MaxEntrySize int64
	// MaxTotalSize caps the cumulative uncompressed bytes of all entries.
	MaxTotalSize int64
	// MinInflateRatio is the minimum allowed compressed/uncompressed ratio.
	// An entry that inflates more aggressively (ratio below this, past the
	// grace size) is rejected as a zip bomb.
	MinInflateRatio float64
	// MaxEntries caps the number of entries in the archive.
	MaxEntries int
}

// DefaultZipLimits is the canonical set of zip limits, applied identically
// across CLI, server, and WASM (see the package doc).
var DefaultZipLimits = ZipLimits{
	MaxEntrySize:    DefaultMaxEntrySize,
	MaxTotalSize:    DefaultMaxTotalSize,
	MinInflateRatio: DefaultMinInflateRatio,
	MaxEntries:      DefaultMaxEntries,
}

// withDefaults returns a copy with any zero-valued field backfilled from the
// package defaults, so a partially-specified ZipLimits is still safe.
func (z ZipLimits) withDefaults() ZipLimits {
	if z.MaxEntrySize == 0 {
		z.MaxEntrySize = DefaultMaxEntrySize
	}
	if z.MaxTotalSize == 0 {
		z.MaxTotalSize = DefaultMaxTotalSize
	}
	if z.MinInflateRatio == 0 {
		z.MinInflateRatio = DefaultMinInflateRatio
	}
	if z.MaxEntries == 0 {
		z.MaxEntries = DefaultMaxEntries
	}
	return z
}

// clampToInt64 converts a uint64 to int64, saturating at math.MaxInt64 so a
// hostile Zip64 header that declares a size larger than int64 doesn't wrap to a
// negative diagnostic value.
func clampToInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// CheckEntry validates a single zip entry's declared header sizes before any
// bytes are read: it rejects an entry whose declared uncompressed size exceeds
// MaxEntrySize, and an entry whose declared compression ratio is a bomb. This
// is the cheap up-front gate; [ZipLimits.OpenEntry] additionally enforces the
// caps on the *actual* stream, because a malicious header can lie.
func (z ZipLimits) CheckEntry(f *zip.File) error {
	z = z.withDefaults()
	if z.MaxEntrySize > 0 && f.UncompressedSize64 > uint64(z.MaxEntrySize) {
		return newLimitError(ErrEntryTooLarge, z.MaxEntrySize, clampToInt64(f.UncompressedSize64), f.Name)
	}
	if f.UncompressedSize64 > uint64(graceEntrySize) && f.CompressedSize64 > 0 {
		ratio := float64(f.CompressedSize64) / float64(f.UncompressedSize64)
		if ratio < z.MinInflateRatio {
			return newLimitError(ErrInflateRatio, 0, 0, f.Name)
		}
	}
	return nil
}

// CheckReader validates a whole archive up front using declared header sizes:
// the entry count against MaxEntries, every entry against [ZipLimits.CheckEntry],
// and the summed declared uncompressed size against MaxTotalSize. It catches
// honestly-declared bombs (including Zip64 headers claiming petabyte sizes)
// before a single byte is decompressed. Per-entry streaming enforcement
// (lying headers) is handled by [ZipLimits.OpenEntry] when each entry is read.
func (z ZipLimits) CheckReader(zr *zip.Reader) error {
	z = z.withDefaults()
	if z.MaxEntries > 0 && len(zr.File) > z.MaxEntries {
		return newLimitError(ErrTooManyEntries, int64(z.MaxEntries), int64(len(zr.File)), "")
	}
	var total uint64
	for _, f := range zr.File {
		if err := z.CheckEntry(f); err != nil {
			return err
		}
		total += f.UncompressedSize64
		if z.MaxTotalSize > 0 && total > uint64(z.MaxTotalSize) {
			return newLimitError(ErrTotalTooLarge, z.MaxTotalSize, clampToInt64(total), "")
		}
	}
	return nil
}

// OpenEntry opens a zip entry for reading, returning a reader that enforces the
// per-entry uncompressed-size cap and the inflate-ratio (zip-bomb) guard on the
// actual decompressed stream. Use this instead of f.Open() so a lying header
// cannot evade the caps. The returned ReadCloser must be closed by the caller.
func (z ZipLimits) OpenEntry(f *zip.File) (io.ReadCloser, error) {
	return z.NewGuard().Open(f)
}

// ReadEntry opens a zip entry with [ZipLimits.OpenEntry] and reads it fully,
// returning a typed [LimitError] if any cap is breached. It is the drop-in
// replacement for `f.Open()` followed by `io.ReadAll`.
func (z ZipLimits) ReadEntry(f *zip.File) ([]byte, error) {
	return z.NewGuard().ReadEntry(f)
}

// ZipGuard is a stateful zip reader bound to a ZipLimits. Reading every entry
// through a single guard makes the cumulative MaxTotalSize and MaxEntries caps
// apply across the whole archive (not just per entry). Construct one with
// [ZipLimits.NewGuard] or [Budget.ZipGuard]. ZipGuard is not safe for
// concurrent use.
type ZipGuard struct {
	limits  ZipLimits
	total   int64 // cumulative uncompressed bytes streamed so far
	entries int
}

// NewGuard returns a stateful ZipGuard bound to z (defaults backfilled).
func (z ZipLimits) NewGuard() *ZipGuard {
	return &ZipGuard{limits: z.withDefaults()}
}

// Open opens an entry through the guard, enforcing the entry-count cap and the
// declared-header checks up front, and returning a streaming reader that
// enforces the per-entry size, inflate-ratio, and cumulative total-size caps.
func (g *ZipGuard) Open(f *zip.File) (io.ReadCloser, error) {
	g.entries++
	if g.limits.MaxEntries > 0 && g.entries > g.limits.MaxEntries {
		return nil, newLimitError(ErrTooManyEntries, int64(g.limits.MaxEntries), int64(g.entries), f.Name)
	}
	if err := g.limits.CheckEntry(f); err != nil {
		return nil, err
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	return &entryReader{
		rc:         rc,
		limits:     g.limits,
		compressed: clampToInt64(f.CompressedSize64),
		name:       f.Name,
		guard:      g,
	}, nil
}

// ReadEntry opens an entry through the guard and reads it fully.
func (g *ZipGuard) ReadEntry(f *zip.File) ([]byte, error) {
	rc, err := g.Open(f)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// entryReader wraps a zip entry's decompressing reader and enforces the size,
// ratio, and cumulative caps as bytes are produced.
type entryReader struct {
	rc         io.ReadCloser
	limits     ZipLimits
	compressed int64 // declared compressed size of this entry
	name       string
	n          int64 // uncompressed bytes read from this entry
	guard      *ZipGuard
}

// Read implements io.Reader. On any breach it returns a typed [LimitError] and
// stops, rather than handing back the over-cap bytes.
func (e *entryReader) Read(p []byte) (int, error) {
	n, err := e.rc.Read(p)
	if n > 0 {
		e.n += int64(n)
		if e.limits.MaxEntrySize > 0 && e.n > e.limits.MaxEntrySize {
			return 0, newLimitError(ErrEntryTooLarge, e.limits.MaxEntrySize, e.n, e.name)
		}
		if e.guard != nil {
			e.guard.total += int64(n)
			if e.limits.MaxTotalSize > 0 && e.guard.total > e.limits.MaxTotalSize {
				return 0, newLimitError(ErrTotalTooLarge, e.limits.MaxTotalSize, e.guard.total, e.name)
			}
		}
		// Inflate-ratio (zip-bomb) check against the actual stream. Only past
		// the grace size, so small highly-compressible entries don't trip it.
		if e.n > graceEntrySize && e.compressed > 0 {
			if float64(e.compressed)/float64(e.n) < e.limits.MinInflateRatio {
				return 0, newLimitError(ErrInflateRatio, 0, 0, e.name)
			}
		}
	}
	return n, err
}

// Close closes the underlying entry reader.
func (e *entryReader) Close() error { return e.rc.Close() }
