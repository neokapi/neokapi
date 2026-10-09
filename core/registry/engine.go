package registry

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/format"
)

// EngineNative is the user-facing name of the built-in format engine. A
// recipe, a config file and the --engine flag name the built-in formats this
// way; the registry stores them under SourceBuiltIn.
const EngineNative = "native"

// NormalizeEngine maps a user-facing engine name to the registry source it
// names: "native" and "built-in" both name the built-in formats; any other
// name is a plugin's. An empty name stays empty.
func NormalizeEngine(name string) string {
	name = strings.TrimSpace(name)
	if name == EngineNative || name == SourceBuiltIn {
		return SourceBuiltIn
	}
	return name
}

// EngineName renders a registry source as the engine name a person reads:
// the built-in source is "native"; a plugin is its own name.
func EngineName(source string) string {
	if source == "" || source == SourceBuiltIn {
		return EngineNative
	}
	return source
}

// Rules a Resolution reports as the one that chose the engine.
const (
	// RuleFormatEngine: a per-format engine pin (DetectOptions.FormatEngines)
	// named the engine for this extension or one of its claimants.
	RuleFormatEngine = "format engine"
	// RuleEngineOverride: the engine the host forced for the run (--engine).
	RuleEngineOverride = "engine override"
	// RuleEnginePreference: the engine the detection asked for
	// (DetectOptions.Engine, which a recipe's defaults.engine fills).
	RuleEnginePreference = "engine preference"
	// RuleEngineDefault: the engine the host configured as the default
	// (formats.engine in the user config).
	RuleEngineDefault = "engine default"
	// RuleNativeFirst: no preference applied, and a built-in format claims
	// the extension.
	RuleNativeFirst = "native first"
	// RuleEngineOrder: no built-in claims the extension, and the ranked order
	// of the other engines (DetectOptions.EngineOrder, then name order) chose
	// one.
	RuleEngineOrder = "engine order"
)

// Resolution is the trace of one format resolution: what was chosen, from
// which engine, by which rule, and what else claimed the extension.
type Resolution struct {
	// Format is the chosen format id.
	Format FormatID
	// Engine is the chosen format's engine, as EngineName renders it.
	Engine string
	// Rule is the rule that chose the engine (one of the Rule* constants).
	Rule string
	// Content reports that the file's content chose among the engine's
	// candidates; false means priority and name order did, or there was one.
	Content bool
	// Order is the engine order consulted, as EngineName renders it.
	Order []string
	// Candidates is every format that claimed the extension within the
	// allowed sources, in name order.
	Candidates []FormatID
}

// SetEngineOverride names the engine the host forces for every detection:
// the --engine flag. It ranks above a detection's own preference
// (DetectOptions.Engine) and above the configured default. Empty clears it.
// "native" and "built-in" both name the built-in formats.
func (r *FormatRegistry) SetEngineOverride(engine string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engineOverride = NormalizeEngine(engine)
}

// SetDefaultEngine names the engine preferred when neither the host nor the
// detection names one: the user config's formats.engine. Empty clears it,
// which leaves the built-in native-first order.
func (r *FormatRegistry) SetDefaultEngine(engine string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultEngine = NormalizeEngine(engine)
}

// EngineOverride returns the engine SetEngineOverride set, as the registry
// stores it ("" when none).
func (r *FormatRegistry) EngineOverride() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.engineOverride
}

// DefaultEngine returns the engine SetDefaultEngine set, as the registry
// stores it ("" when none).
func (r *FormatRegistry) DefaultEngine() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultEngine
}

// Sources returns every source the registered formats come from, the
// built-in source first and the plugins in name order.
func (r *FormatRegistry) Sources() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]bool{SourceBuiltIn: true}
	var plugins []string
	for _, info := range r.infos {
		src := info.Source
		if src == "" || seen[src] {
			continue
		}
		seen[src] = true
		plugins = append(plugins, src)
	}
	slices.Sort(plugins)
	return append([]string{SourceBuiltIn}, plugins...)
}

