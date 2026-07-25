package blockstore

import (
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
)

// toKBF produces the kbf.Block projection of a ContentStore-backed
// StoredBlock. Bowrain stores a richer model.Block internally; the
// blockstore.Store API speaks kbf.Block. The projection copies the
// fields the Store interface and its consumers read:
//
//   - ID, Translatable
//   - Hash (StoredBlock.ContentHash falling back to model.Block.Identity.Hash)
//   - Source/Target runs (the flat run sequences from the model)
//   - Type (string → kbf.BlockType)
//
// Properties, placeholders, and preview hints stay on the Bowrain
// side for now; adding them here is non-breaking and we'll do it as
// callers need them.
func toKBF(sb *platstore.StoredBlock) *kbf.Block {
	if sb == nil || sb.Block == nil {
		return nil
	}
	b := &kbf.Block{
		ID:           sb.ID,
		Hash:         sb.ContentHash,
		Translatable: sb.Translatable,
		Type:         kbf.BlockType(sb.Type),
		Source:       append([]model.Run(nil), sb.Source...),
	}
	if b.Hash == "" && sb.Identity != nil {
		b.Hash = sb.Identity.ContentHash
	}
	if len(sb.Targets) > 0 {
		b.Targets = make(map[kbf.LocaleID][]kbf.Run, len(sb.Targets))
		for key, target := range sb.Targets {
			if target == nil {
				continue
			}
			b.Targets[string(key.Locale)] = append([]model.Run(nil), target.Runs...)
		}
	}
	return b
}

// fromKBF produces the minimal model.Block needed to round-trip a
// kbf.Block through the ContentStore. The runs ride directly on the
// model's flat Source/Target run sequences — sufficient for the
// overlay-at-a-time read/write pattern the blockstore.Store API exposes.
func fromKBF(b *kbf.Block) *model.Block {
	if b == nil {
		return nil
	}
	mb := model.NewRunsBlock(b.ID, append([]model.Run(nil), b.Source...))
	mb.Translatable = b.Translatable
	mb.Type = string(b.Type)
	for locale, runs := range b.Targets {
		mb.SetTargetRuns(model.LocaleID(locale), append([]model.Run(nil), runs...))
	}
	return mb
}
