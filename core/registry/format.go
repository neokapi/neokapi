package registry

import (
	"fmt"
	"sync"

	"github.com/asgeirf/gokapi/core/format"
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
func (r *FormatRegistry) NewReader(name string) (format.DataFormatReader, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.readers[name]
	if !ok {
		return nil, fmt.Errorf("unknown format: %s", name)
	}
	return factory(), nil
}

// NewWriter creates a new writer instance for the given format name.
func (r *FormatRegistry) NewWriter(name string) (format.DataFormatWriter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.writers[name]
	if !ok {
		return nil, fmt.Errorf("unknown format writer: %s", name)
	}
	return factory(), nil
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
