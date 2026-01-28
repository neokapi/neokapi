package format

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// FormatDetector determines the data format of a document using multiple strategies.
type FormatDetector struct {
	signatures map[string]FormatSignature // format name → signature
}

// NewFormatDetector creates a new FormatDetector.
func NewFormatDetector() *FormatDetector {
	return &FormatDetector{
		signatures: make(map[string]FormatSignature),
	}
}

// Register adds a format signature for detection.
func (d *FormatDetector) Register(name string, sig FormatSignature) {
	d.signatures[name] = sig
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

// DetectByMIME maps a MIME type to a registered format name.
func (d *FormatDetector) DetectByMIME(mimeType string) (string, error) {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	for name, sig := range d.signatures {
		for _, m := range sig.MIMETypes {
			if strings.ToLower(m) == mimeType {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("no format found for MIME type %q", mimeType)
}

// DetectByExtension maps a file extension to a registered format name.
func (d *FormatDetector) DetectByExtension(ext string) (string, error) {
	ext = strings.ToLower(ext)
	if ext == "" {
		return "", fmt.Errorf("empty extension")
	}
	for name, sig := range d.signatures {
		for _, e := range sig.Extensions {
			if strings.ToLower(e) == ext {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("no format found for extension %q", ext)
}

// DetectByContent reads the first bytes and matches against registered signatures.
func (d *FormatDetector) DetectByContent(reader io.ReadSeeker) (string, error) {
	buf := make([]byte, 512)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("reading content: %w", err)
	}
	buf = buf[:n]

	// Reset reader position
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seeking: %w", err)
	}

	// Try magic bytes first
	for name, sig := range d.signatures {
		for _, magic := range sig.MagicBytes {
			if bytes.HasPrefix(buf, magic) {
				return name, nil
			}
		}
	}

	// Try custom sniff functions
	for name, sig := range d.signatures {
		if sig.Sniff != nil && sig.Sniff(buf) {
			return name, nil
		}
	}

	return "", fmt.Errorf("unable to detect format from content")
}
