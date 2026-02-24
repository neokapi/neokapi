package preset

import (
	"sort"
	"sync"
)

// PresetRegistry is a thread-safe registry for format and framework presets.
type PresetRegistry struct {
	mu               sync.RWMutex
	formatPresets    map[string]map[string]*FormatPreset // format -> presetName -> preset
	frameworkPresets map[string]*FrameworkPreset         // name -> preset
}

// NewPresetRegistry creates an empty PresetRegistry.
func NewPresetRegistry() *PresetRegistry {
	return &PresetRegistry{
		formatPresets:    make(map[string]map[string]*FormatPreset),
		frameworkPresets: make(map[string]*FrameworkPreset),
	}
}

// RegisterFormatPreset stores a format preset under the given format and name.
func (r *PresetRegistry) RegisterFormatPreset(format, name string, p *FormatPreset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.formatPresets[format] == nil {
		r.formatPresets[format] = make(map[string]*FormatPreset)
	}
	r.formatPresets[format][name] = p
}

// GetFormatPreset returns the preset for the given format and name, or nil if not found.
func (r *PresetRegistry) GetFormatPreset(format, name string) *FormatPreset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m := r.formatPresets[format]
	if m == nil {
		return nil
	}
	return m[name]
}

// ListFormatPresets returns all presets for a format, sorted by name.
// Returns nil if no presets are registered for the format.
func (r *PresetRegistry) ListFormatPresets(format string) []*FormatPreset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m := r.formatPresets[format]
	if len(m) == 0 {
		return nil
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]*FormatPreset, len(names))
	for i, name := range names {
		result[i] = m[name]
	}
	return result
}

// FormatNames returns sorted format names that have presets registered.
func (r *PresetRegistry) FormatNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.formatPresets))
	for name := range r.formatPresets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RegisterFrameworkPreset stores a framework preset under the given name.
func (r *PresetRegistry) RegisterFrameworkPreset(name string, p *FrameworkPreset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frameworkPresets[name] = p
}

// GetFrameworkPreset returns the framework preset with the given name, or nil if not found.
func (r *PresetRegistry) GetFrameworkPreset(name string) *FrameworkPreset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.frameworkPresets[name]
}

// ListFrameworkPresets returns all framework presets, sorted by name.
func (r *PresetRegistry) ListFrameworkPresets() []*FrameworkPreset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.frameworkPresets) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.frameworkPresets))
	for name := range r.frameworkPresets {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]*FrameworkPreset, len(names))
	for i, name := range names {
		result[i] = r.frameworkPresets[name]
	}
	return result
}
