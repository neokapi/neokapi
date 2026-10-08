package workspace

import (
	"encoding/json"
	"strings"
	"time"
)

// OpContextReset records a person rewinding a project's context to how it
// stood before one operation. It is written by core/contextop as the kind
// "reset" under that package's "context." prefix, and read here because both
// readers of the log, the context ledger and the projector that rebuilds the
// stores, have to agree on what it sets aside.
//
// Its payload names the first operation set aside (Before). Every operation of
// the reset's kinds, in the reset's project, from Before up to the reset itself
// is set aside: it stays in the log, and nothing reading the log applies it.
// The reset is itself an operation, so a later reset before it sets it aside in
// turn and what it set aside applies again.
const OpContextReset = "context.reset"

// resetPayload is the part of a reset's payload this package reads.
type resetPayload struct {
	Before string `json:"before"`
}

// ResetBefore reads the first operation a reset sets aside, empty when op is
// not a reset or names none.
func ResetBefore(op Op) string {
	if op.Kind != OpContextReset || len(op.Payload) == 0 {
		return ""
	}
	var body resetPayload
	if json.Unmarshal(op.Payload, &body) != nil {
		return ""
	}
	return body.Before
}

// OpIDAt is the lowest operation id a moment can carry, so every operation
// accepted at that moment or later sorts at or after it. A reset back to a
// moment names it as the first operation set aside.
func OpIDAt(at time.Time) string {
	ms := max(at.Sub(opIDEpoch).Milliseconds(), 0)
	b := []byte(strings.Repeat("0", OpIDLength))
	for i := opIDTimeChars - 1; i >= 0; i-- {
		b[i] = crockford[ms&31]
		ms >>= 5
	}
	return string(b)
}

// ResetKind reports whether a reset rewinds operations of this kind: the
// context operations themselves and the writes that fill the terms store, the
// content memory, the voice profiles and the widened rules. A document's edits,
// adoptions and review decisions are the project's content rather than its
// context, and a reset leaves them alone.
func ResetKind(kind string) bool {
	if strings.HasPrefix(kind, "context.") {
		return true
	}
	switch kind {
	case "terms.write", "memory.write", "voice.write", "rules.write":
		return true
	}
	return false
}

// SetAside reports the operations the resets among ops set aside, by id.
//
// The log is read newest first, in id order. A reset still in force sets
// aside every operation of a reset kind in its project from the one it names
// up to itself, and the reading resumes below that point, so a reset inside
// the range a later reset covers is itself set aside and has no effect.
// Operations of other kinds are never set aside. ops need not be sorted.
func SetAside(ops []Op) map[string]bool {
	byProject := map[ProjectKey][]Op{}
	resets := false
	for _, op := range ops {
		if !ResetKind(op.Kind) {
			continue
		}
		byProject[op.Project] = append(byProject[op.Project], op)
		resets = resets || op.Kind == OpContextReset
	}
	aside := map[string]bool{}
	if !resets {
		return aside
	}
	for _, held := range byProject {
		SortOps(held)
		for i := len(held) - 1; i >= 0; {
			before := ResetBefore(held[i])
			if before == "" {
				i--
				continue
			}
			j := i - 1
			for j >= 0 && held[j].ID >= before {
				aside[held[j].ID] = true
				j--
			}
			i = j
		}
	}
	return aside
}
