package review

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// MatchOf renders a content memory match as the history layer carries it: the
// score on the percent scale, rounded half up, with the wording on both sides.
// Every assembler converts through this one function, so a 0.915 reads as 92
// on each surface rather than as 92 on one and 91 on the other.
//
// A match whose entry holds no wording for the target locale answers nil: a
// percentage with nothing behind it tells a reviewer that something close
// exists and never what it says.
func MatchOf(m memory.Match, source, target model.LocaleID) *MemoryMatch {
	answer := m.Entry.VariantText(target)
	if answer == "" {
		return nil
	}
	return &MemoryMatch{
		Score:  int(m.Score*100 + 0.5),
		Kind:   string(m.MatchType),
		Source: m.Entry.VariantText(source),
		Target: answer,
	}
}

// GoverningFingerprint is the fingerprint of the context the unit's current
// target was produced under: what a prior version is judged governed against.
//
// The producer stamps it on the target's origin (model.Origin.ContextFingerprint,
// the same value the translate prompt gates its own prior version on). The
// format's stamp wins where the format carries one, because it describes the
// bytes on disk; recorded is the state record's origin, which describes what
// was last written through kapi, and answers when the format keeps no
// provenance. Empty when neither carries a stamp, which reads as ungoverned.
func GoverningFingerprint(b *model.Block, loc model.LocaleID, recorded model.Origin) string {
	if b != nil {
		if t := b.Target(loc); t != nil && t.Origin.ContextFingerprint != "" {
			return t.Origin.ContextFingerprint
		}
	}
	return recorded.ContextFingerprint
}

// PriorVersionOf reads the answer immediately before this one from the version
// chain, keyed on the block's chain identity (never its key: a chain links
// successive answers across edits, and an edit moves the key).
//
// Ungated, unlike the prompt's. A prompt withholds a prior version approved
// under superseded rules because it would anchor the model to wording those
// rules now reject, while a reviewer reading history is the party who judges
// whether the rules moved. Governed says which it is: fingerprint is the
// governing context the current target was produced under
// (GoverningFingerprint), and an empty one reports the version as ungoverned,
// because nothing vouches for it.
func PriorVersionOf(
	ctx context.Context,
	vr memory.VersionReader,
	b *model.Block,
	source, target model.LocaleID,
	fingerprint string,
) *PriorVersion {
	if vr == nil || b == nil {
		return nil
	}
	chain := b.ChainUnit()
	if chain == "" {
		return nil
	}
	versions, err := vr.Versions(ctx, memory.VersionQuery{Unit: chain, Limit: 1}, "")
	if err != nil || len(versions) == 0 {
		return nil
	}
	v := versions[0]
	prior := &PriorVersion{
		Source:             v.Entry.VariantText(source),
		Target:             v.Entry.VariantText(target),
		ContextFingerprint: v.ContextFingerprint,
	}
	if prior.Source == "" || prior.Target == "" {
		return nil
	}
	prior.Governed = v.GovernedBy(fingerprint)
	return prior
}
