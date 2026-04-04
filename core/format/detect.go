package format

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultBuiltInPriority is the default priority for built-in formats.
const DefaultBuiltInPriority = 50

// DefaultPluginPriority is the default priority for plugin-provided formats.
const DefaultPluginPriority = 100

// FormatDetector determines the data format of a document using multiple strategies.
type FormatDetector struct {
	mu         sync.RWMutex
	signatures map[string]FormatSignature // format name → signature
	priorities map[string]int             // format name → priority (higher = preferred)
}

// NewFormatDetector creates a new FormatDetector.
func NewFormatDetector() *FormatDetector {
	return &FormatDetector{
		signatures: make(map[string]FormatSignature),
		priorities: make(map[string]int),
	}
}

// Register adds a format signature for detection.
func (d *FormatDetector) Register(name string, sig FormatSignature) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.signatures[name] = sig
	if _, ok := d.priorities[name]; !ok {
		d.priorities[name] = DefaultBuiltInPriority
	}
}

// SetPriority sets the detection priority for a named format. Higher values
// are preferred when multiple formats match the same MIME type or extension.
func (d *FormatDetector) SetPriority(name string, priority int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.priorities[name] = priority
}

// Priority returns the priority for a named format, or 0 if not set.
func (d *FormatDetector) Priority(name string) int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.priorities[name]
}

// Detect tries all strategies: explicit MIME → extension → content sniffing.
func (d *FormatDetector) Detect(path string, reader io.ReadSeeker, mimeType string) (string, error) {
	// Try MIME type first
	if mimeType != "" {
		if name, err := d.DetectByMIME(mimeType); err == nil {
			return name, nil
		}
	}

	// Try extension
	if path != "" {
		if name, err := d.DetectByExtension(filepath.Ext(path)); err == nil {
			return name, nil
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
func (d *FormatDetector) DetectByMIME(mimeType string) (string, error) {
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
func (d *FormatDetector) DetectByExtension(ext string) (string, error) {
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

// DetectByContent reads the first bytes and matches against registered signatures.
// When multiple formats match, the one with the highest priority is returned.
func (d *FormatDetector) DetectByContent(reader io.ReadSeeker) (string, error) {
	buf := make([]byte, 512)
	n, err := reader.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading content: %w", err)
	}
	buf = buf[:n]

	// Reset reader position
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seeking: %w", err)
	}

	bestName := ""
	bestPriority := -1

	// Try magic bytes first
	for name, sig := range d.signatures {
		for _, magic := range sig.MagicBytes {
			if bytes.HasPrefix(buf, magic) {
				pri := d.priorities[name]
				if bestName == "" || pri > bestPriority || (pri == bestPriority && name < bestName) {
					bestName = name
					bestPriority = pri
				}
				break // no need to check other magic bytes for same format
			}
		}
	}

	if bestName != "" {
		return bestName, nil
	}

	// Try custom sniff functions
	for name, sig := range d.signatures {
		if sig.Sniff != nil && sig.Sniff(buf) {
			pri := d.priorities[name]
			if bestName == "" || pri > bestPriority || (pri == bestPriority && name < bestName) {
				bestName = name
				bestPriority = pri
			}
		}
	}

	if bestName != "" {
		return bestName, nil
	}

	return "", errors.New("unable to detect format from content")
}
