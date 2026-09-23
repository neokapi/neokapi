package workspace

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// An operation id names one operation in every log that holds it.
//
// It is 24 characters of lower-case Crockford base32: eight for the moment the
// operation was accepted, as milliseconds since the start of 2026, and sixteen
// random ones. The time comes first, so ids sort in the order operations were
// accepted, and two logs recorded on different machines merge by taking the
// union and sorting by id. The random part breaks the tie between two machines
// that accepted an operation in the same millisecond.
//
// A backend never hands out an id that sorts before one its log already holds:
// where the clock has not moved past the newest id (two operations in one
// millisecond, a merged log from a machine whose clock runs ahead), the time
// part advances one millisecond past it. An operation recorded after another
// was seen therefore sorts after it, whichever machine recorded either.
//
// A person reads and types the first ten characters (ShortOpID), as git shows a
// commit. Any unambiguous prefix resolves (ResolveOpID).

// opIDEpoch is the zero of an id's time part.
var opIDEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

const (
	// opIDTimeChars is the width of the time part: 40 bits of milliseconds,
	// which reaches into the 2060s.
	opIDTimeChars = 8
	// opIDRandomChars is the width of the random part: 80 bits.
	opIDRandomChars = 16
	// OpIDLength is the length of a full operation id.
	OpIDLength = opIDTimeChars + opIDRandomChars
	// ShortOpIDLength is the length of the form a log line shows: the time part
	// and two random characters. One machine never accepts two operations in
	// one millisecond, so its short ids never collide; two machines collide
	// only when they accepted operations in the same millisecond and drew the
	// same ten random bits.
	ShortOpIDLength = opIDTimeChars + 2
)

// crockford is the base32 alphabet an id is written in: digits and lower-case
// letters, without i, l, o and u, so an id read aloud or typed from a screen
// has no look-alike characters.
const crockford = "0123456789abcdefghjkmnpqrstvwxyz"

// NewOpID mints an operation id for an operation accepted at the given moment,
// sorting after the id given as after. An empty after means the log is empty.
func NewOpID(at time.Time, after string) string {
	ms := max(at.Sub(opIDEpoch).Milliseconds(), 0)
	if prev, ok := opIDMillis(after); ok && ms <= prev {
		ms = prev + 1
	}
	var b [OpIDLength]byte
	for i := opIDTimeChars - 1; i >= 0; i-- {
		b[i] = crockford[ms&31]
		ms >>= 5
	}
	var random [opIDRandomChars]byte
	_, _ = rand.Read(random[:])
	for i, r := range random {
		b[opIDTimeChars+i] = crockford[r&31]
	}
	return string(b[:])
}

// opIDMillis reads an id's time part back as milliseconds since the epoch.
func opIDMillis(id string) (int64, bool) {
	if len(id) < opIDTimeChars {
		return 0, false
	}
	var ms int64
	for i := range opIDTimeChars {
		v := strings.IndexByte(crockford, id[i])
		if v < 0 {
			return 0, false
		}
		ms = ms<<5 | int64(v)
	}
	return ms, true
}

// OpIDTime reports the moment an id's time part names. It is the moment the
// operation was accepted, or a millisecond or so after it where the backend
// had to advance past a newer id.
func OpIDTime(id string) (time.Time, bool) {
	ms, ok := opIDMillis(id)
	if !ok {
		return time.Time{}, false
	}
	return opIDEpoch.Add(time.Duration(ms) * time.Millisecond), true
}

// ValidOpID reports whether s is a full operation id.
func ValidOpID(s string) bool {
	if len(s) != OpIDLength {
		return false
	}
	for i := range len(s) {
		if strings.IndexByte(crockford, s[i]) < 0 {
			return false
		}
	}
	return true
}

// ShortOpID renders an id the way a log line shows it: its first ten
// characters. Anything that is not a full id is returned as it stands.
func ShortOpID(id string) string {
	if len(id) == OpIDLength {
		return id[:ShortOpIDLength]
	}
	return id
}

// NormalizeOpIDPrefix reads what a person typed as an id or the start of one:
// surrounding space and a leading "#" go, and the letters are folded to lower
// case, so an id copied from a log line or read aloud resolves.
func NormalizeOpIDPrefix(typed string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(typed), "#"))
}

// ErrNoOperation reports an id, or the start of one, that names no operation.
var ErrNoOperation = errors.New("no such operation")

// AmbiguousOpIDError reports a typed prefix that starts more than one id. It
// lists every candidate, so a person can type enough to tell them apart.
type AmbiguousOpIDError struct {
	Prefix     string
	Candidates []string
}

func (e *AmbiguousOpIDError) Error() string {
	return fmt.Sprintf("%q names %d operations (%s): type more of the id",
		e.Prefix, len(e.Candidates), strings.Join(e.Candidates, ", "))
}

// ResolveOpID finds the id a typed prefix names among the ids given. A full id
// resolves to itself; a prefix resolves when exactly one id starts with it. It
// reports ErrNoOperation when none does and an *AmbiguousOpIDError when more
// than one does.
func ResolveOpID(typed string, ids []string) (string, error) {
	prefix := NormalizeOpIDPrefix(typed)
	if prefix == "" {
		return "", fmt.Errorf("%w: no id given", ErrNoOperation)
	}
	var found []string
	for _, id := range ids {
		if id == prefix {
			return id, nil
		}
		if strings.HasPrefix(id, prefix) {
			found = append(found, id)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("%w: %s", ErrNoOperation, typed)
	case 1:
		return found[0], nil
	}
	slices.Sort(found)
	return "", &AmbiguousOpIDError{Prefix: prefix, Candidates: found}
}

// SortOps orders operations by id, which is the order they were accepted in
// across every log that has been merged into this one.
func SortOps(ops []Op) {
	slices.SortStableFunc(ops, func(a, b Op) int { return strings.Compare(a.ID, b.ID) })
}

// Merge records into a log every operation read from another, and reports how
// many it did not already hold.
//
// It is the union of two logs: an operation the log holds under the same id or
// content address is left as it is, so merging the same operations again adds
// nothing, and two logs merged into each other hold the same operations in the
// same id order, whichever was merged first.
func Merge(ctx context.Context, into Backend, ops []Op) (int, error) {
	if len(ops) == 0 {
		return 0, nil
	}
	before, err := into.Head(ctx)
	if err != nil {
		return 0, err
	}
	arriving := make([]Op, len(ops))
	for i, op := range ops {
		if op.ID == "" {
			return 0, fmt.Errorf("workspace: merge %s: an operation read from another log carries an id", op.Kind)
		}
		op.Seq = 0
		arriving[i] = op
	}
	SortOps(arriving)
	held, err := into.Record(ctx, arriving...)
	if err != nil {
		return 0, err
	}
	added := 0
	for _, op := range held {
		if op.Seq > before {
			added++
		}
	}
	return added, nil
}
