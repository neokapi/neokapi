package registry

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/schema"
)

// FormatReaderFactory creates a new DataFormatReader instance.
type FormatReaderFactory func() format.DataFormatReader

// FormatWriterFactory creates a new DataFormatWriter instance.
type FormatWriterFactory func() format.DataFormatWriter

// FormatInfo describes a registered format with its metadata.
type FormatInfo struct {
	Name        FormatID `json:"name"`
	DisplayName string   `json:"display_name"`
	MimeTypes   []string `json:"mime_types,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
	HasReader   bool     `json:"has_reader"`
	HasWriter   bool     `json:"has_writer"`
	// Generative reports that the writer can serialize a complete document from
	// the content model alone (no source skeleton) — so the format is a valid
	// cross-format conversion target. Declarative: for built-ins it is probed
	// once from the writer's GenerativeWriter capability at registration; for
	// plugin formats it comes from the cached manifest ("generative" capability),
	// never by loading the plugin. Meaningful only when HasWriter is true.
	Generative bool `json:"generative,omitempty"`
	// Interchange reports a bilingual translation-interchange format (XLIFF, PO,
	// TMX, …). These belong to the extract→translate→merge loop and are excluded
	// as `convert` targets. Declarative, like Generative.
	Interchange bool   `json:"interchange,omitempty"`
	Source      string `json:"source"`   // SourceBuiltIn or plugin name
	Priority    int    `json:"priority"` // higher = preferred when multiple formats match
}

// FormatRegistry manages available DataFormats and their configurations.
type FormatRegistry struct {
	mu       sync.RWMutex
	readers  map[FormatID]FormatReaderFactory
	writers  map[FormatID]FormatWriterFactory
	infos    map[FormatID]*FormatInfo
	aliases  map[FormatID]FormatID // alias id → canonical id
	detector *format.Detector

	// onMiss is called at most once per registered callback when
	// NewReader/NewWriter fails to find a format. It allows lazy-loading of
	// plugin formats (e.g., starting bridge processes) only when a non-built-in
	// format is actually needed.
	//
	// onMissMu serializes triggerOnMiss so that concurrent callers block until
	// the in-flight callback completes (the callback typically registers
	// formats, so it must run without holding mu to avoid self-deadlock). The
	// onMiss func itself is read under mu. onMissLoaded — guarded by onMissMu —
	// records whether the current callback has already fired; SetOnMiss resets
	// it under both locks so a freshly registered callback may fire once.
	onMiss       func()
	onMissMu     sync.Mutex
	onMissLoaded bool
}

// NewFormatRegistry creates a new FormatRegistry.
func NewFormatRegistry() *FormatRegistry {
	return &FormatRegistry{
		readers:  make(map[FormatID]FormatReaderFactory),
		writers:  make(map[FormatID]FormatWriterFactory),
		infos:    make(map[FormatID]*FormatInfo),
		aliases:  make(map[FormatID]FormatID),
		detector: format.NewDetector(),
	}
}

// SetOnMiss registers a callback invoked once when NewReader or NewWriter
// cannot find a format among currently registered factories. This is used
// to lazily load bridge plugins only when a non-built-in format is needed.
func (r *FormatRegistry) SetOnMiss(fn func()) {
	// Take onMissMu before mu to match triggerOnMiss's lock order and avoid a
	// deadlock with an in-flight callback.
	r.onMissMu.Lock()
	defer r.onMissMu.Unlock()
	r.mu.Lock()
	r.onMiss = fn
	r.mu.Unlock()
	r.onMissLoaded = false
}

// triggerOnMiss calls the onMiss callback if one is set and has not yet fired
// for the current registration. It returns true when a callback is registered
// (whether or not it fired this call), so callers know a lazy-load path exists
// and a retry is worthwhile.
//
// onMissMu serializes the whole operation, so concurrent callers block until
// any in-flight callback completes — guaranteeing lazy-loaded formats are
// available before a caller retries the lookup, and that the callback fires at
// most once per registered callback. The callback itself runs without holding
// mu, because it typically calls back into RegisterReader/RegisterWriter (which
// take mu) to register the newly loaded formats.
func (r *FormatRegistry) triggerOnMiss() bool {
	r.onMissMu.Lock()
	defer r.onMissMu.Unlock()

	r.mu.RLock()
	fn := r.onMiss
	r.mu.RUnlock()

	if fn == nil {
		return false
	}
	if !r.onMissLoaded {
		r.onMissLoaded = true
		fn()
	}
	return true
}

// RegisterReader registers a DataFormatReader factory with static metadata.
// The signature and display name are provided directly — no reader instance
// is created during registration, keeping startup fast.
func (r *FormatRegistry) RegisterReader(name FormatID, factory FormatReaderFactory, sig format.FormatSignature, displayName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readers[name] = factory

	r.detector.Register(string(name), sig)

	info := r.getOrCreateInfo(name)
	info.HasReader = true
	info.DisplayName = displayName
	if len(sig.MIMETypes) > 0 {
		info.MimeTypes = sig.MIMETypes
	}
	if len(sig.Extensions) > 0 {
		info.Extensions = sig.Extensions
	}
	if info.Priority != 0 {
		r.detector.SetPriority(string(name), info.Priority)
	} else {
		info.Priority = format.DefaultBuiltInPriority
	}
}

// RegisterWriter registers a DataFormatWriter factory.
func (r *FormatRegistry) RegisterWriter(name FormatID, factory FormatWriterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writers[name] = factory

	info := r.getOrCreateInfo(name)
	info.HasWriter = true

	// Probe the writer's declared Generative capability — but ONLY for built-in
	// formats. Constructing a built-in writer is cheap and side-effect-free;
	// for plugin formats the factory dispatches to a daemon, and the capability
	// is already set declaratively from the cached manifest (RegisterFormatInfo),
	// so we never instantiate a plugin writer here. This keeps capability
	// resolution plugin-load-free (AD-005 "Writer output modes").
	if info.Source == "" || info.Source == SourceBuiltIn {
		w := factory()
		if gw, ok := w.(format.GenerativeWriter); ok {
			info.Generative = gw.Generative()
		}
		if iw, ok := w.(format.InterchangeWriter); ok {
			info.Interchange = iw.IsInterchange()
		}
	}
}

// RegisterAlias registers a name-only alias that resolves to a
// canonical format id. Looking up a reader or writer by the alias
// (NewReader / NewWriter / ResolveReader / ResolveWriter / HasReader /
// HasWriter) transparently resolves to the canonical id's factory.
//
// An alias is deliberately *not* a detection signature and gets *no*
// FormatInfo entry: auto-detection (by extension, MIME, or content)
// and `kapi formats` always surface the canonical id, never the alias.
// This is how a renamed format keeps its old `--format <old>` id
// working without polluting the format listing or creating a
// detection conflict between two ids claiming the same extension.
//
// Registering an alias whose name equals the canonical id is a no-op.
func (r *FormatRegistry) RegisterAlias(alias, canonical FormatID) {
	if alias == canonical || alias == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aliases[alias] = canonical
}

// AliasTarget returns the canonical id an alias resolves to, and
// whether the name is a registered alias.
func (r *FormatRegistry) AliasTarget(alias FormatID) (FormatID, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	canonical, ok := r.aliases[alias]
	return canonical, ok
}

// resolveAlias returns the canonical id for a name. If the name is a
// registered alias it returns the alias target; otherwise it returns
// the name unchanged. Caller must NOT hold the lock.
func (r *FormatRegistry) resolveAlias(name FormatID) FormatID {
	r.mu.RLock()
	canonical, ok := r.aliases[name]
	r.mu.RUnlock()
	if ok {
		return canonical
	}
	return name
}

// RegisterFormatInfo registers format metadata without a reader/writer factory.
// This is used during plugin metadata scanning so that "formats list" can show
// bridge-provided formats before the bridge process is started.
// When the bridge is later loaded, RegisterReader/RegisterWriter will update
// the existing info entry with HasReader/HasWriter = true.
func (r *FormatRegistry) RegisterFormatInfo(name FormatID, info FormatInfo) {
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
	// Generative / Interchange are declared by plugins in the cached manifest;
	// carry them onto the seeded info so the conversion-target query needs no
	// plugin load.
	if info.Generative {
		existing.Generative = true
	}
	if info.Interchange {
		existing.Interchange = true
	}

	// Register detection signature so bridge/plugin formats participate in
	// DetectByExtension and DetectByMIME from metadata scan time, before
	// the bridge process or plugin is actually loaded.
	if len(existing.Extensions) > 0 || len(existing.MimeTypes) > 0 {
		r.detector.Register(string(name), format.FormatSignature{
			Extensions: existing.Extensions,
			MIMETypes:  existing.MimeTypes,
		})
		// Apply priority: use existing priority if set, otherwise default plugin priority.
		pri := existing.Priority
		if pri == 0 {
			pri = format.DefaultPluginPriority
			existing.Priority = pri
		}
		r.detector.SetPriority(string(name), pri)
	}
}

// SetFormatSource sets the source (provider) for a format. Use "built-in" for
// built-in formats or the plugin name for plugin-provided formats.
// Plugin formats automatically receive DefaultPluginPriority unless a priority
// has already been explicitly set.
func (r *FormatRegistry) SetFormatSource(name FormatID, source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info := r.getOrCreateInfo(name)
	info.Source = source

	// Assign default plugin priority if the source is not built-in and no
	// explicit priority has been set (priority is still at default built-in).
	if source != "" && source != SourceBuiltIn && info.Priority == format.DefaultBuiltInPriority {
		info.Priority = format.DefaultPluginPriority
		r.detector.SetPriority(string(name), format.DefaultPluginPriority)
	}
}

// SetFormatPriority sets an explicit priority for the named format. Higher
// values are preferred when multiple formats match the same MIME type or
// extension during detection.
func (r *FormatRegistry) SetFormatPriority(name FormatID, priority int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info := r.getOrCreateInfo(name)
	info.Priority = priority
	r.detector.SetPriority(string(name), priority)
}

// ResolveFormat finds the best format name for a given MIME type by consulting
// the detector (which considers priorities). Returns an empty string if no
// format matches.
func (r *FormatRegistry) ResolveFormat(mimeType string) FormatID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, err := r.detector.DetectByMIME(mimeType)
	if err != nil {
		return ""
	}
	return FormatID(name)
}

// FormatInfos returns metadata for all registered formats, sorted by name.
func (r *FormatRegistry) FormatInfos() []FormatInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]FormatInfo, 0, len(r.infos))
	for _, info := range r.infos {
		cp := *info
		if cp.Source == "" {
			cp.Source = SourceBuiltIn
		}
		result = append(result, cp)
	}

	slices.SortFunc(result, func(a, b FormatInfo) int {
		return cmp.Compare(string(a.Name), string(b.Name))
	})
	return result
}

// FormatInfo returns metadata for a specific format, or nil if not found.
func (r *FormatRegistry) FormatInfo(name FormatID) *FormatInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.infos[name]
	if !ok {
		return nil
	}
	cp := *info
	if cp.Source == "" {
		cp.Source = SourceBuiltIn
	}
	return &cp
}

// getOrCreateInfo returns the FormatInfo for the given name, creating it if needed.
// Caller must hold the write lock.
func (r *FormatRegistry) getOrCreateInfo(name FormatID) *FormatInfo {
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
func (r *FormatRegistry) DetectByExtension(ext string) (FormatID, error) {
	if name, err := r.detector.DetectByExtension(ext); err == nil {
		return FormatID(name), nil
	}
	if r.triggerOnMiss() {
		name, err := r.detector.DetectByExtension(ext)
		return FormatID(name), err
	}
	return "", fmt.Errorf("no format found for extension %q", ext)
}

// DetectByExtensionForSources detects a format by extension, restricted to
// formats whose Source matches one of the allowed sources. Pass nil or empty
// to allow all sources (equivalent to DetectByExtension).
//
// This enables project-scoped format detection: a project that doesn't declare
// a plugin should not auto-detect plugin-provided formats. Pass
// []string{"built-in"} to restrict to built-in formats only, or
// []string{"built-in", "okapi-bridge"} to also include that plugin's formats.
func (r *FormatRegistry) DetectByExtensionForSources(ext string, allowedSources []string) (FormatID, error) {
	return r.DetectByExtensionForSourcesWithPriorities(ext, allowedSources, nil)
}

// DetectByExtensionForSourcesWithPriorities is DetectByExtensionForSources with
// per-call priority overrides. overrides maps a format name to a priority that
// takes precedence over the registry's stored priority for the duration of this
// detection only — used by project-scoped detection so a recipe's
// `defaults.formats[name].priority` can pick the preferred engine for an
// extension claimed by several formats (e.g. okf_vtt over okf_regex for .srt)
// without mutating global registry state (which would race across open projects).
func (r *FormatRegistry) DetectByExtensionForSourcesWithPriorities(ext string, allowedSources []string, overrides map[string]int) (FormatID, error) {
	if len(allowedSources) == 0 && len(overrides) == 0 {
		return r.DetectByExtension(ext)
	}
	var allowed map[string]bool
	if len(allowedSources) > 0 {
		allowed = make(map[string]bool, len(allowedSources))
		for _, s := range allowedSources {
			allowed[s] = true
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	ext = strings.ToLower(ext)
	if ext == "" {
		return "", errors.New("empty extension")
	}

	var bestName FormatID
	bestPriority := -1
	for name, info := range r.infos {
		source := info.Source
		if source == "" {
			source = SourceBuiltIn
		}
		if allowed != nil && !allowed[source] {
			continue
		}
		for _, e := range info.Extensions {
			if strings.ToLower(e) == ext {
				pri := r.detector.Priority(string(name))
				if ov, ok := overrides[string(name)]; ok {
					pri = ov
				}
				if bestName == "" || pri > bestPriority || (pri == bestPriority && string(name) < string(bestName)) {
					bestName = name
					bestPriority = pri
				}
			}
		}
	}
	if bestName != "" {
		return bestName, nil
	}
	return "", fmt.Errorf("no format found for extension %q with allowed sources", ext)
}

// DetectFile detects a format for a file by extension AND, when that extension
// is claimed by more than one format, by content. This stops a file from being
// resolved purely on its extension when several formats share it — e.g. an
// ".xliff" can be XLIFF 1.x or 2.x, and ".xml" is claimed by many formats; the
// file head decides which. allowedSources restricts to those plugin sources
// (nil/empty = all), mirroring DetectByExtensionForSources. Only the file head
// is read; on any read error it falls back to extension-only detection.
func (r *FormatRegistry) DetectFile(path string, allowedSources []string) (FormatID, error) {
	return r.DetectFileWithPriorities(path, allowedSources, nil)
}

// DetectFileWithPriorities is DetectFile with per-call priority overrides (see
// DetectByExtensionForSourcesWithPriorities). Content sniffing still decides
// among candidates that claim the same extension; the overrides only break the
// deterministic extension/priority tie when sniffing is inconclusive.
func (r *FormatRegistry) DetectFileWithPriorities(path string, allowedSources []string, overrides map[string]int) (FormatID, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return "", fmt.Errorf("no extension to detect: %q", path)
	}

	var allowed map[string]bool
	if len(allowedSources) > 0 {
		allowed = make(map[string]bool, len(allowedSources))
		for _, s := range allowedSources {
			allowed[s] = true
		}
	}

	// Collect the formats claiming this extension (honouring allowed sources).
	r.mu.RLock()
	cands := make(map[string]bool)
	for name, info := range r.infos {
		src := info.Source
		if src == "" {
			src = SourceBuiltIn
		}
		if allowed != nil && !allowed[src] {
			continue
		}
		for _, e := range info.Extensions {
			if strings.ToLower(e) == ext {
				cands[string(name)] = true
				break
			}
		}
	}
	r.mu.RUnlock()

	// More than one claimant → let the content sniff pick among them.
	if len(cands) > 1 {
		if f, err := os.Open(path); err == nil {
			name, derr := r.detector.DetectByContent(f)
			f.Close()
			if derr == nil && cands[name] {
				return FormatID(name), nil
			}
		}
	}

	// Single claimant, unreadable file, or content didn't match a candidate:
	// fall back to the deterministic extension/priority pick.
	return r.DetectByExtensionForSourcesWithPriorities(ext, allowedSources, overrides)
}

// NewReader creates a new reader instance for the given format name.
// If the name contains "@", it looks up the exact versioned name.
// If the bare name is not found, it scans for versioned entries and
// returns the latest version as a fallback.
// If no reader is found and an onMiss callback is set, it triggers
// lazy loading (e.g., starting bridge processes) and retries once.
func (r *FormatRegistry) NewReader(name FormatID) (format.DataFormatReader, error) {
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
func (r *FormatRegistry) NewWriter(name FormatID) (format.DataFormatWriter, error) {
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
// A registered alias resolves to its canonical id first.
func (r *FormatRegistry) findReader(name FormatID) FormatReaderFactory {
	name = r.resolveAlias(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if factory, ok := r.readers[name]; ok {
		return factory
	}
	if !strings.Contains(string(name), "@") {
		if f, ok := findLatest(r.readers, name); ok {
			return f
		}
	}
	return nil
}

// findWriter looks up a writer factory by exact name or latest versioned entry.
// A registered alias resolves to its canonical id first.
func (r *FormatRegistry) findWriter(name FormatID) FormatWriterFactory {
	name = r.resolveAlias(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if factory, ok := r.writers[name]; ok {
		return factory
	}
	if !strings.Contains(string(name), "@") {
		if f, ok := findLatest(r.writers, name); ok {
			return f
		}
	}
	return nil
}

// findLatest scans a map for entries matching "prefix@version" and returns the
// value for the highest semantic version. This generic helper collapses the
// previously separate findLatestReader and findLatestWriter functions.
func findLatest[F any](m map[FormatID]F, name FormatID) (F, bool) {
	prefix := string(name) + "@"
	var bestVersion string
	var bestFactory F
	var found bool
	for key, factory := range m {
		if !strings.HasPrefix(string(key), prefix) {
			continue
		}
		version := string(key)[len(prefix):]
		if !found || compareSemver(version, bestVersion) > 0 {
			bestVersion = version
			bestFactory = factory
			found = true
		}
	}
	return bestFactory, found
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
		if strings.Contains(string(name), "@") {
			continue
		}
		reader := factory()
		cfg := reader.Config()
		if cfg == nil {
			continue
		}
		if sp, ok := cfg.(format.SchemaProvider); ok {
			schemaReg.RegisterSchema(string(name), sp.Schema())
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
		if strings.Contains(string(name), "@") {
			continue
		}
		reader := factory()
		cfg := reader.Config()
		if cfg == nil {
			continue
		}

		kind := config.FormatConfigKind(string(name))
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
	return r.NewReader(FormatID(name))
}

// ResolveWriter creates a new writer for the named format. Implements SubfilterResolver.
func (r *FormatRegistry) ResolveWriter(name string) (format.DataFormatWriter, error) {
	return r.NewWriter(FormatID(name))
}

// Detector returns the format Detector backed by this registry.
func (r *FormatRegistry) Detector() *format.Detector {
	return r.detector
}

// ReaderNames returns the names of all registered readers.
func (r *FormatRegistry) ReaderNames() []FormatID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]FormatID, 0, len(r.readers))
	for name := range r.readers {
		names = append(names, name)
	}
	return names
}

// WriterNames returns the names of all registered writers.
func (r *FormatRegistry) WriterNames() []FormatID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]FormatID, 0, len(r.writers))
	for name := range r.writers {
		names = append(names, name)
	}
	return names
}

// HasReader returns true if a reader is registered for the given format
// name. A registered alias resolves to its canonical id first.
func (r *FormatRegistry) HasReader(name FormatID) bool {
	name = r.resolveAlias(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.readers[name]
	return ok
}

// HasWriter returns true if a writer is registered for the given format
// name. A registered alias resolves to its canonical id first.
func (r *FormatRegistry) HasWriter(name FormatID) bool {
	name = r.resolveAlias(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.writers[name]
	return ok
}

// ReaderFactory returns the reader factory for the given format name, or nil.
// Use this to build alias factories without triggering lock re-entry.
func (r *FormatRegistry) ReaderFactory(name FormatID) FormatReaderFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.readers[name]
}

// WriterFactory returns the writer factory for the given format name, or nil.
// Use this to build alias factories without triggering lock re-entry.
func (r *FormatRegistry) WriterFactory(name FormatID) FormatWriterFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.writers[name]
}
