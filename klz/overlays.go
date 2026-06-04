package klz

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// OverlaySetVersion is the overlays.klfo schema version (MAJOR.MINOR, same
// forward-compatibility contract as the rest of the KLF family).
const OverlaySetVersion = "1.0"

// OverlaySetKind is the magic string on the overlay-set envelope.
const OverlaySetKind = "kapi-overlay-set"

// overlaySet is the on-disk envelope for the overlays.klfo member.
type overlaySet struct {
	SchemaVersion string       `json:"schemaVersion"`
	Kind          string       `json:"kind"`
	Overlays      []OverlayDoc `json:"overlays"`
}

// marshalOverlaySet serializes overlays deterministically: sorted by
// (kind, blockHash), 2-space indent, no HTML escaping, trailing newline —
// so the member bytes (and thus the package RootHash) are stable for a
// given workspace state. Payloads are re-marshaled through a canonical
// pass so semantically-identical JSON hashes identically regardless of how
// the producing tool spaced it.
func marshalOverlaySet(overlays []OverlayDoc) ([]byte, error) {
	sorted := make([]OverlayDoc, len(overlays))
	for i, o := range overlays {
		payload, err := canonicalJSON(o.Payload)
		if err != nil {
			return nil, fmt.Errorf("overlay %q/%q: %w", o.Kind, o.BlockHash, err)
		}
		sorted[i] = OverlayDoc{Source: o.Source, Kind: o.Kind, BlockHash: o.BlockHash, Payload: payload}
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Source != sorted[j].Source {
			return sorted[i].Source < sorted[j].Source
		}
		if sorted[i].Kind != sorted[j].Kind {
			return sorted[i].Kind < sorted[j].Kind
		}
		return sorted[i].BlockHash < sorted[j].BlockHash
	})

	set := overlaySet{SchemaVersion: OverlaySetVersion, Kind: OverlaySetKind, Overlays: sorted}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&set); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unmarshalOverlaySet parses an overlays.klfo member, validating the
// envelope with the reject-unknown-major / accept-unknown-minor contract.
func unmarshalOverlaySet(data []byte) ([]OverlayDoc, error) {
	var set overlaySet
	if err := json.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("decode overlay set: %w", err)
	}
	if set.Kind != OverlaySetKind {
		return nil, fmt.Errorf("unexpected kind %q (want %q)", set.Kind, OverlaySetKind)
	}
	major, ok := majorVersion(set.SchemaVersion)
	if !ok {
		return nil, fmt.Errorf("invalid schemaVersion %q", set.SchemaVersion)
	}
	if wantMajor, _ := majorVersion(OverlaySetVersion); major != wantMajor {
		return nil, fmt.Errorf("unsupported major schemaVersion %d (this build speaks %s)", major, OverlaySetVersion)
	}
	return set.Overlays, nil
}

// HistoryEvent is one line of the advisory history log: a human-readable
// record of something that happened to the workspace. It is purely
// advisory — nothing reads it back to decide progress (AD-025 §5).
type HistoryEvent struct {
	// Timestamp is an RFC3339 time the caller supplies (the klz package
	// takes no clock, keeping it deterministic for tests).
	Timestamp string `json:"ts,omitempty"`
	// Event is the verb: "open", "step", "finish", "no-op", …
	Event string `json:"event"`
	// Step is the tool/node the event concerns, when applicable.
	Step string `json:"step,omitempty"`
	// Note is free-form detail.
	Note string `json:"note,omitempty"`
	// Prev is the Hash of the previous event, linking the log into a
	// tamper-evident chain ("" for the genesis line).
	Prev string `json:"prev,omitempty"`
	// Hash is sha256 over (Prev, Timestamp, Event, Step, Note). Any edit,
	// reorder, insertion, or deletion of a past line breaks the chain at
	// that point — verifiable with VerifyHistory. The hash being advisory
	// (the log is excluded from the package RootHash) does not weaken it:
	// it makes the *log itself* tamper-evident, which is its whole job.
	Hash string `json:"hash,omitempty"`
}

// historyDigest computes an event's chain hash over its content + the prior
// hash. Excludes the Hash field itself. The field order is fixed so the
// digest is deterministic across builds.
func historyDigest(prev, ts, event, step, note string) string {
	h := sha256.New()
	for _, f := range []string{prev, ts, event, step, note} {
		// Length-prefix each field so no field-boundary ambiguity exists
		// (e.g. step="a",note="b" must not collide with step="",note="ab").
		fmt.Fprintf(h, "%d:%s", len(f), f)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// AppendHistory appends one event as a JSON line to an existing append-only
// history log and returns the new bytes (existing bytes are never
// rewritten). The event is linked into a tamper-evident hash chain: its
// Prev is set to the last line's Hash and its own Hash is computed. Errors
// are impossible for the fixed struct, so it never fails.
func AppendHistory(existing []byte, ev HistoryEvent) []byte {
	ev.Prev = lastHistoryHash(existing)
	ev.Hash = historyDigest(ev.Prev, ev.Timestamp, ev.Event, ev.Step, ev.Note)
	line, _ := json.Marshal(ev)
	out := make([]byte, 0, len(existing)+len(line)+1)
	out = append(out, existing...)
	out = append(out, line...)
	out = append(out, '\n')
	return out
}

// lastHistoryHash returns the Hash of the final event in a log, or "" when
// the log is empty/unparseable (a new chain starts).
func lastHistoryHash(existing []byte) string {
	trimmed := bytes.TrimRight(existing, "\n")
	if len(trimmed) == 0 {
		return ""
	}
	if i := bytes.LastIndexByte(trimmed, '\n'); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	var ev HistoryEvent
	if err := json.Unmarshal(trimmed, &ev); err != nil {
		return ""
	}
	return ev.Hash
}

// VerifyHistory walks a hash-chained history log and reports the first line
// whose chain is broken — a recomputed hash mismatch (the line was edited)
// or a Prev that doesn't link to the previous line's Hash (a line was
// inserted, removed, or reordered). Returns nil for an empty or fully
// intact log.
func VerifyHistory(data []byte) error {
	prev := ""
	for i, raw := range bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var ev HistoryEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			return fmt.Errorf("history line %d: not valid JSON: %w", i+1, err)
		}
		if ev.Prev != prev {
			return fmt.Errorf("history line %d: broken chain (prev %q, expected %q)", i+1, ev.Prev, prev)
		}
		want := historyDigest(ev.Prev, ev.Timestamp, ev.Event, ev.Step, ev.Note)
		if ev.Hash != want {
			return fmt.Errorf("history line %d: tampered (hash %q, recomputed %q)", i+1, ev.Hash, want)
		}
		prev = ev.Hash
	}
	return nil
}

// canonicalJSON re-encodes a JSON payload with sorted object keys (Go's
// encoding/json sorts map keys) so byte-different-but-equal payloads hash
// alike. An empty/nil payload encodes as JSON null.
func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage("null"), nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("canonicalize payload: %w", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}
