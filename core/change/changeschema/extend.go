package changeschema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Side says which side of a schema a change must not break: the side that
// writes values the schema describes, or the side that reads them.
type Side int

const (
	// Writer is the side of a request: a change set or a tool's arguments,
	// which a sender writes and the service decodes. A property newly
	// required breaks a sender that omits it.
	Writer Side = iota
	// Reader is the side of a reply: a result or a read, which the service
	// writes and a caller reads. A property no longer required breaks a
	// reader that relies on it.
	Reader
)

// Extends reports how current fails to extend frozen, one line per
// difference, and nothing when it extends it. A schema extends another when
// every value the frozen one describes keeps its meaning: a property, a $defs
// entry or a oneOf member it holds is still there, under the same name; its
// type, const, $ref, pattern, format and bounds are unchanged; no enum value
// is gone; additionalProperties is unchanged; and the required properties
// change only in the direction side allows. New optional properties, new
// oneOf members, new enum values, new $defs entries and any change to a
// description, a title or a $comment extend a schema.
//
// Patterns are compared exactly, so a widened pattern is a difference too:
// whether one pattern accepts everything another does is past what the walk
// can judge, and a contract change that alters one is reviewed.
//
// oneOf and anyOf members are matched by their op const, else by their
// required properties, else by their type, else by the keywords they hold.
func Extends(frozen, current []byte, side Side) ([]string, error) {
	var f, c any
	if err := json.Unmarshal(frozen, &f); err != nil {
		return nil, fmt.Errorf("frozen schema: %w", err)
	}
	if err := json.Unmarshal(current, &c); err != nil {
		return nil, fmt.Errorf("current schema: %w", err)
	}
	w := extendWalk{side: side}
	w.schema("", f, c)
	return w.problems, nil
}

type extendWalk struct {
	side     Side
	problems []string
}

func (w *extendWalk) fail(at, format string, args ...any) {
	if at == "" {
		at = "(root)"
	}
	w.problems = append(w.problems, at+": "+fmt.Sprintf(format, args...))
}

// exactKeywords must hold the same value, or be absent from both: each
// narrows or redirects what a value may be.
var exactKeywords = []string{
	"type", "const", "$ref", "pattern", "format", "not",
	"minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
	"minItems", "maxItems", "minLength", "maxLength", "minProperties", "maxProperties",
	"uniqueItems", "multipleOf",
}

func (w *extendWalk) schema(at string, frozen, current any) {
	f, fok := frozen.(map[string]any)
	c, cok := current.(map[string]any)
	if !fok || !cok {
		// A boolean schema (true or false) holds its meaning only by
		// staying the same.
		if !reflect.DeepEqual(frozen, current) {
			w.fail(at, "the schema changed from %s to %s", compact(frozen), compact(current))
		}
		return
	}
	for _, k := range exactKeywords {
		fv, fh := f[k]
		cv, ch := c[k]
		switch {
		case fh && !ch:
			w.fail(at, "%s %s was removed", k, compact(fv))
		case !fh && ch:
			w.fail(at, "%s %s was added", k, compact(cv))
		case fh && !sameValue(k, fv, cv):
			w.fail(at, "%s changed from %s to %s", k, compact(fv), compact(cv))
		}
	}
	w.enum(at, f["enum"], c["enum"])
	w.required(at, f["required"], c["required"])
	w.additional(at, f["additionalProperties"], c["additionalProperties"])
	w.named(at, "properties", f["properties"], c["properties"])
	w.named(at, "$defs", f["$defs"], c["$defs"])
	w.named(at, "patternProperties", f["patternProperties"], c["patternProperties"])
	for _, k := range []string{"items", "propertyNames", "contains", "if", "then", "else"} {
		fv, fh := f[k]
		cv, ch := c[k]
		switch {
		case fh && ch:
			w.schema(join(at, k), fv, cv)
		case fh != ch:
			w.fail(at, "%s was %s", k, presence(fh))
		}
	}
	for _, k := range []string{"oneOf", "anyOf", "allOf", "prefixItems"} {
		w.members(at, k, f[k], c[k])
	}
}

func (w *extendWalk) enum(at string, frozen, current any) {
	fs, fok := frozen.([]any)
	if !fok {
		if current != nil {
			w.fail(at, "an enum was added")
		}
		return
	}
	cs, _ := current.([]any)
	for _, v := range fs {
		if !slices.ContainsFunc(cs, func(x any) bool { return reflect.DeepEqual(x, v) }) {
			w.fail(at, "enum value %s was removed", compact(v))
		}
	}
}

