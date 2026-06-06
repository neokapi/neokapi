package format

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gabriel-vasile/mimetype"
)

// DefaultBuiltInPriority is the default priority for built-in formats.
const DefaultBuiltInPriority = 50

// DefaultPluginPriority is the default priority for plugin-provided formats.
const DefaultPluginPriority = 100

// Detector determines the data format of a document using multiple strategies.
type Detector struct {
	mu         sync.RWMutex
	signatures map[string]FormatSignature // format name → signature
	priorities map[string]int             // format name → priority (higher = preferred)
}

// FormatDetector is a deprecated alias for [Detector].
//
// Deprecated: Use [Detector] instead.
type FormatDetector = Detector

// NewDetector creates a new Detector.
func NewDetector() *Detector {
	return &Detector{
		signatures: make(map[string]FormatSignature),
		priorities: make(map[string]int),
	}
}

// NewFormatDetector is a deprecated alias for [NewDetector].
//
// Deprecated: Use [NewDetector] instead.
func NewFormatDetector() *Detector {
	return NewDetector()
}

// Register adds a format signature for detection.
func (d *Detector) Register(name string, sig FormatSignature) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.signatures[name] = sig
	if _, ok := d.priorities[name]; !ok {
		d.priorities[name] = DefaultBuiltInPriority
	}
}

// SetPriority sets the detection priority for a named format. Higher values
// are preferred when multiple formats match the same MIME type or extension.
func (d *Detector) SetPriority(name string, priority int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.priorities[name] = priority
}

// Priority returns the priority for a named format, or 0 if not set.
func (d *Detector) Priority(name string) int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.priorities[name]
}

// Detect tries all strategies: explicit MIME → extension → content sniffing.
func (d *Detector) Detect(path string, reader io.ReadSeeker, mimeType string) (string, error) {
	// Try MIME type first
	if mimeType != "" {
		if name, err := d.DetectByMIME(mimeType); err == nil {
			return name, nil
		}
	}

	// Try extension. When an extension is claimed by more than one format
	// (e.g. ".xliff" → XLIFF 1.x and 2.x, or ".xml" → many), the bare
	// extension can't tell them apart — let content sniffing pick among the
	// candidates so a 2.x file isn't silently read by the 1.x reader. A
	// single-claimant extension still resolves without reading content.
	if path != "" {
		if ext := filepath.Ext(path); ext != "" {
			cands := d.extensionCandidates(ext)
			if len(cands) > 1 && reader != nil {
				if name, err := d.DetectByContent(reader); err == nil && cands[name] {
					return name, nil
				}
			}
			if name, err := d.DetectByExtension(ext); err == nil {
				return name, nil
			}
		}
	}

	// Try content sniffing
	if reader != nil {
		if name, err := d.DetectByContent(reader); err == nil {
			return name, nil
		}
	}

	return "", fmt.Errorf("unable to detect format for %q", path)
}

// DetectByMIME maps a MIME type to a registered format name. When multiple
// formats match, the one with the highest priority is returned.
// When priorities are equal, the lexicographically first name wins.
func (d *Detector) DetectByMIME(mimeType string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	bestName := ""
	bestPriority := -1
	for name, sig := range d.signatures {
		for _, m := range sig.MIMETypes {
			if strings.ToLower(m) == mimeType {
				pri := d.priorities[name]
				if bestName == "" || pri > bestPriority || (pri == bestPriority && name < bestName) {
					bestName = name
					bestPriority = pri
				}
			}
		}
	}
	if bestName != "" {
		return bestName, nil
	}
	return "", fmt.Errorf("no format found for MIME type %q", mimeType)
}

// DetectByExtension maps a file extension to a registered format name. When
// multiple formats match, the one with the highest priority is returned.
// When priorities are equal, the lexicographically first name wins for
// deterministic results (e.g. okf_xliff before okf_xliff2 for ".xlf").
func (d *Detector) DetectByExtension(ext string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ext = strings.ToLower(ext)
	if ext == "" {
		return "", errors.New("empty extension")
	}
	bestName := ""
	bestPriority := -1
	for name, sig := range d.signatures {
		for _, e := range sig.Extensions {
			if strings.ToLower(e) == ext {
				pri := d.priorities[name]
				if bestName == "" || pri > bestPriority || (pri == bestPriority && name < bestName) {
					bestName = name
					bestPriority = pri
				}
			}
		}
	}
	if bestName != "" {
		return bestName, nil
	}
	return "", fmt.Errorf("no format found for extension %q", ext)
}

