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

// Store is the project state store: an authoritative record of unit workflow
// decisions. The working set lives in memory (or, behind the same interface, a
// SQLite index); the durable source of truth is the committed serialization that
// Save writes and Open reads — delete any derived index and Open rebuilds it.
type Store interface {
	// Get returns the unit's state, or ok=false when none is recorded.
	Get(k Key) (UnitState, bool)
	// Put records (or replaces) a unit's state.
	Put(s UnitState)
	// Delete removes a unit's state.
	Delete(k Key)
	// All returns every recorded state in deterministic order.
	All() []UnitState
	// Save persists the working set to the committed serialization. No-op when
	// nothing changed since Open/Save.
	Save() error
}

// FileStore is a Store whose source of truth is a committed JSON file. Open loads
// it into an in-memory working set; Put/Delete mutate the set; Save writes it
// back atomically. The file is the authoritative, diff-friendly artifact; the
// in-memory set is a derived index that can be thrown away and rebuilt from it.
type FileStore struct {
	path  string
	units map[Key]UnitState
	dirty bool
}

// Open loads the committed state file at path into a working store. A missing
// file yields an empty store (the project has no recorded state yet); a malformed
// file is an error (state is authoritative — never silently discard it).
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

// Save writes the working set back to the committed file atomically (temp +
// rename), deterministically ordered. No-op when nothing changed.
func (s *FileStore) Save() error {
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
