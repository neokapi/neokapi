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

// RecordHash folds both halves into the hash that decides TRANSFER: whether a
// far side already holds what this block currently is.
//
// The two halves answer different questions and neither answers this one alone.
// ContentHash says what the text is, and is the identity a decision, a memory
// entry and a block key are all filed under — it must never move for a reason
// other than the text moving. But a block is stored with more than its text: its
// name, its type, and the properties a reader recorded about it. A reader that
// starts recording something new — a message key, a role, a note — leaves the
// text untouched, so a transfer decision made on ContentHash alone reports the
// block unchanged and the new field never arrives. Not on the next push: never,
// because the text is the same forever.
//
// Folding the context half in is what makes an ordinary push deliver it, per
// block, with nobody declaring a version or remembering to force anything. The
// cost this avoids — a locator like a line number shifting and re-uploading a
// file's whole tail — is paid for by AdvisoryPropertyPrefix, which keeps derived
// locators out of the context hash in the first place.
//
// A field this hash cannot see is a field that never reaches content already
// stored. So anything a block is persisted with belongs in one half or the
// other, and anything computed rather than read belongs behind the advisory
// prefix.
func (i *BlockIdentity) RecordHash() string {
	if i == nil {
		return ""
	}
	return ComputeRecordHash(i.ContentHash, i.ContextHash)
}

// ComputeRecordHash folds a content hash and a context hash into the transfer
// hash — see BlockIdentity.RecordHash. Takes the halves rather than a block so
// a store that already holds both as columns folds them without rehashing.
func ComputeRecordHash(contentHash, contextHash string) string {
	h := sha256.New()
	h.Write([]byte(contentHash))
	h.Write([]byte{0})
	h.Write([]byte(contextHash))
	return hex.EncodeToString(h.Sum(nil))
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

// AdvisoryPropertyPrefix marks a property as carried but not identifying.
//
// Most properties are structural — a heading's level, a cell's column — and
// belong in the context hash: change one and the block genuinely sits somewhere
// else. But some are DERIVED LOCATORS: the line a block starts on, a byte
// offset, anything that says "where to find this" rather than "what this is".
//
// Those must not identify a block. A locator moves whenever anything above it
// moves, so folding it into the context hash makes an untouched block report as
// moved every time a blank line is added somewhere earlier in the file — noise
// that says a block changed when nothing about it did.
//
// So a property whose key starts with this prefix is carried on the block and
// skipped when hashing. Readers use it for anything they compute rather than
// read from the document.
const AdvisoryPropertyPrefix = "@"

// IsAdvisoryProperty reports whether a property is a derived locator rather than
// part of the block's identity.
func IsAdvisoryProperty(key string) bool {
	return strings.HasPrefix(key, AdvisoryPropertyPrefix)
}

// ComputeContextHash computes a SHA-256 hash of contextual information
// (block name, type, and identifying properties) to detect structural changes.
//
// Advisory properties are excluded — see AdvisoryPropertyPrefix.
func ComputeContextHash(name, typ string, properties map[string]string) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte{0})
	h.Write([]byte(typ))
	h.Write([]byte{0})

	// Sort keys for deterministic hashing.
	keys := make([]string, 0, len(properties))
	for k := range properties {
		if IsAdvisoryProperty(k) {
			continue
		}
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
