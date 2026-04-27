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
//   - Source/Target runs (flattened from Segments)
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
		Source:       flattenSegments(sb.Source),
	}
	if b.Hash == "" && sb.Identity != nil {
		b.Hash = sb.Identity.ContentHash
	}
	if len(sb.Targets) > 0 {
		b.Targets = make(map[klf.LocaleID][]klf.Run, len(sb.Targets))
		for locale, segs := range sb.Targets {
			b.Targets[string(locale)] = flattenSegments(segs)
		}
	}
	return b
}

// fromKLF produces the minimal model.Block needed to round-trip a
// klf.Block through the ContentStore. The converter doesn't invent
// segment IDs beyond the single "s1" segment model.NewRunsBlock uses;
// that is sufficient for the overlay-at-a-time read/write pattern the
// blockstore.Store API exposes.
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

// flattenSegments concatenates the Run sequences from a segment list
// into one flat slice — the shape klf.Block uses. The segment
// boundaries inside a Bowrain Block are not preserved here; callers
// that care about segments (editor, QA) work with model.Block
// directly.
func flattenSegments(segs []*model.Segment) []model.Run {
	if len(segs) == 0 {
		return nil
	}
	n := 0
	for _, s := range segs {
		if s != nil {
			n += len(s.Runs)
		}
	}
	out := make([]model.Run, 0, n)
	for _, s := range segs {
		if s == nil {
			continue
		}
		out = append(out, s.Runs...)
	}
	return out
}