func (w *extendWalk) required(at string, frozen, current any) {
	fr, cr := stringSet(frozen), stringSet(current)
	switch w.side {
	case Writer:
		for _, name := range sortedKeys(cr) {
			if !fr[name] {
				w.fail(at, "property %q became required: a sender that omits it is refused", name)
			}
		}
	case Reader:
		for _, name := range sortedKeys(fr) {
			if !cr[name] {
				w.fail(at, "property %q is no longer required: a reader that relies on it finds it missing", name)
			}
		}
	}
}

// additional compares additionalProperties. A closed object (false, or
// {"not": {}}) stays closed and an open one stays open; where it is a schema
// for the values of a map, the walk goes into it.
func (w *extendWalk) additional(at string, frozen, current any) {
	fClosed, cClosed := closed(frozen), closed(current)
	switch {
	case frozen == nil && current == nil:
	case fClosed || cClosed:
		if fClosed != cClosed {
			w.fail(at, "additionalProperties changed from %s to %s", compact(frozen), compact(current))
		}
	case frozen == nil || current == nil:
		w.fail(at, "additionalProperties changed from %s to %s", compact(frozen), compact(current))
	default:
		w.schema(join(at, "additionalProperties"), frozen, current)
	}
}

func closed(v any) bool {
	if b, ok := v.(bool); ok {
		return !b
	}
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	not, ok := m["not"].(map[string]any)
	return ok && len(m) == 1 && len(not) == 0
}

// named walks a keyword that maps names to schemas: properties, $defs. Every
// name the frozen schema holds must still be there.
func (w *extendWalk) named(at, keyword string, frozen, current any) {
	fm, _ := frozen.(map[string]any)
	cm, _ := current.(map[string]any)
	for _, name := range sortedKeys(fm) {
		cv, ok := cm[name]
		if !ok {
			what := "property"
			if keyword == "$defs" {
				what = "$defs entry"
			}
			w.fail(at, "%s %q was removed (a rename is a removal)", what, name)
			continue
		}
		w.schema(join(at, keyword, name), fm[name], cv)
	}
}

// members walks a list of subschemas. A member the frozen list holds must
// still be there, found by its key; new members extend the list.
func (w *extendWalk) members(at, keyword string, frozen, current any) {
	fl, _ := frozen.([]any)
	cl, _ := current.([]any)
	if keyword == "allOf" || keyword == "prefixItems" {
		if len(fl) != len(cl) {
			w.fail(at, "%s changed from %d to %d members", keyword, len(fl), len(cl))
			return
		}
		for i := range fl {
			w.schema(join(at, fmt.Sprintf("%s[%d]", keyword, i)), fl[i], cl[i])
		}
		return
	}
	if len(fl) == 0 && len(cl) > 0 {
		w.fail(at, "%s was added", keyword)
		return
	}
	byKey := map[string]any{}
	for _, m := range cl {
		byKey[memberKey(m)] = m
	}
	for _, m := range fl {
		key := memberKey(m)
		cm, ok := byKey[key]
		if !ok {
			w.fail(at, "%s member %s was removed", keyword, key)
			continue
		}
		w.schema(join(at, keyword+"["+key+"]"), m, cm)
	}
}

// memberKey names a oneOf or anyOf member: by its op const, else its required
// properties, else its type, else the keywords it holds.
func memberKey(member any) string {
	m, ok := member.(map[string]any)
	if !ok {
		return compact(member)
	}
	if props, ok := m["properties"].(map[string]any); ok {
		if op, ok := props["op"].(map[string]any); ok {
			if c, ok := op["const"].(string); ok {
				return "op=" + c
			}
		}
	}
	if req := stringSet(m["required"]); len(req) > 0 {
		return "required=" + strings.Join(sortedKeys(req), ",")
	}
	if t, ok := m["type"]; ok {
		return "type=" + compact(t)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "description" && k != "title" && k != "$comment" {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return "keywords=" + strings.Join(keys, ",")
}

// sameValue compares a keyword's values; a type list compares as a set.
func sameValue(keyword string, a, b any) bool {
	if keyword == "type" {
		as, aok := a.([]any)
		bs, bok := b.([]any)
		if aok && bok {
			return reflect.DeepEqual(stringSet(as), stringSet(bs))
		}
	}
	return reflect.DeepEqual(a, b)
}

func stringSet(v any) map[string]bool {
	out := map[string]bool{}
	list, _ := v.([]any)
	for _, x := range list {
		if s, ok := x.(string); ok {
			out[s] = true
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func join(at string, parts ...string) string {
	for _, p := range parts {
		if at == "" {
			at = p
		} else {
			at += "." + p
		}
	}
	return at
}

func presence(present bool) string {
	if present {
		return "removed"
	}
	return "added"
}

func compact(v any) string {
	if v == nil {
		return "absent"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	const limit = 80
	if len(b) > limit {
		return string(b[:limit]) + "…"
	}
	return string(b)
}
