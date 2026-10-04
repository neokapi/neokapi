package change

import (
	"bytes"
	"encoding/json"
)

// Mode says whether a change set writes.
type Mode string

const (
	// ModeApply applies the change set. It is the default.
	ModeApply Mode = "apply"
	// ModePreview computes, checks and renders the change set, writes nothing,
	// and returns the same result with every applied operation previewed.
	ModePreview Mode = "preview"
)

// Gate says what a failing governance finding the change introduces does.
type Gate string

const (
	// GateEnforce refuses the change set with gate_failed. It is the default.
	GateEnforce Gate = "enforce"
	// GateReport lands the change with its findings.
	GateReport Gate = "report"
)

// Set is a change set: ordered operations under one envelope. There is no
// actor field; the transport that carries a change set says who sent it.
type Set struct {
	// Schema is the contract version, SchemaID.
	Schema string `json:"schema,omitempty"`
	Mode   Mode   `json:"mode,omitempty"`
	Gate   Gate   `json:"gate,omitempty"`
	// RequireBasis refuses a write to a derived edition whose authoritative
	// edition has moved since its sender read it.
	RequireBasis bool `json:"require_basis,omitempty"`
	// Note is one line a person reads in history and review.
	Note string `json:"note,omitempty"`
	// Evidence is where the wording behind the change was seen.
	Evidence []Evidence `json:"evidence,omitempty"`
	// Ops apply in order: an operation sees the result of the operations
	// before it on the same edition, while every if_match is checked against
	// the content as it stood when the change set began, and every position
	// (an edit's start and end, a run range, a path's run index) names that
	// content too and is moved past what the operations before it changed.
	// A find matches the text as they left it. No operation is a change set
	// that changes nothing.
	Ops []Op `json:"ops"`
}

// Evidence is where the wording behind a change was seen.
type Evidence struct {
	// Path is a project-relative, slash-separated file.
	Path string `json:"path,omitempty" jsonschema:"a project-relative file"`
	// Block is the key of the block inside the file, as a read reports it in
	// ref.block.
	Block string `json:"block,omitempty" jsonschema:"the block key a read reports as ref.block"`
	// Quote is the text the wording was seen in.
	Quote string `json:"quote,omitempty" jsonschema:"the text the wording was seen in"`
	// URL is a web page the wording was seen on.
	URL string `json:"url,omitempty" jsonschema:"a web page the wording was seen on"`
}

// MarshalJSON writes the envelope with HTML escaping off, so markup in text
// reads as written.
func (s Set) MarshalJSON() ([]byte, error) {
	type wire Set
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(wire(s)); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
