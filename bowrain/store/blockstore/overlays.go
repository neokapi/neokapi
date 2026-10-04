package blockstore

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// overlayTable is the destination table for a given overlay kind.
// The blockstore.Store interface is polymorphic in kind; under it
// the adapter dispatches to purpose-built tables so each access
// pattern gets the right indexes. See #403.
type overlayTable int

const (
	tableOverlaysExt overlayTable = iota // plugin catchall
	tableTranslations
	tableAnnotations
)

// routeKind decides which table a kind lives in. Kinds are strings
// with a namespace prefix followed by `/` + sub-key:
//
//	"targets/<locale>"      → translations  (locale = sub-key)
//	"annotations/<name>"    → annotations   (name  = sub-key of kind)
//	anything else           → overlays_ext  (catchall, opaque kind)
func routeKind(kind string) overlayTable {
	prefix, _ := splitKindOnce(kind)
	switch prefix {
	case "targets":
		return tableTranslations
	case "annotations":
		return tableAnnotations
	default:
		return tableOverlaysExt
	}
}

// splitKindOnce splits on the first `/`. Returns (prefix, rest);
// rest is empty if there's no slash.
func splitKindOnce(kind string) (prefix, rest string) {
	before, after, ok := strings.Cut(kind, "/")
	if !ok {
		return kind, ""
	}
	return before, after
}

// An `annotations/<name>` overlay is a block annotation, not a private layer:
// <name> is the key Block.SetAnno files it under, and the payload is that
// annotation's body. The adapter therefore hands the store the same three
// coordinates the block round-trip does — the block's row id, the bare key, and
// a typed payload — so a check tool's findings written through a flow reach the
// editor that reads the block, and a note the editor writes reaches the flow.

// annotationKey returns the annotation key an `annotations/<name>` kind names.
// A kind with no name addresses no annotation and is refused rather than filed
// under the empty key, where nothing would ever look for it.
func annotationKey(kind string) (string, error) {
	_, key := splitKindOnce(kind)
	if key == "" {
		return "", fmt.Errorf("bowrain/blockstore: kind %q names no annotation", kind)
	}
	return key, nil
}

// encodeAnnotationPayload renders a stored annotation back as an overlay
// payload. A payload whose type this process knows comes back through that
// type's own JSON; one it does not comes back as the bytes its writer wrote.
func encodeAnnotationPayload(ann model.Payload) ([]byte, error) {
	if ann == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(ann)
}

// A `targets/<locale>` overlay payload is not opaque: it is a projection of
// the edition the translations row stores, and the store reads it as one. The
// translate-family tools spell the content three ways, as `runs` (structure
// preserved), `text` or `target`, and carry the lifecycle status alongside it;
// the codec below is the one place that knows which fields mean the edition
// and which belong to the writer.
//
// Anything an edition has no room for (a tool's config fingerprint, the
// provider label an MT cache checks) is residue: it survives verbatim in the
// row's metadata column and comes back on the same read, so a tool's own
// round-trip is unaffected while every platform reader sees the target itself.
const (
	fieldRuns   = "runs"
	fieldText   = "text"
	fieldTarget = "target"
	fieldStatus = "status"
	fieldOrigin = "origin"
	fieldScore  = "score"
)

// decodeTargetPayload reads an overlay payload as the edition it describes,
// returning the edition and the writer's residual fields.
//
// A payload that is not JSON at all is the edition's plain text, the shape a
// caller writing a bare string produces. JSON null is an empty edition, not the
// four letters.
func decodeTargetPayload(payload []byte) (model.Edition, []byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return model.Edition{Runs: []model.Run{model.TextR(string(payload))}}, nil, nil
	}
	if fields == nil {
		return model.Edition{}, nil, nil
	}

	var e model.Edition
	if raw, ok := fields[fieldRuns]; ok {
		if err := json.Unmarshal(raw, &e.Runs); err != nil {
			return model.Edition{}, nil, fmt.Errorf("decode target runs: %w", err)
		}
	}
	if len(e.Runs) == 0 {
		for _, key := range []string{fieldText, fieldTarget} {
			var text string
			if raw, ok := fields[key]; ok && json.Unmarshal(raw, &text) == nil && text != "" {
				e.Runs = []model.Run{model.TextR(text)}
				break
			}
		}
	}
	if raw, ok := fields[fieldStatus]; ok {
		if err := json.Unmarshal(raw, &e.Status); err != nil {
			return model.Edition{}, nil, fmt.Errorf("decode target status: %w", err)
		}
	}
	if raw, ok := fields[fieldOrigin]; ok {
		if err := json.Unmarshal(raw, &e.Origin); err != nil {
			return model.Edition{}, nil, fmt.Errorf("decode target origin: %w", err)
		}
	}
	if raw, ok := fields[fieldScore]; ok {
		if err := json.Unmarshal(raw, &e.Score); err != nil {
			return model.Edition{}, nil, fmt.Errorf("decode target score: %w", err)
		}
	}

	residue := map[string]json.RawMessage{}
	for k, v := range fields {
		switch k {
		case fieldRuns, fieldText, fieldTarget, fieldStatus, fieldOrigin, fieldScore:
			continue
		}
		residue[k] = v
	}
	if len(residue) == 0 {
		return e, nil, nil
	}
	extra, err := json.Marshal(residue)
	if err != nil {
		return model.Edition{}, nil, fmt.Errorf("encode target payload residue: %w", err)
	}
	return e, extra, nil
}

// encodeTargetPayload renders a stored edition back as an overlay payload,
// carrying both the runs and their flattened text so a reader of either shape
// finds what it asks for. The residue rides alongside, under the keys its
// writer used.
func encodeTargetPayload(e model.Edition, extra []byte) ([]byte, error) {
	fields := map[string]json.RawMessage{}
	if len(extra) > 0 {
		if err := json.Unmarshal(extra, &fields); err != nil {
			return nil, fmt.Errorf("decode target payload residue: %w", err)
		}
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
	}
	set := func(key string, v any) error {
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encode target %s: %w", key, err)
		}
		fields[key] = raw
		return nil
	}
	if len(e.Runs) > 0 {
		if err := set(fieldRuns, e.Runs); err != nil {
			return nil, err
		}
		if err := set(fieldText, model.RunsText(e.Runs)); err != nil {
			return nil, err
		}
	}
	if e.Status != "" {
		if err := set(fieldStatus, e.Status); err != nil {
			return nil, err
		}
	}
	if e.Origin != (model.Origin{}) {
		if err := set(fieldOrigin, e.Origin); err != nil {
			return nil, err
		}
	}
	if e.Score != 0 {
		if err := set(fieldScore, e.Score); err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}
