package host

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Every MCP tool argument that can carry a string is declared either as a
// location or as not a location. The roots middleware (mcp_roots.go) checks
// each location before the tool runs, and refuses an argument the tool's
// schema does not list.
//
// An argument is named by its path in the input schema: `name` at the top
// level, `parent.child` for a property of an object, with arrays transparent
// (`ops.at.doc` is the `doc` of the `at` of every element of `ops`) and `*`
// for the values of a map. The declarations come from two places:
//
//   - The schema itself. An `x-path` annotation declares a location, and
//     `"x-not-location": true` declares none. Either one on an object or an
//     array covers everything inside it that does not annotate itself. A
//     framework tool writes them with the `path=` key of a config field's
//     schema tag: `path=file` or `path=directory` for a location,
//     `path=none` for none (core/schema).
//   - DeclareMCPPathArgs, called beside the registration of a tool with a
//     typed input. A declaration of an object or an array covers every
//     argument inside it that is not declared itself, as an annotation does.
//
// A declaration by DeclareMCPPathArgs wins over the schema's. `project` at
// the top level is a location unless declared otherwise. The audit
// (UndeclaredMCPPathArgs, run by TestEveryMCPPathArgumentIsDeclared and its
// siblings in each module that registers tools) fails on any string argument
// declared neither way, whatever its name, and on a declaration that names an
// argument the schema does not have. A plugin's proxied tool arrives at run
// time, past the audit: every string argument its schema leaves undeclared is
// checked as a location (mcp_plugin_proxy.go).

// contextURIScheme is the address space the by-location primitive lives in.
const contextURIScheme = "context://"

// MCPArgKind is how a declared argument names a location.
type MCPArgKind int

const (
	// MCPArgPath is a file system path, absolute or relative to the directory
	// the server was started in. It must lie under a directory the call may
	// reach (MCPReachable).
	MCPArgPath MCPArgKind = iota + 1
	// MCPArgProjectRelative is a path inside the project the call acts on. It
	// must be local (no root, no climbing), or, spelled absolute, lie under a
	// directory the call may reach.
	MCPArgProjectRelative
	// MCPArgNotLocation declares an argument that names no location (text to
	// process, a revision, a pack name, a locale). It is never checked.
	MCPArgNotLocation
)

// mcpNotLocationKey is the schema annotation that declares an argument names
// no location, the counterpart of x-path.
const mcpNotLocationKey = "x-not-location"

var (
	mcpPathArgsMu sync.RWMutex
	mcpPathArgs   = map[string]map[string]MCPArgKind{}
)

// DeclareMCPPathArgs declares how each of args, a path in tool's input schema
// (see above), names a location, or that it names none. Call it beside the
// tool's registration.
func DeclareMCPPathArgs(tool string, kind MCPArgKind, args ...string) {
	mcpPathArgsMu.Lock()
	defer mcpPathArgsMu.Unlock()
	m := mcpPathArgs[tool]
	if m == nil {
		m = map[string]MCPArgKind{}
		mcpPathArgs[tool] = m
	}
	for _, a := range args {
		m[a] = kind
	}
}

// declaredMCPPathArgs returns the arguments of tool declared with
// DeclareMCPPathArgs.
func declaredMCPPathArgs(tool string) map[string]MCPArgKind {
	mcpPathArgsMu.RLock()
	defer mcpPathArgsMu.RUnlock()
	out := make(map[string]MCPArgKind, len(mcpPathArgs[tool]))
	maps.Copy(out, mcpPathArgs[tool])
	return out
}

// mcpToolSchemas holds the input schema of every tool a server lists, read
// once from the server itself.
type mcpToolSchemas struct {
	once  sync.Once
	props map[string]mcpInputSchema
	err   error
}

