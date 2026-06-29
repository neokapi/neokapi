package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// SchemaVersion is the on-disk schema version of the committed state file.
const SchemaVersion = "1.0"

// Kind tags the committed state document.
const Kind = "kapi-project-state"

// file is the committed, diff-friendly serialization — the source of truth. Units
// are written in a deterministic order so the file diffs cleanly under git.
type file struct {
	SchemaVersion string      `json:"schemaVersion"`
	Kind          string      `json:"kind"`
	Units         []UnitState `json:"units"`
}

// Store is the project state store: a record of unit workflow decisions whose
// working set is explicitly TRANSIENT — mutations (Put/Delete) are in-transit and
// NOT durable until an explicit Export materializes them to the durable home (a
// committed serialization in git mode, or the server via `kapi push` in bowrain
// mode). This mirrors git/bowrain: decisions are like staged changes; you export
// deliberately (a CI step commits the state file or pushes to the server). The
// working set can be discarded and re-imported (Import / `kapi pull`) from the
// durable home — only un-exported decisions are lost, exactly like uncommitted
// changes. Pending() reports whether un-exported decisions exist so they are
// never lost silently.
type Store interface {
	// Get returns the unit's state, or ok=false when none is recorded.
	Get(k Key) (UnitState, bool)
	// Put records (or replaces) a unit's state in the transient working set.
	Put(s UnitState)
	// Delete removes a unit's state from the transient working set.
	Delete(k Key)
	// All returns every recorded state in deterministic order.
	All() []UnitState
	// Pending reports whether the working set holds decisions not yet exported.
	Pending() bool
	// Export materializes the working set to the durable home (the committed
	// serialization). No-op when nothing changed since Import/Export. The export
	// destination is a binding: a committed file now; the server (`kapi push`)
	// when the project binds one.
	Export() error
}

// FileStore is a Store whose durable home is a committed JSON file. Open imports
// it into a transient in-memory working set; Put/Delete mutate the set in transit;
// Export writes it back atomically. The file is the authoritative, diff-friendly
// artifact (the "remote" for git mode); the in-memory set is transient and can be
// thrown away and re-imported from it.
type FileStore struct {
	path  string
	units map[Key]UnitState
	dirty bool
}

// Open imports the committed state file at path into a transient working store. A
// missing file yields an empty store (no recorded state yet); a malformed file is
// an error (state is authoritative — never silently discard it).
func Open(path string) (*FileStore, error) {
	s := &FileStore{path: path, units: map[Key]UnitState{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("state: parse %s: %w", path, err)
	}
	if f.Kind != "" && f.Kind != Kind {
		return nil, fmt.Errorf("state: %s: unexpected kind %q", path, f.Kind)
	}
	for _, u := range f.Units {
		s.units[u.Key()] = u
	}
	return s, nil
}

func (s *FileStore) Get(k Key) (UnitState, bool) {
	u, ok := s.units[k]
	return u, ok
}

func (s *FileStore) Put(u UnitState) {
	s.units[u.Key()] = u
	s.dirty = true
}

func (s *FileStore) Delete(k Key) {
	if _, ok := s.units[k]; ok {
		delete(s.units, k)
		s.dirty = true
	}
}

// All returns every recorded state, ordered by (unit, variant) so callers and the
// serialization are deterministic.
func (s *FileStore) All() []UnitState {
	out := make([]UnitState, 0, len(s.units))
	for _, u := range s.units {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Unit != out[j].Unit {
			return out[i].Unit < out[j].Unit
		}
		ki, _ := out[i].Variant.MarshalText()
		kj, _ := out[j].Variant.MarshalText()
		return string(ki) < string(kj)
	})
	return out
}

// Pending reports whether the transient working set holds decisions not yet
// exported to the committed file — the "you have N unexported decisions" signal.
func (s *FileStore) Pending() bool { return s.dirty }

// Export writes the working set back to the committed file atomically (temp +
// rename), deterministically ordered. No-op when nothing changed. This is the
// explicit materialization step — mutations are transient until it runs.
func (s *FileStore) Export() error {
	if !s.dirty {
		return nil
	}
	f := file{SchemaVersion: SchemaVersion, Kind: Kind, Units: s.All()}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(&f); err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("state: mkdir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("state: write: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("state: rename: %w", err)
	}
	s.dirty = false
	return nil
}

// compile-time check.
var _ Store = (*FileStore)(nil)
