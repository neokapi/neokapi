// json.go — the canonical JSON form of the content model.
//
// THE canonical JSON for the content model (AD-034) is protojson of the
// neokapi.content.v1 messages, with the fixed options below. Every JSON
// surface that claims to carry the model — REST payloads, polyglot APIs,
// generated TypeScript types, the checked-in JSON Schema — is defined by this
// encoding; everything else is an explicitly-labeled projection.
//
// Fixed encoding rules (protojson defaults, pinned here so they cannot drift):
//   - Field names use the lowerCamelCase protojson JSON names derived from the
//     proto field names (pc_open → "pcOpen"). Proto-name spellings are NOT
//     emitted, but both spellings are accepted on unmarshal (protojson always
//     accepts either).
//   - Unpopulated fields are omitted (proto3 zero values do not appear).
//   - bytes fields encode as base64 strings; int32 as JSON numbers.
//   - Unknown fields are ignored on unmarshal, so an older consumer can read a
//     newer producer's output (forward compatibility across appended fields).
//
// Marshal output is byte-deterministic: protojson deliberately randomizes its
// whitespace, so both marshal helpers re-serialize through encoding/json
// (which preserves protojson's stable field order — field-number order for
// messages, sorted keys for maps). The golden files under testdata/ lock the
// encoding; json_test.go fails on any incompatible change.
package contentv1

import (
	"bytes"
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// jsonMarshalOptions is the single, fixed protojson configuration for the
// canonical JSON. Defaults are deliberate: lowerCamelCase JSON names
// (UseProtoNames=false) and omitted zero values (EmitUnpopulated=false).
var jsonMarshalOptions = protojson.MarshalOptions{}

// jsonUnmarshalOptions is the fixed decode configuration. DiscardUnknown lets
// an older binary read JSON produced by a newer schema revision (appended
// fields are ignored, never an error).
var jsonUnmarshalOptions = protojson.UnmarshalOptions{DiscardUnknown: true}

// MarshalJSON renders a canonical content-model message as compact canonical
// JSON. The output is byte-deterministic for a given message value.
func MarshalJSON(m proto.Message) ([]byte, error) {
	b, err := jsonMarshalOptions.Marshal(m)
	if err != nil {
		return nil, err
	}
	// protojson output is deliberately unstable (random whitespace);
	// json.Compact strips it without touching the token stream.
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalJSONIndent renders a canonical content-model message as indented
// (two-space) canonical JSON — the form used for the checked-in golden files.
// Deterministic, same encoding as MarshalJSON.
func MarshalJSONIndent(m proto.Message) ([]byte, error) {
	b, err := jsonMarshalOptions.Marshal(m)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalJSON decodes canonical JSON into a content-model message. Both
// lowerCamelCase JSON names and original proto field names are accepted;
// unknown fields are ignored (forward compatibility).
func UnmarshalJSON(data []byte, m proto.Message) error {
	return jsonUnmarshalOptions.Unmarshal(data, m)
}
