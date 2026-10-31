package redaction

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// RedactedValue is one secret↔token mapping held by a [Vault]. It is the
// only record of an original sensitive value.
type RedactedValue struct {
	Token    string         `json:"token"`
	Category string         `json:"category"`
	Disp     string         `json:"disp,omitempty"` // visible placeholder string, for text-based restore
	Original string         `json:"original"`
	Locale   model.LocaleID `json:"locale,omitempty"`
	// Doc names the document the block sits in: its project-relative,
	// slash-separated path. Empty names content that sits in no document,
	// whose block ID names it alone (a recorded edit, a note), and every
	// entry a version 1 vault holds.
	Doc     string `json:"doc,omitempty"`
	BlockID string `json:"blockId,omitempty"`
}

// Vault stores the original values that redaction removed from content,
// keyed by document, block ID and token. Block IDs and tokens repeat from
// one document to the next (every JSON file can hold a block "greeting",
// and the token counter starts again in every block), so the document is
// part of the key. Implementations keep the originals local: an in-process
// map for single-run flows, or a gitignored sidecar file for the extract →
// external translation → merge roundtrip.
type Vault interface {
	// Put stores (or overwrites) a value. Doc, BlockID and Token together
	// form the key.
	Put(v RedactedValue) error
	// Get returns the value for a document/block/token triple. When doc is
	// set and the vault holds nothing under it, Get returns the entry for
	// the block and token that names no document, which is how a version 1
	// vault stored every entry.
	Get(doc, blockID, token string) (RedactedValue, bool)
	// All returns every stored value in a stable order.
	All() []RedactedValue
}

func vaultKey(doc, blockID, token string) string {
	return doc + "\x00" + blockID + "\x00" + token
}

// ValuesForBlock returns every stored value belonging to one block of one
// document. Used for text-based restore, which matches by visible
// placeholder rather than by token. When doc is set and the vault holds
// nothing for the block under it, it returns the block's entries that name
// no document. A version 1 vault keys every entry by block alone, so an
// extraction or a push it recorded still restores.
func ValuesForBlock(v Vault, doc, blockID string) []RedactedValue {
	var exact, legacy []RedactedValue
	for _, rv := range v.All() {
		if rv.BlockID != blockID {
			continue
		}
		switch rv.Doc {
		case doc:
			exact = append(exact, rv)
		case "":
			legacy = append(legacy, rv)
		}
	}
	if len(exact) > 0 || doc == "" {
		return exact
	}
	return legacy
}

// MemoryVault is a concurrency-safe in-process [Vault]. Nothing it holds is
// ever written to disk, so it is the natural backing for a single-process
// secure-translate flow.
type MemoryVault struct {
	mu sync.RWMutex
	m  map[string]RedactedValue
}

// NewMemoryVault returns an empty in-process vault.
func NewMemoryVault() *MemoryVault {
	return &MemoryVault{m: make(map[string]RedactedValue)}
}

// Put stores a value.
func (v *MemoryVault) Put(rv RedactedValue) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m[vaultKey(rv.Doc, rv.BlockID, rv.Token)] = rv
	return nil
}

// Get returns the value for a document/block/token triple, falling back to
// the entry that names no document (see [Vault]).
func (v *MemoryVault) Get(doc, blockID, token string) (RedactedValue, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if rv, ok := v.m[vaultKey(doc, blockID, token)]; ok {
		return rv, true
	}
	if doc == "" {
		return RedactedValue{}, false
	}
	rv, ok := v.m[vaultKey("", blockID, token)]
	return rv, ok
}

// All returns every stored value sorted by document, block ID, then token.
func (v *MemoryVault) All() []RedactedValue {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return sortedValues(v.m)
}

// fileVaultDoc is the on-disk shape of a sidecar vault.
type fileVaultDoc struct {
	Version int `json:"version"`
	// ID names the vault to the package it restores: a redacted .kpz records
	// the same ID in its manifest, and kapi merge restores the package only
	// from the vault whose ID matches. Empty for a vault no package names.
	ID     string          `json:"id,omitempty"`
	Values []RedactedValue `json:"values"`
}