// HasSource reports whether engine names the built-in formats or a source
// some registered format comes from.
func (r *FormatRegistry) HasSource(engine string) bool {
	engine = NormalizeEngine(engine)
	return engine != "" && slices.Contains(r.Sources(), engine)
}

// FormatSource returns the source a format comes from: SourceBuiltIn for a
// built-in or unknown format, else the plugin's name.
func (r *FormatRegistry) FormatSource(name FormatID) string {
	name = r.resolveAlias(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if info, ok := r.infos[name]; ok && info.Source != "" {
		return info.Source
	}
	return SourceBuiltIn
}

// candidate is one format that claims an extension.
type candidate struct {
	name     FormatID
	source   string
	priority int
}

// Resolve is Detect with the decision reported. It is the one resolver every
// surface that detects a format for a named file goes through: the engine
// order decides which engine serves the extension, then the file's content
// (when several of that engine's formats claim it) or the formats'
// priorities decide which of its formats does.
//
// The engine order, first to last: a per-format pin (opts.FormatEngines),
// the host's override (SetEngineOverride), the detection's preference
// (opts.Engine), the host's default (SetDefaultEngine), the built-in
// formats, the ranked engines (opts.EngineOrder), then every other source in
// name order. The first engine with a format claiming the extension wins. A
// priority never moves a format across engines.
func (r *FormatRegistry) Resolve(path string, opts DetectOptions) (Resolution, error) {
	ext := format.Ext(path)
	if ext == "" {
		return Resolution{}, fmt.Errorf("no extension to detect: %q", path)
	}
	res, cands, err := r.resolveExtension(ext, opts)
	if err != nil {
		return Resolution{}, err
	}
	if len(cands) == 1 || opts.ExtensionOnly {
		return res, nil
	}
	// Several of the engine's formats claim the extension: the file's head
	// decides among them when it can be read and names one of them.
	open := opts.Content
	if open == nil {
		open = func() (io.ReadSeeker, error) { return openFile(path) }
	}
	content, oerr := open()
	if oerr != nil {
		return res, nil
	}
	name, derr := r.detector.DetectByContent(content)
	if c, ok := content.(io.Closer); ok {
		_ = c.Close()
	}
	if derr != nil {
		return res, nil
	}
	for _, c := range cands {
		if string(c.name) == name {
			res.Format = c.name
			res.Content = true
			return res, nil
		}
	}
	return res, nil
}

// resolveExtension ranks the engines for ext, picks the first with a
// claimant and returns the resolution by priority among that engine's
// claimants, with those claimants for a content sniff to choose among.
func (r *FormatRegistry) resolveExtension(ext string, opts DetectOptions) (Resolution, []candidate, error) {
	all := r.claimants(ext, opts)
	if len(all) == 0 && len(opts.AllowedSources) == 0 {
		// Nothing claims the extension. A lazy loader may register the format
		// that does; ask it once and look again.
		if r.triggerOnMiss() {
			all = r.claimants(ext, opts)
		}
	}
	if len(all) == 0 {
		if len(opts.AllowedSources) > 0 {
			return Resolution{}, nil, fmt.Errorf("no format found for extension %q with allowed sources", ext)
		}
		return Resolution{}, nil, fmt.Errorf("no format found for extension %q", ext)
	}

	order, rules := r.engineOrder(ext, all, opts)
	var chosen string
	var rule string
	for i, engine := range order {
		if slices.ContainsFunc(all, func(c candidate) bool { return c.source == engine }) {
			chosen, rule = engine, rules[i]
			break
		}
	}

	var cands []candidate
	for _, c := range all {
		if c.source == chosen {
			cands = append(cands, c)
		}
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.priority > best.priority || (c.priority == best.priority && c.name < best.name) {
			best = c
		}
	}

	res := Resolution{Format: best.name, Engine: EngineName(chosen), Rule: rule}
	for _, e := range order {
		res.Order = append(res.Order, EngineName(e))
	}
	for _, c := range all {
		res.Candidates = append(res.Candidates, c.name)
	}
	return res, cands, nil
}

// claimants lists the formats claiming ext within the allowed sources, in
// name order, each with the priority the detection sees. A reader
// resolution prefers formats with a reader and a writer resolution takes
// only formats with a writer.
func (r *FormatRegistry) claimants(ext string, opts DetectOptions) []candidate {
	var allowed map[string]bool
	if len(opts.AllowedSources) > 0 {
		allowed = make(map[string]bool, len(opts.AllowedSources))
		for _, s := range opts.AllowedSources {
			allowed[NormalizeEngine(s)] = true
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out, readable []candidate
	for name, info := range r.infos {
		source := info.Source
		if source == "" {
			source = SourceBuiltIn
		}
		if allowed != nil && !allowed[source] {
			continue
		}
		if opts.Writer && !info.HasWriter {
			continue
		}
		if !slices.ContainsFunc(info.Extensions, func(e string) bool { return strings.EqualFold(e, ext) }) {
			continue
		}
		pri := r.detector.Priority(string(name))
		if ov, ok := opts.PriorityOverrides[string(name)]; ok {
			pri = ov
		}
		c := candidate{name: name, source: source, priority: pri}
		out = append(out, c)
		if info.HasReader {
			readable = append(readable, c)
		}
	}
	// A format registered for writing alone never reads the file, so it is
	// a reader candidate only when nothing that reads claims the extension.
	if !opts.Writer && len(readable) > 0 {
		out = readable
	}
	slices.SortFunc(out, func(a, b candidate) int { return strings.Compare(string(a.name), string(b.name)) })
	return out
}

// engineOrder is the ordered list of engines consulted for ext, each with
// the rule that placed it. Caller must not hold the lock.
func (r *FormatRegistry) engineOrder(ext string, all []candidate, opts DetectOptions) ([]string, []string) {
	r.mu.RLock()
	override, def := r.engineOverride, r.defaultEngine
	r.mu.RUnlock()

	var order, rules []string
	add := func(engine, rule string) {
		engine = NormalizeEngine(engine)
		if engine == "" || slices.Contains(order, engine) {
			return
		}
		order = append(order, engine)
		rules = append(rules, rule)
	}
	if e, ok := lookupFold(opts.FormatEngines, ext); ok {
		add(e, RuleFormatEngine)
	}
	for _, c := range all {
		if e, ok := opts.FormatEngines[string(c.name)]; ok {
			add(e, RuleFormatEngine)
		}
	}
	add(override, RuleEngineOverride)
	add(opts.Engine, RuleEnginePreference)
	add(def, RuleEngineDefault)
	add(SourceBuiltIn, RuleNativeFirst)
	for _, e := range opts.EngineOrder {
		add(e, RuleEngineOrder)
	}
	for _, c := range all {
		add(c.source, RuleEngineOrder)
	}
	return order, rules
}

// lookupFold finds ext in m, matching the extension without regard to case.
func lookupFold(m map[string]string, ext string) (string, bool) {
	if len(m) == 0 {
		return "", false
	}
	if e, ok := m[ext]; ok {
		return e, true
	}
	for k, e := range m {
		if strings.EqualFold(k, ext) {
			return e, true
		}
	}
	return "", false
}

// ErrNoEngine reports an engine name no registered format comes from.
var ErrNoEngine = errors.New("unknown format engine")

// CheckEngine reports an error when engine names neither the built-in
// formats nor a source a registered format comes from. An empty name is no
// engine and passes.
func (r *FormatRegistry) CheckEngine(engine string) error {
	if strings.TrimSpace(engine) == "" || r.HasSource(engine) {
		return nil
	}
	names := make([]string, 0, 4)
	for _, s := range r.Sources() {
		names = append(names, EngineName(s))
	}
	return fmt.Errorf("%w %q. Installed engines: %s", ErrNoEngine, engine, strings.Join(names, ", "))
}
