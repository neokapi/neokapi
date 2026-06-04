package redaction

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// RedactedValue is one secret↔token mapping held by a [Vault]. It is the
// authoritative — and only — record of an original sensitive value.
type RedactedValue struct {
	Token    string         `json:"token"`
	Category string         `json:"category"`
	Disp     string         `json:"disp,omitempty"` // visible placeholder string, for text-based restore
	Original string         `json:"original"`
	Locale   model.LocaleID `json:"locale,omitempty"`
	BlockID  string         `json:"blockId,omitempty"`
}

// Vault stores the original values that redaction removed from content,
// keyed by block ID and token. Implementations keep the originals local: an
// in-process map for single-run flows, or a gitignored sidecar file for the
// extract → external translation → merge roundtrip.
type Vault interface {
	// Put stores (or overwrites) a value. BlockID and Token together form
	// the key.
	Put(v RedactedValue) error
	// Get returns the value for a block/token pair.
	Get(blockID, token string) (RedactedValue, bool)
	// All returns every stored value in a stable order.
	All() []RedactedValue
}

func vaultKey(blockID, token string) string { return blockID + "\x00" + token }

// ValuesForBlock returns every stored value belonging to a block. Used for
// text-based restore, which matches by visible placeholder rather than by
// token.
func ValuesForBlock(v Vault, blockID string) []RedactedValue {
	var out []RedactedValue
	for _, rv := range v.All() {
		if rv.BlockID == blockID {
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
	v.m[vaultKey(rv.BlockID, rv.Token)] = rv
	return nil
}

// Get returns the value for a block/token pair.
func (v *MemoryVault) Get(blockID, token string) (RedactedValue, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	rv, ok := v.m[vaultKey(blockID, token)]
	return rv, ok
}

// All returns every stored value sorted by block ID then token.
func (v *MemoryVault) All() []RedactedValue {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return sortedValues(v.m)
}

// fileVaultDoc is the on-disk shape of a sidecar vault.
type fileVaultDoc struct {
	Version int             `json:"version"`
	Values  []RedactedValue `json:"values"`
}

const fileVaultVersion = 1

// FileVault is a [Vault] backed by a JSON sidecar file. It is meant to live
// under a project's gitignored cache directory (e.g.
// .kapi/cache/redaction/<batch>.json) so secrets stay on the machine and out
// of version control. Writes are buffered in memory; call [FileVault.Flush]
// (or [FileVault.Close]) to persist.
type FileVault struct {
	path string
	mem  *MemoryVault
}

// OpenFileVault opens (loading if present) a sidecar vault at path. A missing
// file yields an empty vault; the file is not created until Flush.
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
	for _, rv := range doc.Values {
		_ = fv.mem.Put(rv)
	}
	return fv, nil
}

// Put stores a value in memory; it is persisted on the next Flush.
func (f *FileVault) Put(rv RedactedValue) error { return f.mem.Put(rv) }

// Get returns the value for a block/token pair.
func (f *FileVault) Get(blockID, token string) (RedactedValue, bool) {
	return f.mem.Get(blockID, token)
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
	doc := fileVaultDoc{Version: fileVaultVersion, Values: f.mem.All()}
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
		return cmp.Or(cmp.Compare(a.BlockID, b.BlockID), cmp.Compare(a.Token, b.Token))
	})
	return out
}