// fileVaultVersion is the version Flush writes. Version 3 records the
// vault's ID, and version 2 each entry's document (RedactedValue.Doc). A file
// below version 3 has no ID, so no redacted package names it. A version 1
// file reads as entries that name no document, which Get and ValuesForBlock
// fall back to. The vault is the only record of the originals, never a cache
// a later run rebuilds, so it is read as it is rather than reset. A file with
// no version field is version 1, and a file above this version is refused
// (ErrNewerVault).
const fileVaultVersion = 3

// ErrNewerVault is the refusal of a vault file whose version is above the
// one this binary writes. A newer kapi wrote it in a format this one cannot
// read, so its entries could restore the wrong originals. OpenFileVault
// returns it wrapped with the file, the version found and the version
// supported.
var ErrNewerVault = errors.New("a newer kapi wrote this vault")

// FileVault is a [Vault] backed by a JSON sidecar file. It is meant to live
// in a project's vault directory (.kapi/vault/, prepared owner-only and out of
// version control by project.Layout.PrepareVault) so secrets stay on the
// machine. Writes are buffered in memory; call [FileVault.Flush]
// (or [FileVault.Close]) to persist.
type FileVault struct {
	path string
	id   string
	mem  *MemoryVault
}

// OpenFileVault opens (loading if present) a sidecar vault at path. A missing
// file yields an empty vault; the file is not created until Flush. A file
// whose version is above fileVaultVersion is refused with [ErrNewerVault],
// so neither its entries nor a Flush over it are ever used.
func OpenFileVault(path string) (*FileVault, error) {
	fv := &FileVault{path: path, mem: NewMemoryVault()}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fv, nil
		}
		return nil, fmt.Errorf("redaction: open vault %s: %w", path, err)
	}
	var doc fileVaultDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("redaction: parse vault %s: %w", path, err)
	}
	if doc.Version > fileVaultVersion {
		return nil, fmt.Errorf("redaction: vault %s is version %d and this kapi reads up to version %d: %w; upgrade kapi to read it",
			path, doc.Version, fileVaultVersion, ErrNewerVault)
	}
	fv.id = doc.ID
	for _, rv := range doc.Values {
		_ = fv.mem.Put(rv)
	}
	return fv, nil
}

// NewVaultID returns a fresh random vault ID of 32 lowercase hex digits.
func NewVaultID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ID returns the vault's ID, empty when it has none.
func (f *FileVault) ID() string { return f.id }

// SetID sets the ID the next Flush writes.
func (f *FileVault) SetID(id string) { f.id = id }

// Put stores a value in memory; it is persisted on the next Flush.
func (f *FileVault) Put(rv RedactedValue) error { return f.mem.Put(rv) }

// Get returns the value for a document/block/token triple (see [Vault]).
func (f *FileVault) Get(doc, blockID, token string) (RedactedValue, bool) {
	return f.mem.Get(doc, blockID, token)
}

// All returns every stored value.
func (f *FileVault) All() []RedactedValue { return f.mem.All() }

// Path returns the sidecar file path.
func (f *FileVault) Path() string { return f.path }

// Flush writes the vault to its sidecar file, creating parent directories as
// needed. The file is written 0600 since it holds secrets.
func (f *FileVault) Flush() error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return fmt.Errorf("redaction: create vault dir: %w", err)
	}
	doc := fileVaultDoc{Version: fileVaultVersion, ID: f.id, Values: f.mem.All()}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("redaction: encode vault: %w", err)
	}
	if err := os.WriteFile(f.path, data, 0o600); err != nil {
		return fmt.Errorf("redaction: write vault %s: %w", f.path, err)
	}
	return nil
}

// Close flushes and is provided for callers that prefer defer Close().
func (f *FileVault) Close() error { return f.Flush() }

func sortedValues(m map[string]RedactedValue) []RedactedValue {
	out := make([]RedactedValue, 0, len(m))
	for _, rv := range m {
		out = append(out, rv)
	}
	slices.SortFunc(out, func(a, b RedactedValue) int {
		return cmp.Or(cmp.Compare(a.Doc, b.Doc), cmp.Compare(a.BlockID, b.BlockID), cmp.Compare(a.Token, b.Token))
	})
	return out
}
