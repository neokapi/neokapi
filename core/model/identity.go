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
// Normalization trims whitespace and lowercases the text to ensure stable hashing.
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