// extensionCandidates returns the set of registered format names that claim the
// given extension. Used by Detect to decide whether content sniffing is needed
// to disambiguate an extension shared by several formats.
func (d *Detector) extensionCandidates(ext string) map[string]bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ext = strings.ToLower(ext)
	out := make(map[string]bool)
	if ext == "" {
		return out
	}
	for name, sig := range d.signatures {
		for _, e := range sig.Extensions {
			if strings.ToLower(e) == ext {
				out[name] = true
			}
		}
	}
	return out
}

// DetectByContent identifies the format of a document from its content using a
// layered strategy, in order:
//
//  1. Custom Sniff functions — the precise, format-specific detectors (an EPUB
//     mimetype member, an Android <resources> root, …). These run FIRST so a
//     coarse magic prefix shared by several formats can't shadow a format that
//     positively identifies itself.
//  2. Magic-byte prefixes, but only when UNAMBIGUOUS: if exactly one registered
//     format claims the matched prefix it is trusted; when several share it —
//     notably the ZIP prefix used by every OOXML / ODF / IDML / EPUB format —
//     detection defers to step 3 rather than guessing by name order.
//  3. Container/binary detection via gabriel-vasile/mimetype, mapped back to a
//     registered format through its MIME type (walking the MIME hierarchy so a
//     format that only registered a parent type still resolves). This is what
//     distinguishes a .docx from an .epub when both are just "a ZIP".
//  4. As a last resort, the best (ambiguous) magic match from step 2.
func (d *Detector) DetectByContent(reader io.ReadSeeker) (string, error) {
	buf := make([]byte, 512)
	n, err := reader.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading content: %w", err)
	}
	buf = buf[:n]
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seeking: %w", err)
	}

	sniffName, magicName, magicAmbiguous := d.matchBuffer(buf)

	// 1. Precise sniffers win outright.
	if sniffName != "" {
		return sniffName, nil
	}

	// 2. An unambiguous magic prefix is trusted directly.
	if magicName != "" && !magicAmbiguous {
		return magicName, nil
	}

	// 3. Container/binary detection. mimetype reads what it needs from the
	//    reader; reset afterwards so callers can re-read from the start.
	if mt, mErr := mimetype.DetectReader(reader); mErr == nil {
		_, _ = reader.Seek(0, io.SeekStart)
		for m := mt; m != nil; m = m.Parent() {
			mime := strings.TrimSpace(strings.SplitN(m.String(), ";", 2)[0])
			// Skip mimetype's "don't know" fallbacks: text/plain and
			// octet-stream are not positive identifications — mapping them to a
			// registered format would turn every texty/binary blob into that
			// format. Callers that want a plaintext default supply their own.
			if mime == "" || mime == "text/plain" || mime == "application/octet-stream" {
				continue
			}
			if name, err := d.DetectByMIME(mime); err == nil {
				return name, nil
			}
		}
	} else {
		_, _ = reader.Seek(0, io.SeekStart)
	}

	// 4. Fall back to the best (ambiguous) magic match.
	if magicName != "" {
		return magicName, nil
	}
	return "", errors.New("unable to detect format from content")
}

// matchBuffer evaluates the registered Sniff functions and magic-byte prefixes
// against buf. It returns the best Sniff match, the best magic-prefix match, and
// whether that magic prefix is shared by more than one format (ambiguous).
// Within each category, higher priority wins and ties break lexicographically.
func (d *Detector) matchBuffer(buf []byte) (sniffName, magicName string, magicAmbiguous bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sniffPri := -1
	for name, sig := range d.signatures {
		if sig.Sniff != nil && sig.Sniff(buf) {
			pri := d.priorities[name]
			if sniffName == "" || pri > sniffPri || (pri == sniffPri && name < sniffName) {
				sniffName, sniffPri = name, pri
			}
		}
	}

	magicPri := -1
	matches := 0
	for name, sig := range d.signatures {
		hit := false
		for _, magic := range sig.MagicBytes {
			if bytes.HasPrefix(buf, magic) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		matches++
		pri := d.priorities[name]
		if magicName == "" || pri > magicPri || (pri == magicPri && name < magicName) {
			magicName, magicPri = name, pri
		}
	}
	return sniffName, magicName, matches > 1
}
