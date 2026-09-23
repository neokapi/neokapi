package profile

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// UnknownKeys reports each key in a voice profile document that the profile
// model does not define. Each problem is a warning with CodeUnknownKey, and its
// Field is the key's dotted path, such as "channels.docs.vocab" or
// "style.prohibited_patterns[0].sevrity".
//
// LoadProfileYAML ignores such a key without a message, so a profile can read as
// governed while part of it applies nothing. DecodeProfileStrict refuses the
// same document, and it decides what is unknown: a document it accepts has no
// unknown keys. Walking the document against the model only places each key the
// decoder refused. A refusal the walk cannot place is reported in the decoder's
// own words, so nothing the decoder refuses goes unreported.
//
// data is a document LoadProfileYAML accepts; one that does not parse returns
// the decode error.
func UnknownKeys(data []byte) ([]ProfileProblem, error) {
	_, strictErr := DecodeProfileStrict(bytes.NewReader(data))
	if strictErr == nil {
		return nil, nil
	}
	if _, ok := errors.AsType[*yaml.TypeError](strictErr); !ok {
		return nil, strictErr
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var found []ProfileProblem
	walkKeys(&doc, reflect.TypeFor[VoiceProfile](), "", &found)
	if len(found) > 0 {
		return found, nil
	}
	for _, p := range StrictDecodeProblems(strictErr) {
		if p.Field == "" {
			continue
		}
		p.Code, p.Warning = CodeUnknownKey, true
		found = append(found, p)
	}
	return found, nil
}

var yamlUnmarshaler = reflect.TypeFor[yaml.Unmarshaler]()

// walkKeys follows node through the type yaml.v3 decodes it into, and records
// each mapping key that type has no field for. A type with its own
// UnmarshalYAML decides its keys itself, so the walk stops there.
func walkKeys(node *yaml.Node, t reflect.Type, path string, found *[]ProfileProblem) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, c := range node.Content {
			walkKeys(c, t, path, found)
		}
		return
	case yaml.AliasNode:
		if node.Alias != nil {
			walkKeys(node.Alias, t, path, found)
		}
		return
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if reflect.PointerTo(t).Implements(yamlUnmarshaler) {
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return
		}
		fields, open := yamlFields(t)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Kind == yaml.ScalarNode && key.ShortTag() == "!!merge" {
				// A merge key lends its mappings' keys to this mapping.
				merged := []*yaml.Node{value}
				if value.Kind == yaml.SequenceNode {
					merged = value.Content
				}
				for _, m := range merged {
					walkKeys(m, t, path, found)
				}
				continue
			}
			field, ok := fields[key.Value]
			switch {
			case ok:
				walkKeys(value, field, joinKey(path, key.Value), found)
			case !open && key.Value == "severity" && fieldsHas(fields, "advisory"):
				// A rule fails unless it is advisory, so a severity reads as a
				// setting and decides nothing. Name the key that does.
				*found = append(*found, ProfileProblem{
					Code:  CodeUnknownKey,
					Field: joinKey(path, key.Value),
					Message: fmt.Sprintf("key \"severity\" (line %d) is ignored: a rule fails a check unless it says "+
						"`advisory: true`, which makes it report only", key.Line),
					Warning: true,
				})
			case !open:
				*found = append(*found, ProfileProblem{
					Code:  CodeUnknownKey,
					Field: joinKey(path, key.Value),
					Message: fmt.Sprintf("unknown key %q (line %d) is ignored when the profile loads; "+
						"check its spelling and the section it sits under", key.Value, key.Line),
					Warning: true,
				})
			}
		}
	case reflect.Map:
		if node.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			walkKeys(node.Content[i+1], t.Elem(), joinKey(path, node.Content[i].Value), found)
		}
	case reflect.Slice, reflect.Array:
		if node.Kind != yaml.SequenceNode {
			return
		}
		for i, item := range node.Content {
			walkKeys(item, t.Elem(), path+"["+strconv.Itoa(i)+"]", found)
		}
	}
}

// yamlFields returns the keys yaml.v3 decodes into t's fields, each with the
// field's type, and whether an inline map accepts every other key as well. The
// naming follows yaml.v3: the tag's name, else the lowercased field name.
func yamlFields(t reflect.Type) (map[string]reflect.Type, bool) {
	fields := map[string]reflect.Type{}
	open := false
	for f := range t.Fields() {
		if f.PkgPath != "" && !f.Anonymous {
			continue
		}
		tag := f.Tag.Get("yaml")
		if tag == "" && !strings.Contains(string(f.Tag), ":") {
			tag = string(f.Tag)
		}
		if tag == "-" {
			continue
		}
		name, flags, _ := strings.Cut(tag, ",")
		if slices.Contains(strings.Split(flags, ","), "inline") {
			inner := f.Type
			for inner.Kind() == reflect.Pointer {
				inner = inner.Elem()
			}
			switch inner.Kind() {
			case reflect.Map:
				open = true
			case reflect.Struct:
				innerFields, innerOpen := yamlFields(inner)
				maps.Copy(fields, innerFields)
				open = open || innerOpen
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		fields[name] = f.Type
	}
	return fields, open
}

func joinKey(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// unknownFieldLine extracts the line number and field name from a yaml.v3
// KnownFields(true) "field X not found in type ..." error line.
var unknownFieldLine = regexp.MustCompile(`line (\d+): field (\S+) not found in type`)

// StrictDecodeProblems turns a strict-decode error (from DecodeProfileStrict)
// into per-field problems. It recognises yaml.v3's unknown-field lines and
// rewrites them without the leaking Go type name; any unrecognised remainder is
// surfaced verbatim so no decode error is swallowed.
func StrictDecodeProblems(err error) []ProfileProblem {
	if err == nil {
		return nil
	}
	var probs []ProfileProblem
	for line := range strings.SplitSeq(err.Error(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "yaml: unmarshal errors:") {
			continue
		}
		if m := unknownFieldLine.FindStringSubmatch(line); m != nil {
			probs = append(probs, ProfileProblem{
				Field:   m[2],
				Message: fmt.Sprintf("unknown field %q (line %s)", m[2], m[1]),
			})
			continue
		}
		probs = append(probs, ProfileProblem{Message: line})
	}
	return probs
}

// fieldsHas reports whether the decoded type has a field for key.
func fieldsHas(fields map[string]reflect.Type, key string) bool {
	_, ok := fields[key]
	return ok
}
