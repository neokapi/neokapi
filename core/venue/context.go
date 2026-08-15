package venue

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"

	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
)

// The context content type: a project's declared structure and governance —
// which collections exist, where each sits in the project's context space, and
// which voice governs it — carried inside the push transaction.
//
// This file holds the parts both sides need: the ownership vocabulary and the
// hashes. It lives in the framework-only bowrain/core module, beside the block
// hashes, so the push client and the server compute them from one definition
// rather than two that have to agree.

// Ownership of a context entity: which side of the sync is authoritative for
// it. Recipe-owned means kapi.yaml decides, so the server stores the entry and
// reads it but never rewrites it, and a pull leaves the local governance alone.
// Workspace-owned means the server decides, and the recipe has no say.
const (
	ContextOwnerRecipe    = "recipe"
	ContextOwnerWorkspace = "workspace"
)

// NormalizeContextOwner defaults anything unrecognized to workspace ownership.
//
// The default is deliberately the conservative one. A collection created before
// this content type existed — by the web hub, the editor, a connector — carries
// no owner, and reading it as recipe-owned would hand authority over it to a
// kapi.yaml that never mentioned it. The server owns what the recipe has not
// claimed.
func NormalizeContextOwner(owner string) string {
	if owner == ContextOwnerRecipe {
		return ContextOwnerRecipe
	}
	return ContextOwnerWorkspace
}

// IsRecipeOwned reports whether the owner value means "kapi.yaml is
// authoritative". It is the single lookup every ownership decision goes
// through: refusing a server-side mutation, and skipping an entry on pull.
func IsRecipeOwned(owner string) bool {
	return NormalizeContextOwner(owner) == ContextOwnerRecipe
}

// ComputeContextEntryHash hashes one context entry over the fields the protocol
// owns: the collection name, its coordinates, its resolved governance, and its
// owner. The authored profile content is folded in as well, so editing the
// voice a collection is governed by changes the entry's hash and the push
// carries the new governance instead of being skipped by the fast path.
//
// Server-assigned state (the collection id, timestamps, the resolved profile
// id) is deliberately excluded: the hash answers "has the recipe changed?", and
// including anything the server minted would make the answer always yes.
func ComputeContextEntryHash(e *pb.SyncContextEntry) string {
	if e == nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(e.Name))
	for _, axis := range slices.Sorted(maps.Keys(e.Coordinates)) {
		h.Write([]byte(axis))
		h.Write([]byte{0})
		h.Write([]byte(e.Coordinates[axis]))
		h.Write([]byte{0})
	}
	h.Write([]byte(e.Channel))
	h.Write([]byte{0})
	h.Write([]byte(e.VoiceProfile))
	h.Write([]byte{0})
	h.Write(e.VoiceProfileJson)
	h.Write([]byte{0})
	h.Write([]byte(NormalizeContextOwner(e.Owner)))
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeContextHash folds every entry hash into the project's context hash —
// the fast path the push init negotiates on. It is the same name→hash fold as
// ComputeRootHash, over context entries instead of items, so the two sides
// compare like for like: the client hashes what the recipe declares, the server
// hashes the entry hashes it stored on the collections.
func ComputeContextHash(entryHashes map[string]string) string {
	return ComputeRootHash(entryHashes)
}

// ContextHashOf is the whole client-side computation in one call: hash each
// entry, then fold the entry hashes. It also stamps each entry's ContentHash,
// so what the client negotiates on and what it sends in the manifest cannot
// drift apart.
func ContextHashOf(entries []*pb.SyncContextEntry) string {
	byName := make(map[string]string, len(entries))
	for _, e := range entries {
		if e == nil || e.Name == "" {
			continue
		}
		e.ContentHash = ComputeContextEntryHash(e)
		byName[e.Name] = e.ContentHash
	}
	return ComputeContextHash(byName)
}
