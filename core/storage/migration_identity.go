package storage

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// A migration ledger records each migration under its version, its name
// (Description) and its identity: a digest of its SQL, plus the revision a
// step written in Go declares (Revision). Before applying anything, a runner
// compares the ledger with the code (CheckLedger) and refuses to migrate when
// they disagree, which stops a server before it serves.
//
// A row recorded before identities existed is adopted on the first run that
// sees it: the code's name and identity are written over it once, without
// comparing, because some names were reworded after their rows were recorded.

// MigrationIdentity is the identity a ledger records for m: a digest of its
// name, its SQL as the database executes it (NormalizeSQL, over the tokenizer
// placeholder rather than the build's tokenizer) and, for a step written in
// Go, its declared revision. Editing what a shipped migration executes
// changes it; rewording a comment inside it does not. A change inside a Go step that keeps its
// revision is invisible to the digest, which is why ValidateMigrations
// requires a Go step to declare one.
func MigrationIdentity(m Migration) string {
	h := sha256.New()
	h.Write([]byte(m.Description))
	h.Write([]byte{0})
	h.Write([]byte(NormalizeSQL(m.SQL)))
	if m.Apply != nil {
		h.Write([]byte{0})
		h.Write([]byte(m.Revision))
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// AppliedMigration is one row of a migration ledger.
type AppliedMigration struct {
	Version     int
	Description string
	// Identity is empty for a row recorded before identities existed.
	Identity string
}

// ValidateMigrations checks a ledger's migrations in the code: one migration
// per version, and versions ascending without a gap. A version that is spent
// and no longer applied is declared Retired and carries no work. Every step
// written in Go declares its Revision. It returns them sorted by version.
func ValidateMigrations(ledger string, migrations []Migration) ([]Migration, error) {
	ordered := slices.Clone(migrations)
	slices.SortStableFunc(ordered, func(a, b Migration) int { return cmp.Compare(a.Version, b.Version) })
	var problems []string
	for i, m := range ordered {
		if i > 0 {
			prev := ordered[i-1]
			switch {
			case m.Version == prev.Version:
				problems = append(problems, fmt.Sprintf("version %d is issued twice: %q and %q", m.Version, prev.Description, m.Description))
			case m.Version != prev.Version+1:
				problems = append(problems, fmt.Sprintf("versions jump from %d to %d; declare each spent version between them Retired", prev.Version, m.Version))
			}
		}
		if m.Retired && (m.SQL != "" || m.Apply != nil) {
			problems = append(problems, fmt.Sprintf("version %d is Retired and still carries work", m.Version))
		}
		if m.Apply != nil && strings.TrimSpace(m.Revision) == "" {
			problems = append(problems, fmt.Sprintf("version %d (%q) runs Go and declares no Revision; a change inside it would be invisible to the ledger", m.Version, m.Description))
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("the migrations of %q are not a ledger: %s", ledger, strings.Join(problems, "; "))
	}
	return ordered, nil
}

// ShippedMigration is what a committed ledger file (migration_ledger.json)
// knows about one shipped version: the identity it shipped with, and every
// name it has carried, the current one included, read from the history of
// the migration files.
type ShippedMigration struct {
	Description string   `json:"description"`
	Identity    string   `json:"identity"`
	Names       []string `json:"names"`
}

// ShippedLedgers maps a ledger's table to its shipped versions.
type ShippedLedgers map[string]map[int]ShippedMigration

// CheckLedger compares what a database recorded with the code's migrations
// (validated and sorted) and with what shipped (shipped, the ledger's entry in
// the committed file; nil for a ledger it does not hold). It returns the
// migrations whose rows carry no identity yet, for the runner to adopt, and
// refuses, naming both sides, when:
//
//   - a shipped version's code no longer has the identity it shipped with;
//   - a recorded row's name is neither the code's nor a name that version has
//     carried (shipped.Names);
//   - a recorded row's identity differs from the code's.
//
// A row without an identity is adopted only when its name passes that test,
// so the first run that turns the check on refuses a collision instead of
// accepting it. A recorded version the code no longer carries (a folded
// baseline, a retired number) is left alone.
func CheckLedger(ledger string, migrations []Migration, applied []AppliedMigration, shipped map[int]ShippedMigration) ([]Migration, error) {
	byVersion := make(map[int]Migration, len(migrations))
	var problems []string
	for _, m := range migrations {
		if m.Retired {
			continue
		}
		byVersion[m.Version] = m
		if s, ok := shipped[m.Version]; ok && s.Identity != MigrationIdentity(m) {
			problems = append(problems, fmt.Sprintf("version %d (%q) shipped with identity %s and the code's version %d now has identity %s: a shipped migration was edited", m.Version, s.Description, s.Identity, m.Version, MigrationIdentity(m)))
		}
	}
	var adopt []Migration
	for _, a := range applied {
		m, ok := byVersion[a.Version]
		if !ok {
			continue
		}
		if !knownName(a.Description, m, shipped[a.Version]) {
			problems = append(problems, fmt.Sprintf("version %d is recorded as %q and the code's version %d is %q, a name that version has never carried", a.Version, a.Description, m.Version, m.Description))
			continue
		}
		switch {
		case a.Identity == "":
			adopt = append(adopt, m)
		case a.Identity != MigrationIdentity(m):
			problems = append(problems, fmt.Sprintf("version %d (%q) was applied with identity %s and the code's version %d now has identity %s: a shipped migration was edited", a.Version, a.Description, a.Identity, m.Version, MigrationIdentity(m)))
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("refusing to migrate %q: %s", ledger, strings.Join(problems, "; "))
	}
	return adopt, nil
}

// knownName reports whether recorded is a name the version has carried: the
// code's current name, or one of the names the committed ledger file read
// from the version's history.
func knownName(recorded string, m Migration, s ShippedMigration) bool {
	return recorded == m.Description || slices.Contains(s.Names, recorded)
}
