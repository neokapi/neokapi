package change

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// Ref addresses one edition of one block in one document.
//
// Doc is a project-relative path, an archive member (container!entry), an item
// path inside a server stream, or a workspace document key. Block is the block
// key: the durable key reconciliation assigned (Block.Unit) where there is
// one, else the structural name the format reports. Edition is the edition
// key, canonical; the zero key is the document's own edition.
type Ref struct {
	Doc     string
	Block   string
	Edition model.EditionKey
}

// refWire is the JSON shape of a Ref.
type refWire struct {
	Doc     string `json:"doc" jsonschema:"the document: a project-relative path, container!entry for an archive member, or a workspace document key"`
	Block   string `json:"block" jsonschema:"the block key a read reports"`
	Edition string `json:"edition,omitempty" jsonschema:"the edition: a language tag with optional ;tone= and ;channel=, for example fr or en;channel=short; omitted is the document's own edition"`
}

// EditionText is the edition key in its text form ("fr", "en;channel=short"),
// empty for the document's own edition.
func (r Ref) EditionText() string {
	return keyText(r.Edition)
}

// String renders the reference for messages: doc#block@edition.
func (r Ref) String() string {
	s := r.Doc
	if r.Block != "" {
		s += "#" + r.Block
	}
	if e := r.EditionText(); e != "" {
		s += "@" + e
	}
	return s
}

// MarshalJSON writes {"doc", "block", "edition"}, leaving out an empty block
// and the document's own edition.
func (r Ref) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	w := struct {
		Doc     string `json:"doc"`
		Block   string `json:"block,omitempty"`
		Edition string `json:"edition,omitempty"`
	}{r.Doc, r.Block, r.EditionText()}
	if err := enc.Encode(w); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// UnmarshalJSON reads a reference strictly: an unknown field is an error, and
// the edition is parsed with model.ParseEditionKey and kept canonical.
func (r *Ref) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var w refWire
	if err := dec.Decode(&w); err != nil {
		return err
	}
	k, err := model.ParseEditionKey(w.Edition)
	if err != nil {
		return fmt.Errorf("edition: %w", err)
	}
	*r = Ref{Doc: w.Doc, Block: w.Block, Edition: k}
	return nil
}

// keyText is the text form of an edition key, empty for the zero key.
func keyText(k model.EditionKey) string {
	if k.IsZero() {
		return ""
	}
	b, _ := k.Canonical().MarshalText()
	return string(b)
}
