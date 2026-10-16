package registry

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ExtensionEngines indexes the extensions the registered formats claim by
// the engines claiming them: a lower-cased extension maps to its engines as
// EngineName renders them, native first and the plugins in name order. An
// extension with more than one engine is one the engine order decides.
func (r *FormatRegistry) ExtensionEngines() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sources := make(map[string]map[string]bool)
	for _, info := range r.infos {
		source := info.Source
		if source == "" {
			source = SourceBuiltIn
		}
		for _, ext := range info.Extensions {
			ext = strings.ToLower(ext)
			if sources[ext] == nil {
				sources[ext] = make(map[string]bool)
			}
			sources[ext][source] = true
		}
	}
	out := make(map[string][]string, len(sources))
	for ext, set := range sources {
		var plugins []string
		for source := range set {
			if source != SourceBuiltIn {
				plugins = append(plugins, source)
			}
		}
		slices.Sort(plugins)
		engines := make([]string, 0, len(set))
		if set[SourceBuiltIn] {
			engines = append(engines, EngineNative)
		}
		out[ext] = append(engines, plugins...)
	}
	return out
}

// SharedExtensions lists the extensions of the named format that a format
// of another engine also claims, in the order the format declares them.
// Empty when every extension is the engine's alone.
func (r *FormatRegistry) SharedExtensions(name FormatID) []string {
	info := r.FormatInfo(name)
	if info == nil {
		return nil
	}
	index := r.ExtensionEngines()
	var shared []string
	for _, ext := range info.Extensions {
		if len(index[strings.ToLower(ext)]) > 1 {
			shared = append(shared, ext)
		}
	}
	return shared
}

// EngineFormat names the format of engine that reads the logical format
// name names: name itself when it belongs to the engine, else the engine's
// format claiming the most of name's extensions, with priority and then name
// order breaking a tie. A name no format carries is read as an extension
// (`strings` for `.strings`). The error names the engine and the extensions
// when the engine has no format for them.
func (r *FormatRegistry) EngineFormat(name FormatID, engine string) (FormatID, error) {
	engine = NormalizeEngine(engine)
	if engine == "" {
		return name, nil
	}
	name = r.resolveAlias(name)
	var exts []string
	if info := r.FormatInfo(name); info != nil {
		if info.Source == engine {
			return name, nil
		}
		exts = info.Extensions
	} else {
		exts = []string{"." + strings.TrimPrefix(strings.ToLower(string(name)), ".")}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	type match struct {
		name     FormatID
		overlap  int
		priority int
	}
	var best *match
	for id, info := range r.infos {
		source := info.Source
		if source == "" {
			source = SourceBuiltIn
		}
		if source != engine || !info.HasReader {
			continue
		}
		overlap := 0
		for _, ext := range info.Extensions {
			if slices.ContainsFunc(exts, func(e string) bool { return strings.EqualFold(e, ext) }) {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}
		m := match{name: id, overlap: overlap, priority: r.detector.Priority(string(id))}
		if best == nil || m.overlap > best.overlap ||
			(m.overlap == best.overlap && (m.priority > best.priority ||
				(m.priority == best.priority && m.name < best.name))) {
			best = &m
		}
	}
	if best == nil {
		return "", fmt.Errorf("%w: no format of engine %q reads %s (%s)", ErrNoEngineFormat, EngineName(engine), name, strings.Join(exts, ", "))
	}
	return best.name, nil
}

// ErrNoEngineFormat reports an engine with no format for the logical format
// a reference names.
var ErrNoEngineFormat = errors.New("no format for engine")
