package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// BlockIdentity provides content-addressable identification for blocks,
// enabling deduplication and change detection in the ContentStore.
type BlockIdentity struct {
	ContentHash string // SHA-256 of normalized source text
	ContextHash string // SHA-256 of contextual information (name, type, properties)
}

// ComputeContentHash computes a SHA-256 hash of the normalized source text.
//
// Normalization is leading/trailing whitespace trimming ONLY (strings.TrimSpace).
// It deliberately does NOT change case, collapse interior whitespace, or apply
// any other transform — case and interior spacing are content-significant.
//
// WARNING: this normalization is part of the on-the-wire content-hash contract
// used by the sync diff engine. Changing it (e.g. adding lowercasing or
// Unicode normalization) alters every emitted hash and would force a full
// re-sync of all existing projects. Do NOT change the normalization without a
// coordinated hash-version migration. The golden-hash test in identity_test.go
// pins the exact output for a known input to catch accidental changes.
func ComputeContentHash(sourceText string) string {
	normalized := strings.TrimSpace(sourceText)
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// ComputeContextHash computes a SHA-256 hash of contextual information
// (block name, type, and sorted properties) to detect structural changes.
func ComputeContextHash(name, typ string, properties map[string]string) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte{0})
	h.Write([]byte(typ))
	h.Write([]byte{0})

	// Sort keys for deterministic hashing.
	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(properties[k]))
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))
}

// sortStrings sorts a slice of strings in place (insertion sort, avoids sort import).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// ComputeIdentity computes a BlockIdentity from a Block.
func ComputeIdentity(b *Block) *BlockIdentity {
	props := b.Properties
	if props == nil {
		props = map[string]string{}
	}
	return &BlockIdentity{
		ContentHash: ComputeContentHash(b.SourceText()),
		ContextHash: ComputeContextHash(b.Name, b.Type, props),
	}
}
