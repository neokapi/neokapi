package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

// The browser build has no file-backed SQLite (storage.ErrNoSQLite), and the
// review loop must still work in the lab, so the ledger and the checkout view
// live in process memory and persist to a JSON sidecar beside where the
// database would sit. The model is the one the database holds: an append-only
// ledger of content-addressed entries, and a view naming the pairing each unit
// has in this checkout.
//
// A sidecar is written by one checkout and read by the same one, so it carries
// no checkout column.

type memWork struct {
	path string

	// entries is the ledger in arrival order, and byID indexes it by content
	// address so recording an entry the ledger already holds is a no-op.
	entries []memEntry
	byID    map[string]int
	// latest answers a pairing with the entry in force at it.
	latest map[Pairing]int

	view map[Key]memViewRow
	docs map[string]memDoc

	// committed is the digest of the shards the view was imported from.
	committed string
}

// memEntry is one ledger entry in the sidecar.
type memEntry struct {
	ID       string      `json:"id"`
	State    UnitState   `json:"state"`
	Actor    string      `json:"actor,omitempty"`
	Origin   EntryOrigin `json:"origin,omitempty"`
	Recorded string      `json:"recorded"`
	Revoked  bool        `json:"revoked,omitempty"`
}

// memViewRow is one unit's pairing in this checkout.
type memViewRow struct {
	Scope       string           `json:"scope,omitempty"`
	Unit        string           `json:"unit"`
	Variant     model.VariantKey `json:"variant"`
	ContentHash string           `json:"contentHash,omitempty"`
	TargetHash  string           `json:"targetHash,omitempty"`
	// Exported reports that the committed shards already carry this row. A row
	// they do not carry survives an import that rebuilds the view, so a
	// decision made here and not yet written out is still written out.
	Exported bool `json:"exported,omitempty"`
}

func (r memViewRow) key() Key {
	return Key{Scope: r.Scope, Unit: r.Unit, Variant: r.Variant}
}

func (r memViewRow) pairing() Pairing {
	return Pairing{Key: r.key(), ContentHash: r.ContentHash, TargetHash: r.TargetHash}
}

// memDoc is one document's identity in the sidecar: where it lives now, and
// what it held when it was last read, which is what recognises it again after a
// rename.
type memDoc struct {
	Path    string   `json:"path"`
	Content []string `json:"content,omitempty"`
}

// memFile is the sidecar serialization.
type memFile struct {
	Entries   []memEntry        `json:"entries,omitempty"`
	View      []memViewRow      `json:"view,omitempty"`
	Docs      map[string]memDoc `json:"docs,omitempty"`
	Committed string            `json:"committed,omitempty"`
	// Units is the shape a sidecar written before the ledger carries. Its rows
	// are drained into entries and the view on load, because a decision made in
	// a browser session may have no other copy.
	Units []memLegacyUnit `json:"units,omitempty"`
}

// memLegacyUnit is one row of the pre-ledger sidecar.
type memLegacyUnit struct {
	Unit   UnitState `json:"unit"`
	Staged bool      `json:"staged,omitempty"`
}

func newMemWork(path string) *memWork {
	return &memWork{
		path:   path,
		byID:   map[string]int{},
		latest: map[Pairing]int{},
		view:   map[Key]memViewRow{},
		docs:   map[string]memDoc{},
	}
}

// load reads the sidecar back. A missing sidecar is an empty store; a malformed
// one is an error, because it may hold decisions no other copy has.
func (m *memWork) load(now time.Time) error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("state: read work set %s: %w", m.path, err)
	}
	var f memFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("state: parse work set %s: %w", m.path, err)
	}
	for _, e := range f.Entries {
		m.appendEntry(e)
	}
	for _, r := range f.View {
		m.view[r.key()] = r
	}
	if f.Docs != nil {
		m.docs = f.Docs
	}
	m.committed = f.Committed
	for _, u := range f.Units {
		origin := OriginImport
		if u.Staged {
			origin = OriginLocal
		}
		if err := m.record(u.Unit, u.Unit.Decision.By, origin, false, entryTimeText(now)); err != nil {
			return err
		}
		m.view[u.Unit.Key()] = memViewRow{
			Scope: u.Unit.Scope, Unit: u.Unit.Unit, Variant: u.Unit.Variant,
			ContentHash: u.Unit.ContentHash, TargetHash: u.Unit.TargetHash,
			Exported: !u.Staged,
		}
	}
	return nil
}

// appendEntry puts an entry in the ledger, keeping the address index and the
// per-pairing answer current. An address the ledger already holds is left
// alone.
func (m *memWork) appendEntry(e memEntry) {
	if _, held := m.byID[e.ID]; held {
		return
	}
	m.entries = append(m.entries, e)
	pos := len(m.entries) - 1
	m.byID[e.ID] = pos
	p := e.State.Pairing()
	if cur, ok := m.latest[p]; !ok || !entryPrecedes(m.entries[pos], m.entries[cur]) {
		m.latest[p] = pos
	}
}

// entryPrecedes reports whether a comes before b in the order that decides
// which entry answers for a pairing: the recorded stamp, then arrival.
func entryPrecedes(a, b memEntry) bool { return a.Recorded < b.Recorded }

// record appends one entry, addressed by what it says.
func (m *memWork) record(u UnitState, actor string, origin EntryOrigin, revoked bool, recorded string) error {
	id, err := Address(u, actor, revoked)
	if err != nil {
		return err
	}
	m.appendEntry(memEntry{ID: id, State: u, Actor: actor, Origin: origin, Recorded: recorded, Revoked: revoked})
	return nil
}

// applies returns the record in force at a pairing.
func (m *memWork) applies(p Pairing) (UnitState, bool) {
	pos, ok := m.latest[p]
	if !ok || m.entries[pos].Revoked {
		return UnitState{}, false
	}
	return m.entries[pos].State, true
}

// rows returns this checkout's view, ordered by the identity key.
func (m *memWork) rows() []memViewRow {
	out := make([]memViewRow, 0, len(m.view))
	for _, r := range m.view {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return viewLess(out[i], out[j]) })
	return out
}

func viewLess(a, b memViewRow) bool {
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	if a.Unit != b.Unit {
		return a.Unit < b.Unit
	}
	ka, _ := a.Variant.MarshalText()
	kb, _ := b.Variant.MarshalText()
	return string(ka) < string(kb)
}

// persist writes the store to the sidecar. Called after every mutation: this
// file is the browser build's only durable copy.
func (m *memWork) persist() error {
	f := memFile{Entries: m.entries, View: m.rows(), Committed: m.committed}
	if len(m.docs) > 0 {
		f.Docs = m.docs
	}
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("state: marshal work set: %w", err)
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("state: write work set: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return fmt.Errorf("state: rename work set: %w", err)
	}
	return nil
}

// documents returns the documents this checkout knows.
func (m *memWork) documents() []reconcile.DocUnit {
	out := make([]reconcile.DocUnit, 0, len(m.docs))
	for key, d := range m.docs {
		out = append(out, reconcile.DocUnit{Key: key, Path: d.Path, Content: d.Content})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
