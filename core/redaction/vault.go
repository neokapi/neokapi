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
	// whose block ID names it alone (a recorded edit, a note).
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
	// Get returns the value for a document/block/token triple.
	Get(doc, blockID, token string) (RedactedValue, bool)
	// All returns every stored value in a stable order.
	All() []RedactedValue
}

func vaultKey(doc, blockID, token string) string {
	return doc + "\x00" + blockID + "\x00" + token
}

// ValuesForBlock returns every stored value belonging to one block of one
// document. Used for text-based restore, which matches by visible
// placeholder rather than by token.
func ValuesForBlock(v Vault, doc, blockID string) []RedactedValue {
	var out []RedactedValue
	for _, rv := range v.All() {
		if rv.BlockID == blockID && rv.Doc == doc {
			out = append(out, rv)
		}
	}
	return out
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

// Get returns the value for a document/block/token triple.
func (v *MemoryVault) Get(doc, blockID, token string) (RedactedValue, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	rv, ok := v.m[vaultKey(doc, blockID, token)]
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

// readVaultDoc is the on-disk shape as OpenFileVault reads it: Version is nil
// for a file with no version field.
type readVaultDoc struct {
	Version *int            `json:"version"`
	ID      string          `json:"id,omitempty"`
	Values  []RedactedValue `json:"values"`
}

// fileVaultVersion is the version Flush writes. Version 2 records each
// entry's document (RedactedValue.Doc), and may record the vault's ID; a
// vault with no ID restores no redacted package. A file above it was written
// by a newer kapi and is refused with ErrNewerVault.
const fileVaultVersion = 2

// oldestVaultVersion is the oldest version OpenFileVault reads. Version 2 is
// the first that records each entry's document (RedactedValue.Doc). A file
// below it, or with no version field, is refused with an [*OlderVaultError].
// It stays where it is when fileVaultVersion moves, so the vaults this kapi
// writes still open under a kapi that writes a later version. The vault is the
// only record of the originals, never a cache a later run rebuilds, so a
// refused file is left as it is.
const oldestVaultVersion = 2

// ErrOlderVault is the refusal of a vault file whose version is below
// oldestVaultVersion, or which carries no version. An earlier kapi wrote it
// keyed by block and token alone, so a lookup by document finds none of its
// entries. OpenFileVault returns it as an [*OlderVaultError].
var ErrOlderVault = errors.New("an earlier kapi wrote this vault")

// OlderVaultError names a vault file an earlier kapi wrote and the version it
// carries, or NoVersion for a file with no version field. Run is the sentence
// that names the command writing a new vault in its place: the caller that
// knows which vault it opened sets it.
type OlderVaultError struct {
	Path      string
	Version   int
	NoVersion bool
	Run       string
}

func (e *OlderVaultError) Error() string {
	found := fmt.Sprintf("version %d", e.Version)
	if e.NoVersion {
		found = "no version"
	}
	msg := fmt.Sprintf("%s is a redaction vault from an earlier kapi (%s).", e.Path, found)
	if e.Run != "" {
		msg += " " + e.Run
	}
	return msg
}

// Unwrap makes errors.Is(err, ErrOlderVault) hold.
func (e *OlderVaultError) Unwrap() error { return ErrOlderVault }

// WithRun returns err with the run sentence set on the OlderVaultError it
// carries, or err unchanged when it carries none. The result is the
// OlderVaultError itself, without the context err wrapped it in, so the
// message reads as the two sentences it is.
func WithRun(err error, run string) error {
	var ov *OlderVaultError
	if !errors.As(err, &ov) {
		return err
	}
	out := *ov
	out.Run = run
	return &out
}

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
// whose version is above fileVaultVersion is refused with [ErrNewerVault], and
// one below oldestVaultVersion, or with no version, with an
// [*OlderVaultError], so neither its
// entries nor a Flush over it are ever used.
func OpenFileVault(path string) (*FileVault, error) {
	fv := &FileVault{path: path, mem: NewMemoryVault()}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fv, nil
		}
		return nil, fmt.Errorf("redaction: open vault %s: %w", path, err)
	}
	var doc readVaultDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("redaction: parse vault %s: %w", path, err)
	}
	switch {
	case doc.Version == nil:
		return nil, &OlderVaultError{Path: path, NoVersion: true}
	case *doc.Version > fileVaultVersion:
		return nil, fmt.Errorf("redaction: vault %s is version %d and this kapi reads up to version %d: %w; upgrade kapi to read it",
			path, *doc.Version, fileVaultVersion, ErrNewerVault)
	case *doc.Version < oldestVaultVersion:
		return nil, &OlderVaultError{Path: path, Version: *doc.Version}
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
