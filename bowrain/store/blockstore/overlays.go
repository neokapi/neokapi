package blockstore

import (
	"encoding/json"
	"strings"
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

// translationPayload captures the schema translation writers use
// (`{"text":"…","provider":"…"}`) so the translations table keeps
// text + provider in first-class columns instead of opaque JSON.
// Callers that write richer payloads get the metadata column for
// future fields without another migration.
type translationPayload struct {
	Text     string          `json:"text"`
	Provider string          `json:"provider,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// encodeTranslationPayload reconstructs an overlay's payload from
// the first-class columns the translations table stores separately.
// Preserves the round-trip contract: callers that put `{text, provider,
// metadata}` get the same JSON back out; callers that put an arbitrary
// JSON body (e.g. a runs-shaped target from a rich editor) landed it
// in the `metadata` column on write and we emit that verbatim here.
func encodeTranslationPayload(text, provider string, metadata []byte) ([]byte, error) {
	hasMetadata := hasJSONBody(metadata)
	opaqueOnly := text == "" && provider == "" && hasMetadata
	if opaqueOnly {
		// Payload was opaque JSON without a text field — return
		// the original body so the caller gets byte-identical reads.
		return metadata, nil
	}
	p := translationPayload{Text: text, Provider: provider}
	if hasMetadata {
		p.Metadata = metadata
	}
	return json.Marshal(p)
}

// hasJSONBody reports whether the bytes hold JSON content more
// meaningful than an empty object or null.
func hasJSONBody(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	s := string(b)
	return s != "{}" && s != "null"
}

// decodeTranslationPayload splits the incoming payload into the three
// columns the translations table stores separately:
//
//   - `{"text": "…", "provider": "…", "metadata": …}` → text, provider,
//     metadata populated from their fields.
//   - any other JSON object (e.g. runs-shaped targets, legacy payloads
//     a rich editor might push) → text+provider empty, metadata holds
//     the original body so reads round-trip exactly.
//   - non-JSON payloads → treated as a raw text string.
func decodeTranslationPayload(payload []byte) (text, provider string, metadata []byte) {
	var p translationPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return string(payload), "", nil
	}
	if p.Text == "" && p.Provider == "" {
		// Payload didn't fit the `{text, provider}` shape — preserve
		// the original body verbatim so GetOverlay echoes byte-for-byte.
		return "", "", payload
	}
	return p.Text, p.Provider, []byte(p.Metadata)
}
