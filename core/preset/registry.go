package preset

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	slices.Sort(names)
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
	slices.Sort(names)
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
	slices.Sort(names)
	result := make([]*FrameworkPreset, len(names))
	for i, name := range names {
		result[i] = r.frameworkPresets[name]
	}
	return result
}

// DetectFrameworkPreset scans basePath for telltale files defined by each
// preset's Detect field and returns the first matching preset name.
//
// Detect entries are either:
//   - "filename" — matches if the file exists in basePath.
//   - "filename:substring" — matches if the file exists AND contains the substring.
func (r *PresetRegistry) DetectFrameworkPreset(basePath string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check presets in sorted order for deterministic results.
	names := make([]string, 0, len(r.frameworkPresets))
	for name := range r.frameworkPresets {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		p := r.frameworkPresets[name]
		if matchDetect(basePath, p.Detect) {
			return name
		}
	}
	return ""
}

// matchDetect returns true if any entry in detect matches the given basePath.
func matchDetect(basePath string, detect []string) bool {
	for _, entry := range detect {
		file, substring, hasSubstring := strings.Cut(entry, ":")
		path := filepath.Join(basePath, file)

		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}

		if !hasSubstring {
			return true
		}

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if bytes.Contains(data, []byte(substring)) {
			return true
		}
	}
	return false
}
