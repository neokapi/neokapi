package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	coreg "github.com/neokapi/neokapi/core/graph"
)

// agtypeVertex is the JSON structure inside an AGE vertex agtype value.
type agtypeVertex struct {
	ID         int64          `json:"id"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
}

// agtypeEdge is the JSON structure inside an AGE edge agtype value.
type agtypeEdge struct {
	ID         int64          `json:"id"`
	Label      string         `json:"label"`
	StartID    int64          `json:"start_id"`
	EndID      int64          `json:"end_id"`
	Properties map[string]any `json:"properties"`
}

// ParseVertex parses an agtype vertex string into a Node.
// AGE vertex format: {"id": 123, "label": "Concept", "properties": {...}}::vertex
func ParseVertex(raw string) (*coreg.Node, error) {
	body, err := stripSuffix(raw, "::vertex")
	if err != nil {
		return nil, fmt.Errorf("parse vertex: %w", err)
	}

	var v agtypeVertex
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return nil, fmt.Errorf("parse vertex JSON: %w", err)
	}

	props := toStringMap(v.Properties)
	nodeID := props["id"]
	if nodeID == "" {
		nodeID = strconv.FormatInt(v.ID, 10)
	}

	return &coreg.Node{
		ID:         nodeID,
		Label:      v.Label,
		Properties: props,
	}, nil
}

// ParseEdge parses an agtype edge string into an Edge.
// AGE edge format: {"id": 123, "label": "BROADER", "end_id": 456, "start_id": 789, "properties": {...}}::edge
func ParseEdge(raw string) (*coreg.Edge, error) {
	body, err := stripSuffix(raw, "::edge")
	if err != nil {
		return nil, fmt.Errorf("parse edge: %w", err)
	}

	var e agtypeEdge
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		return nil, fmt.Errorf("parse edge JSON: %w", err)
	}

	props := toStringMap(e.Properties)
	edgeID := props["id"]
	if edgeID == "" {
		edgeID = strconv.FormatInt(e.ID, 10)
	}
	source := props["source"]
	if source == "" {
		source = strconv.FormatInt(e.StartID, 10)
	}
	target := props["target"]
	if target == "" {
		target = strconv.FormatInt(e.EndID, 10)
	}

	edge := &coreg.Edge{
		ID:         edgeID,
		Source:     source,
		Target:     target,
		Label:      e.Label,
		Properties: props,
	}

	edge.Validity = parseValidity(props)

	return edge, nil
}

// ParsePath parses an agtype path into a Path.
// AGE path format: [vertex, edge, vertex, ...]::path
func ParsePath(raw string) (*coreg.Path, error) {
	body, err := stripSuffix(raw, "::path")
	if err != nil {
		return nil, fmt.Errorf("parse path: %w", err)
	}

	// The path body is a JSON array of alternating vertex and edge elements,
	// each with their own ::vertex / ::edge suffixes inside the array.
	// We need to split them carefully.
	elements, err := splitPathElements(body)
	if err != nil {
		return nil, fmt.Errorf("parse path elements: %w", err)
	}

	var path coreg.Path
	for i, elem := range elements {
		elem = strings.TrimSpace(elem)
		if i%2 == 0 {
			// Even indices are vertices.
			node, err := ParseVertex(elem)
			if err != nil {
				return nil, fmt.Errorf("parse path vertex %d: %w", i/2, err)
			}
			path.Nodes = append(path.Nodes, *node)
		} else {
			// Odd indices are edges.
			edge, err := ParseEdge(elem)
			if err != nil {
				return nil, fmt.Errorf("parse path edge %d: %w", i/2, err)
			}
			path.Edges = append(path.Edges, *edge)
		}
	}

	return &path, nil
}

// stripSuffix removes a type suffix (e.g., "::vertex") from an agtype value.
func stripSuffix(raw, suffix string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasSuffix(raw, suffix) {
		return "", fmt.Errorf("expected %s suffix, got: %s", suffix, truncate(raw, 80))
	}
	return strings.TrimSpace(raw[:len(raw)-len(suffix)]), nil
}

// toStringMap converts a map[string]any to map[string]string.
func toStringMap(m map[string]any) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			result[k] = val
		case float64:
			if val == float64(int64(val)) {
				result[k] = strconv.FormatInt(int64(val), 10)
			} else {
				result[k] = strconv.FormatFloat(val, 'f', -1, 64)
			}
		case bool:
			result[k] = strconv.FormatBool(val)
		case nil:
			// Skip nil values.
		default:
			b, _ := json.Marshal(val)
			result[k] = string(b)
		}
	}
	return result
}

// parseValidity extracts Validity from edge properties if present.
func parseValidity(props map[string]string) *coreg.Validity {
	vf := props["valid_from"]
	vt := props["valid_to"]
	tagsJSON := props["tags"]

	if vf == "" && vt == "" && tagsJSON == "" {
		return nil
	}

	v := &coreg.Validity{}
	if vf != "" {
		if t, err := parseTime(vf); err == nil {
			v.ValidFrom = &t
		}
	}
	if vt != "" {
		if t, err := parseTime(vt); err == nil {
			v.ValidTo = &t
		}
	}
	if tagsJSON != "" {
		var tags map[string]string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err == nil {
			v.Tags = tags
		}
	}
	return v
}

// splitPathElements splits a path body "[v, e, v, ...]" into individual agtype elements.
func splitPathElements(body string) ([]string, error) {
	body = strings.TrimSpace(body)
	if len(body) < 2 || body[0] != '[' || body[len(body)-1] != ']' {
		return nil, errors.New("path must be enclosed in brackets")
	}
	body = body[1 : len(body)-1]

	var elements []string
	depth := 0
	start := 0
	for i := range len(body) {
		switch body[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				elements = append(elements, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	if start < len(body) {
		elements = append(elements, strings.TrimSpace(body[start:]))
	}
	return elements, nil
}

// truncate returns at most n characters of s for error messages.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
