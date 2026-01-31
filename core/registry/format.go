package registry

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/gokapi/gokapi/core/format"
)

// FormatReaderFactory creates a new DataFormatReader instance.
type FormatReaderFactory func() format.DataFormatReader

// FormatWriterFactory creates a new DataFormatWriter instance.
type FormatWriterFactory func() format.DataFormatWriter

// FormatRegistry manages available DataFormats and their configurations.
type FormatRegistry struct {
	mu       sync.RWMutex
	readers  map[string]FormatReaderFactory
	writers  map[string]FormatWriterFactory
	detector *format.FormatDetector
}

// NewFormatRegistry creates a new FormatRegistry.
func NewFormatRegistry() *FormatRegistry {
	return &FormatRegistry{
		readers:  make(map[string]FormatReaderFactory),
		writers:  make(map[string]FormatWriterFactory),
		detector: format.NewFormatDetector(),
	}
}

// RegisterReader registers a DataFormatReader factory and its detection signature.
func (r *FormatRegistry) RegisterReader(name string, factory FormatReaderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readers[name] = factory

	// Register the format signature for detection
	reader := factory()
	r.detector.Register(name, reader.Signature())
}

// RegisterWriter registers a DataFormatWriter factory.
func (r *FormatRegistry) RegisterWriter(name string, factory FormatWriterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writers[name] = factory
}

// NewReader creates a new reader instance for the given format name.
// If the name contains "@", it looks up the exact versioned name.
// If the bare name is not found, it scans for versioned entries and
// returns the latest version as a fallback.
func (r *FormatRegistry) NewReader(name string) (format.DataFormatReader, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.readers[name]
	if ok {
		return factory(), nil
	}
	// Fallback: if name has no "@", scan for versioned entries.
	if !strings.Contains(name, "@") {
		if f := r.findLatestReader(name); f != nil {
			return f(), nil
		}
	}
	return nil, fmt.Errorf("unknown format: %s", name)
}

// NewWriter creates a new writer instance for the given format name.
// If the name contains "@", it looks up the exact versioned name.
// If the bare name is not found, it scans for versioned entries and
// returns the latest version as a fallback.
func (r *FormatRegistry) NewWriter(name string) (format.DataFormatWriter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.writers[name]
	if ok {
		return factory(), nil
	}
	// Fallback: if name has no "@", scan for versioned entries.
	if !strings.Contains(name, "@") {
		if f := r.findLatestWriter(name); f != nil {
			return f(), nil
		}
	}
	return nil, fmt.Errorf("unknown format writer: %s", name)
}

// findLatestReader scans the readers map for entries matching "name@version"
// and returns the factory for the highest semantic version.
func (r *FormatRegistry) findLatestReader(name string) FormatReaderFactory {
	prefix := name + "@"
	var bestVersion string
	var bestFactory FormatReaderFactory
	for key, factory := range r.readers {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		version := key[len(prefix):]
		if bestFactory == nil || compareSemver(version, bestVersion) > 0 {
			bestVersion = version
			bestFactory = factory
		}
	}
	return bestFactory
}

// findLatestWriter scans the writers map for entries matching "name@version"
// and returns the factory for the highest semantic version.
func (r *FormatRegistry) findLatestWriter(name string) FormatWriterFactory {
	prefix := name + "@"
	var bestVersion string
	var bestFactory FormatWriterFactory
	for key, factory := range r.writers {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		version := key[len(prefix):]
		if bestFactory == nil || compareSemver(version, bestVersion) > 0 {
			bestVersion = version
			bestFactory = factory
		}
	}
	return bestFactory
}

// compareSemver compares two semantic version strings (major.minor.patch).
func compareSemver(a, b string) int {
	ap := parseSemverParts(a)
	bp := parseSemverParts(b)
	for i := 0; i < 3; i++ {
		if ap[i] < bp[i] {
			return -1
		}
		if ap[i] > bp[i] {
			return 1
		}
	}
	return 0
}

func parseSemverParts(s string) [3]int {
	var parts [3]int
	fields := strings.SplitN(s, ".", 3)
	for i := 0; i < 3; i++ {
		if i < len(fields) {
			n, err := strconv.Atoi(fields[i])
			if err != nil {
				parts[i] = -1
			} else {
				parts[i] = n
			}
		} else {
			parts[i] = -1
		}
	}
	return parts
}

// Detector returns the FormatDetector backed by this registry.
func (r *FormatRegistry) Detector() *format.FormatDetector {
	return r.detector
}

// ReaderNames returns the names of all registered readers.
func (r *FormatRegistry) ReaderNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.readers))
	for name := range r.readers {
		names = append(names, name)
	}
	return names
}

// WriterNames returns the names of all registered writers.
func (r *FormatRegistry) WriterNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.writers))
	for name := range r.writers {
		names = append(names, name)
	}
	return names
}

// HasReader returns true if a reader is registered for the given format name.
func (r *FormatRegistry) HasReader(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.readers[name]
	return ok
}

// HasWriter returns true if a writer is registered for the given format name.
func (r *FormatRegistry) HasWriter(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.writers[name]
	return ok
}
