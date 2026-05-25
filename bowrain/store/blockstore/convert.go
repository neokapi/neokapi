package blockstore

import (
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
)

// toKLF produces the klf.Block projection of a ContentStore-backed
// StoredBlock. Bowrain stores a richer model.Block internally; the
// blockstore.Store API speaks klf.Block. The projection copies the
// fields the Store interface and its consumers read:
//
//   - ID, Translatable
//   - Hash (StoredBlock.ContentHash falling back to model.Block.Identity.Hash)
//   - Source/Target runs (the flat run sequences from the model)
//   - Type (string → klf.BlockType)
//
// Properties, placeholders, and preview hints stay on the Bowrain
// side for now; adding them here is non-breaking and we'll do it as
// callers need them.
func toKLF(sb *platstore.StoredBlock) *klf.Block {
	if sb == nil || sb.Block == nil {
		return nil
	}
	b := &klf.Block{
		ID:           sb.ID,
		Hash:         sb.ContentHash,
		Translatable: sb.Translatable,
		Type:         klf.BlockType(sb.Type),
		Source:       append([]model.Run(nil), sb.Source...),
	}
	if b.Hash == "" && sb.Identity != nil {
		b.Hash = sb.Identity.ContentHash
	}
	if len(sb.Targets) > 0 {
		b.Targets = make(map[klf.LocaleID][]klf.Run, len(sb.Targets))
		for key, target := range sb.Targets {
			if target == nil {
				continue
			}
			b.Targets[string(key.Locale)] = append([]model.Run(nil), target.Runs...)
		}
	}
	return b
}

// fromKLF produces the minimal model.Block needed to round-trip a
// klf.Block through the ContentStore. The runs ride directly on the
// model's flat Source/Target run sequences — sufficient for the
// overlay-at-a-time read/write pattern the blockstore.Store API exposes.
func fromKLF(b *klf.Block) *model.Block {
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