// load lists the server's tools through next, the server's own handler chain.
func (s *mcpToolSchemas) load(ctx context.Context, next mcp.MethodHandler, ss *mcp.ServerSession) {
	s.once.Do(func() {
		s.props = map[string]mcpInputSchema{}
		var cursor string
		for {
			res, err := next(ctx, "tools/list", &mcp.ListToolsRequest{Session: ss, Params: &mcp.ListToolsParams{Cursor: cursor}})
			if err != nil {
				s.err = err
				return
			}
			list, ok := res.(*mcp.ListToolsResult)
			if !ok {
				s.err = fmt.Errorf("tools/list answered with %T", res)
				return
			}
			for _, t := range list.Tools {
				s.props[t.Name] = readMCPInputSchema(t.InputSchema)
			}
			if list.NextCursor == "" {
				return
			}
			cursor = list.NextCursor
		}
	})
}

// mcpInputSchema is the part of a tool's input schema the declarations read:
// its properties, and the definitions a $ref inside them names.
type mcpInputSchema struct {
	Properties  map[string]json.RawMessage `json:"properties"`
	Defs        map[string]json.RawMessage `json:"$defs"`
	Definitions map[string]json.RawMessage `json:"definitions"`
}

// readMCPInputSchema reads a tool's input schema. One it cannot read has no
// properties.
func readMCPInputSchema(schema any) mcpInputSchema {
	var s mcpInputSchema
	if raw, err := json.Marshal(schema); err == nil {
		_ = json.Unmarshal(raw, &s)
	}
	if s.Properties == nil {
		s.Properties = map[string]json.RawMessage{}
	}
	return s
}

