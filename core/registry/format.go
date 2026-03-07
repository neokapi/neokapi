package registry

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gokapi/gokapi/core/config"
	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/format/schema"
)

// FormatReaderFactory creates a new DataFormatReader instance.
type FormatReaderFactory func() format.DataFormatReader

// FormatWriterFactory creates a new DataFormatWriter instance.
type FormatWriterFactory func() format.DataFormatWriter

// FormatInfo describes a registered format with its metadata.
type FormatInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	MimeTypes   []string `json:"mime_types,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
	HasReader   bool     `json:"has_reader"`
	HasWriter   bool     `json:"has_writer"`
	Source      string   `json:"source"`   // "built-in" or plugin name
	Priority    int      `json:"priority"` // higher = preferred when multiple formats match
}

// FormatRegistry manages available DataFormats and their configurations.
type FormatRegistry struct {
	mu       sync.RWMutex
	readers  map[string]FormatReaderFactory
	writers  map[string]FormatWriterFactory
	infos    map[string]*FormatInfo
	detector *format.FormatDetector

	// onMiss is called once when NewReader/NewWriter fails to find a format.
	// It allows lazy-loading of plugin formats (e.g., starting bridge processes)
	// only when a non-built-in format is actually needed.
	// Uses sync.Once to ensure concurrent callers block until loading completes.
	onMiss     func()
	onMissOnce sync.Once
}

// NewFormatRegistry creates a new FormatRegistry.
func NewFormatRegistry() *FormatRegistry {
	return &FormatRegistry{
		readers:  make(map[string]FormatReaderFactory),
		writers:  make(map[string]FormatWriterFactory),
		infos:    make(map[string]*FormatInfo),
		detector: format.NewFormatDetector(),
	}
}

// SetOnMiss registers a callback invoked once when NewReader or NewWriter
// cannot find a format among currently registered factories. This is used
// to lazily load bridge plugins only when a non-built-in format is needed.
func (r *FormatRegistry) SetOnMiss(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onMiss = fn
	r.onMissOnce = sync.Once{}
}

// triggerOnMiss calls the onMiss callback if set and not yet called.
// All concurrent callers block until the callback completes, ensuring
// lazy-loaded formats are available before any caller retries the lookup.
func (r *FormatRegistry) triggerOnMiss() bool {
	r.mu.RLock()
	fn := r.onMiss
	r.mu.RUnlock()

	if fn == nil {
		return false
	}
	r.onMissOnce.Do(fn)
	return true
}

// RegisterReader registers a DataFormatReader factory and its detection signature.
func (r *FormatRegistry) RegisterReader(name string, factory FormatReaderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readers[name] = factory

	// Register the format signature for detection and populate format info.
	reader := factory()
	sig := reader.Signature()
	r.detector.Register(name, sig)

	info := r.getOrCreateInfo(name)
	info.HasReader = true
	info.DisplayName = reader.DisplayName()
	if len(sig.MIMETypes) > 0 {
		info.MimeTypes = sig.MIMETypes
	}
	if len(sig.Extensions) > 0 {
		info.Extensions = sig.Extensions
	}
	// Ensure detector priority matches the info priority (which may have
	// been set before registration, e.g. via SetFormatPriority).
	if info.Priority != 0 {
		r.detector.SetPriority(name, info.Priority)
	} else {
		info.Priority = format.DefaultBuiltInPriority
	}
}

// RegisterWriter registers a DataFormatWriter factory.
func (r *FormatRegistry) RegisterWriter(name string, factory FormatWriterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writers[name] = factory

	info := r.getOrCreateInfo(name)
	info.HasWriter = true
}

// RegisterFormatInfo registers format metadata without a reader/writer factory.
// This is used during plugin metadata scanning so that "formats list" can show
// bridge-provided formats before the bridge process is started.
// When the bridge is later loaded, RegisterReader/RegisterWriter will update
// the existing info entry with HasReader/HasWriter = true.
func (r *FormatRegistry) RegisterFormatInfo(name string, info FormatInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing := r.getOrCreateInfo(name)
	if info.DisplayName != "" {
		existing.DisplayName = info.DisplayName
	}
	if len(info.MimeTypes) > 0 {
		existing.MimeTypes = info.MimeTypes
	}
	if len(info.Extensions) > 0 {
		existing.Extensions = info.Extensions
	}
	if info.Source != "" {
		existing.Source = info.Source
	}
	if info.Priority != 0 {
		existing.Priority = info.Priority
	}
	if info.HasReader {
		existing.HasReader = true
	}
	if info.HasWriter {
		existing.HasWriter = true
	}
}

// SetFormatSource sets the source (provider) for a format. Use "built-in" for
// built-in formats or the plugin name for plugin-provided formats.
// Plugin formats automatically receive DefaultPluginPriority unless a priority
// has already been explicitly set.
func (r *FormatRegistry) SetFormatSource(name, source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info := r.getOrCreateInfo(name)
	info.Source = source

	// Assign default plugin priority if the source is not built-in and no
	// explicit priority has been set (priority is still at default built-in).
	if source != "" && source != "built-in" && info.Priority == format.DefaultBuiltInPriority {
		info.Priority = format.DefaultPluginPriority
		r.detector.SetPriority(name, format.DefaultPluginPriority)
	}
}

// SetFormatPriority sets an explicit priority for the named format. Higher
// values are preferred when multiple formats match the same MIME type or
// extension during detection.
func (r *FormatRegistry) SetFormatPriority(name string, priority int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info := r.getOrCreateInfo(name)
	info.Priority = priority
	r.detector.SetPriority(name, priority)
}

// ResolveFormat finds the best format name for a given MIME type by consulting
// the detector (which considers priorities). Returns an empty string if no
// format matches.
func (r *FormatRegistry) ResolveFormat(mimeType string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, err := r.detector.DetectByMIME(mimeType)
	if err != nil {
		return ""
	}
	return name
}

// FormatInfos returns metadata for all registered formats, sorted by name.
func (r *FormatRegistry) FormatInfos() []FormatInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]FormatInfo, 0, len(r.infos))
	for _, info := range r.infos {
		cp := *info
		if cp.Source == "" {
			cp.Source = "built-in"
		}
		result = append(result, cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// FormatInfo returns metadata for a specific format, or nil if not found.
func (r *FormatRegistry) FormatInfo(name string) *FormatInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.infos[name]
	if !ok {
		return nil
	}
	cp := *info
	if cp.Source == "" {
		cp.Source = "built-in"
	}
	return &cp
}

// getOrCreateInfo returns the FormatInfo for the given name, creating it if needed.
// Caller must hold the write lock.
func (r *FormatRegistry) getOrCreateInfo(name string) *FormatInfo {
	info, ok := r.infos[name]
	if !ok {
		info = &FormatInfo{Name: name}
		r.infos[name] = info
	}
	return info
}

// DetectByExtension maps a file extension to a registered format name.
// If detection fails and an onMiss callback is set, it triggers lazy loading
// (e.g., starting bridge processes) and retries once.
func (r *FormatRegistry) DetectByExtension(ext string) (string, error) {
	if name, err := r.detector.DetectByExtension(ext); err == nil {
		return name, nil
	}
	if r.triggerOnMiss() {
		return r.detector.DetectByExtension(ext)
	}
	return "", fmt.Errorf("no format found for extension %q", ext)
}

// NewReader creates a new reader instance for the given format name.
// If the name contains "@", it looks up the exact versioned name.
// If the bare name is not found, it scans for versioned entries and
// returns the latest version as a fallback.
// If no reader is found and an onMiss callback is set, it triggers
// lazy loading (e.g., starting bridge processes) and retries once.
func (r *FormatRegistry) NewReader(name string) (format.DataFormatReader, error) {
	if f := r.findReader(name); f != nil {
		return f(), nil
	}
	// Trigger lazy loading and retry once.
	if r.triggerOnMiss() {
		if f := r.findReader(name); f != nil {
			return f(), nil
		}
	}
	return nil, fmt.Errorf("unknown format: %s", name)
}

// NewWriter creates a new writer instance for the given format name.
// If the name contains "@", it looks up the exact versioned name.
// If the bare name is not found, it scans for versioned entries and
// returns the latest version as a fallback.
// If no writer is found and an onMiss callback is set, it triggers
// lazy loading (e.g., starting bridge processes) and retries once.
func (r *FormatRegistry) NewWriter(name string) (format.DataFormatWriter, error) {
	if f := r.findWriter(name); f != nil {
		return f(), nil
	}
	// Trigger lazy loading and retry once.
	if r.triggerOnMiss() {
		if f := r.findWriter(name); f != nil {
			return f(), nil
		}
	}
	return nil, fmt.Errorf("unknown format writer: %s", name)
}

// findReader looks up a reader factory by exact name or latest versioned entry.
func (r *FormatRegistry) findReader(name string) FormatReaderFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if factory, ok := r.readers[name]; ok {
		return factory
	}
	if !strings.Contains(name, "@") {
		return r.findLatestReader(name)
	}
	return nil
}

// findWriter looks up a writer factory by exact name or latest versioned entry.
func (r *FormatRegistry) findWriter(name string) FormatWriterFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if factory, ok := r.writers[name]; ok {
		return factory
	}
	if !strings.Contains(name, "@") {
		return r.findLatestWriter(name)
	}
	return nil
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
	for i := range 3 {
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
	for i := range 3 {
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

// CollectNativeSchemas iterates over registered format readers and collects
// schemas from any whose Config implements format.SchemaProvider.
func (r *FormatRegistry) CollectNativeSchemas(schemaReg *schema.SchemaRegistry) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, factory := range r.readers {
		// Skip versioned entries — only collect from bare names.
		if strings.Contains(name, "@") {
			continue
		}
		reader := factory()
		cfg := reader.Config()
		if cfg == nil {
			continue
		}
		if sp, ok := cfg.(format.SchemaProvider); ok {
			schemaReg.RegisterSchema(name, sp.Schema())
		}
	}
}

// CollectNativeDecoders registers SpecDecoders for all native format configs
// into the given config.Registry. For formats implementing ConfigKindProvider,
// their declared kind is used. For others, FormatConfigKind(name) is used.
func (r *FormatRegistry) CollectNativeDecoders(configReg *config.Registry) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, factory := range r.readers {
		if strings.Contains(name, "@") {
			continue
		}
		reader := factory()
		cfg := reader.Config()
		if cfg == nil {
			continue
		}

		kind := config.FormatConfigKind(name)
		if ckp, ok := cfg.(format.ConfigKindProvider); ok {
			kind = ckp.ConfigKind()
		}

		// Capture name for closure
		formatName := name
		configReg.Register(kind, config.SpecDecoderFunc(func(spec map[string]any) (any, error) {
			r.mu.RLock()
			f, ok := r.readers[formatName]
			r.mu.RUnlock()
			if !ok {
				return nil, fmt.Errorf("format %q not found", formatName)
			}
			rdr := f()
			c := rdr.Config()
			if c == nil {
				return spec, nil
			}
			c.Reset()
			if err := c.ApplyMap(spec); err != nil {
				return nil, err
			}
			return c, nil
		}))
	}
}

// Ensure FormatRegistry implements SubfilterResolver.
var _ format.SubfilterResolver = (*FormatRegistry)(nil)

// ResolveReader creates a new reader for the named format. Implements SubfilterResolver.
func (r *FormatRegistry) ResolveReader(name string) (format.DataFormatReader, error) {
	return r.NewReader(name)
}

// ResolveWriter creates a new writer for the named format. Implements SubfilterResolver.
func (r *FormatRegistry) ResolveWriter(name string) (format.DataFormatWriter, error) {
	return r.NewWriter(name)
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

// ReaderFactory returns the reader factory for the given format name, or nil.
// Use this to build alias factories without triggering lock re-entry.
func (r *FormatRegistry) ReaderFactory(name string) FormatReaderFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.readers[name]
}

// WriterFactory returns the writer factory for the given format name, or nil.
// Use this to build alias factories without triggering lock re-entry.
func (r *FormatRegistry) WriterFactory(name string) FormatWriterFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.writers[name]
}