// mcpArgSchema is the part of a property schema the declarations read.
type mcpArgSchema struct {
	Ref                  string                     `json:"$ref"`
	Type                 any                        `json:"type"`
	XPath                json.RawMessage            `json:"x-path"`
	NotLocation          bool                       `json:"x-not-location"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Items                json.RawMessage            `json:"items"`
	AdditionalProperties json.RawMessage            `json:"additionalProperties"`
	AnyOf                []json.RawMessage          `json:"anyOf"`
	OneOf                []json.RawMessage          `json:"oneOf"`
	AllOf                []json.RawMessage          `json:"allOf"`
}

// mcpStringArg is one argument of a tool's input schema that can carry a
// string: a string, an array of strings, or an object whose schema leaves its
// contents open.
type mcpStringArg struct {
	// Path names the argument (see the top of this file).
	Path string
	// Kind is what the schema's own annotations declare, 0 for neither.
	Kind MCPArgKind
}

// mcpStringArgs returns every argument among props that can carry a string,
// at every depth, ordered by path.
func mcpStringArgs(schema mcpInputSchema) ([]mcpStringArg, error) {
	w := mcpArgWalker{
		defs:   map[string]json.RawMessage{},
		active: map[string]bool{},
		seen:   map[string]MCPArgKind{},
	}
	for name, raw := range schema.Definitions {
		w.defs["#/definitions/"+name] = raw
	}
	for name, raw := range schema.Defs {
		w.defs["#/$defs/"+name] = raw
	}
	for name, raw := range schema.Properties {
		if err := w.walk(name, raw, 0); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	out := make([]mcpStringArg, 0, len(w.seen))
	for p, k := range w.seen {
		out = append(out, mcpStringArg{Path: p, Kind: k})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// mcpArgWalker walks a tool's input schema for the arguments that can carry a
// string.
type mcpArgWalker struct {
	defs   map[string]json.RawMessage
	active map[string]bool // the $refs being walked, to stop at a cycle
	seen   map[string]MCPArgKind
}

// walk records each argument at or beneath path that can carry a string,
// with the kind its own schema, or the nearest enclosing one, annotates.
func (w *mcpArgWalker) walk(path string, raw json.RawMessage, inherited MCPArgKind) error {
	return w.walkBranch(path, raw, inherited, false)
}

// walkBranch is walk for a schema that may be a branch of an enclosing
// anyOf, oneOf or allOf. When the enclosing schema says what the value is
// (narrows is true), a branch that says nothing of its own, such as
// {"required": ["text"]}, only narrows it and carries nothing more.
func (w *mcpArgWalker) walkBranch(path string, raw json.RawMessage, inherited MCPArgKind, narrows bool) error {
	var s mcpArgSchema
	if err := json.Unmarshal(raw, &s); err != nil {
		// A boolean schema: true accepts anything, false nothing.
		var accepts bool
		if json.Unmarshal(raw, &accepts) != nil {
			return err
		}
		if accepts && !narrows {
			w.record(path, inherited)
		}
		return nil
	}
	kind := inherited
	switch {
	case s.NotLocation:
		kind = MCPArgNotLocation
	case len(s.XPath) > 0 && string(s.XPath) != "null" && string(s.XPath) != "false":
		kind = MCPArgPath
	}
	if s.Ref != "" {
		target, ok := w.defs[s.Ref]
		switch {
		case !ok || w.active[s.Ref]:
			// A reference kapi cannot follow, or one back into itself (a run
			// whose plural branches hold runs): everything beneath path is
			// one argument, declared as a whole.
			w.record(path, kind)
		default:
			w.active[s.Ref] = true
			err := w.walk(path, target, kind)
			delete(w.active, s.Ref)
			if err != nil {
				return err
			}
		}
	}
	types := schemaTypes(s.Type)
	says := len(types) > 0 || len(s.Properties) > 0 || len(s.Items) > 0 || s.Ref != ""
	alts := append(append(append([]json.RawMessage{}, s.AnyOf...), s.OneOf...), s.AllOf...)
	for _, alt := range alts {
		if err := w.walkBranch(path, alt, kind, says || narrows); err != nil {
			return err
		}
	}
	if len(types) == 0 {
		switch {
		case len(alts) > 0 || s.Ref != "":
			return nil
		case len(s.Properties) > 0:
			types["object"] = true
		case len(s.Items) > 0:
			types["array"] = true
		case narrows:
			return nil
		default:
			// An untyped schema accepts any value.
			w.record(path, kind)
			return nil
		}
	}
	if types["string"] {
		w.record(path, kind)
	}
	if types["array"] {
		if len(s.Items) == 0 {
			w.record(path, kind)
		} else if err := w.walk(path, s.Items, kind); err != nil {
			return err
		}
	}
	if types["object"] {
		for name, child := range s.Properties {
			if err := w.walk(path+"."+name, child, kind); err != nil {
				return err
			}
		}
		switch extra := strings.TrimSpace(string(s.AdditionalProperties)); {
		case extra == "false":
		case strings.HasPrefix(extra, "{"):
			if err := w.walk(path+".*", s.AdditionalProperties, kind); err != nil {
				return err
			}
		case len(s.Properties) == 0:
			// An object whose schema leaves its contents open can carry a
			// string under any key.
			w.record(path, kind)
		}
	}
	return nil
}

// record records path, keeping the first annotation found for it.
func (w *mcpArgWalker) record(path string, kind MCPArgKind) {
	if prev, ok := w.seen[path]; ok && prev != 0 {
		return
	}
	w.seen[path] = kind
}

// schemaTypes returns the JSON schema types t names.
func schemaTypes(t any) map[string]bool {
	out := map[string]bool{}
	switch v := t.(type) {
	case string:
		out[v] = true
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}

// mcpArgKinds returns how each argument of tool is declared, by
// DeclareMCPPathArgs or else by the schema's annotations, with `project` at
// the top level a location unless declared otherwise. An argument declared
// neither way is absent from the map. It also returns every argument that can
// carry a string.
func mcpArgKinds(tool string, schema mcpInputSchema) (map[string]MCPArgKind, []mcpStringArg, error) {
	out := declaredMCPPathArgs(tool)
	args, err := mcpStringArgs(schema)
	if err != nil {
		return out, nil, err
	}
	for _, a := range args {
		if _, ok := out[a.Path]; !ok && a.Kind != 0 {
			out[a.Path] = a.Kind
		}
	}
	if _, ok := schema.Properties["project"]; ok {
		if _, declared := out["project"]; !declared {
			out["project"] = MCPArgPath
		}
	}
	return out, args, nil
}

// mcpArgKindAt returns how the argument at path is declared in kinds: by its
// own path, else by the nearest enclosing one.
func mcpArgKindAt(kinds map[string]MCPArgKind, path string) (MCPArgKind, bool) {
	for p := path; p != ""; p = parentMCPArgPath(p) {
		if k, ok := kinds[p]; ok {
			return k, true
		}
	}
	return 0, false
}

// parentMCPArgPath returns the path of the argument that encloses path, ""
// for a top-level one.
func parentMCPArgPath(path string) string {
	parent, _, _ := strings.CutLast(path, ".")
	return parent
}

// checkMCPCallArgs refuses a call that passes an argument its tool's schema
// does not list, or a declared location the call may not reach.
func checkMCPCallArgs(ctx context.Context, tool string, schema mcpInputSchema, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return fmt.Errorf("decode arguments: %w", err)
	}
	var unknown []string
	for name := range args {
		if _, ok := schema.Properties[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("%s takes no argument %s", tool, strings.Join(unknown, ", "))
	}
	kinds, _, err := mcpArgKinds(tool, schema)
	if err != nil {
		return fmt.Errorf("%s: read its input schema: %w", tool, err)
	}
	paths := make([]string, 0, len(kinds))
	for p := range kinds {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		kind := kinds[p]
		if kind == MCPArgNotLocation {
			continue
		}
		// A declared location a tool takes inside a structure is checked like
		// one passed on its own: apply_edits reads its documents from
		// ops.at.doc, and review_block from at.doc.
		for _, v := range valuesAt(args, strings.Split(p, ".")) {
			if err := checkMCPLocation(ctx, p, kind, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// valuesAt returns the strings held at path in v, a path spelled as the top
// of this file describes: arrays are transparent, and `*` is every value of a
// map. Every string beneath the place path reaches is returned, so a location
// declared on an array of strings, or on an open object, covers each string
// in it.
func valuesAt(v any, path []string) []string {
	if arr, ok := v.([]any); ok {
		var out []string
		for _, e := range arr {
			out = append(out, valuesAt(e, path)...)
		}
		return out
	}
	if len(path) == 0 {
		return stringsIn(v)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if path[0] == "*" {
		var out []string
		for _, e := range m {
			out = append(out, valuesAt(e, path[1:])...)
		}
		return out
	}
	e, ok := m[path[0]]
	if !ok {
		return nil
	}
	return valuesAt(e, path[1:])
}

// stringsIn returns every non-empty string in v, at any depth.
func stringsIn(v any) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []any:
		var out []string
		for _, e := range t {
			out = append(out, stringsIn(e)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, e := range t {
			out = append(out, stringsIn(e)...)
		}
		return out
	}
	return nil
}

// checkMCPResourceURI checks the location a resources/read names: the path of
// a context:// address, and its project parameter.
func checkMCPResourceURI(ctx context.Context, uri string) error {
	if !strings.HasPrefix(uri, contextURIScheme) {
		return nil
	}
	rest := strings.TrimPrefix(uri, contextURIScheme)
	path, query, _ := strings.Cut(rest, "?")
	if err := checkMCPLocation(ctx, "uri", MCPArgProjectRelative, path); err != nil {
		return err
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return fmt.Errorf("uri: %w", err)
	}
	if p := values.Get("project"); p != "" {
		return MCPReachable(ctx, "project", p)
	}
	return nil
}

// checkMCPLocation checks one declared argument value.
func checkMCPLocation(ctx context.Context, arg string, kind MCPArgKind, v string) error {
	v = strings.TrimPrefix(v, contextURIScheme)
	if kind == MCPArgProjectRelative && !filepath.IsAbs(v) {
		if !filepath.IsLocal(filepath.FromSlash(v)) {
			return fmt.Errorf("%s: %q leaves the project. Name a path inside it", arg, v)
		}
		return nil
	}
	return MCPReachable(ctx, arg, v)
}
