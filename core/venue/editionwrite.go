package venue

import "github.com/neokapi/neokapi/core/ref"

// EditionWrite is what a push says about how a translation came to be: the
// last write of the edition the project's block history records. It rides
// beside the decisions and decides nothing. It carries the two facts a venue
// grades a translation by and the checkout alone holds once a run on it has
// written one: the source the translation was made from, which grades it
// stale when that source moves, and who wrote it, which separation of duties
// asks of a verdict on it.
type EditionWrite struct {
	// ItemName is the item whose durable identity namespace Unit lives in.
	ItemName string `json:"item"`
	// Unit is the durable unit identity (convergence.BlockKey), as the push
	// resolved it against the venue's.
	Unit string `json:"unit"`
	// Block is the key the project's own records name the unit by, its
	// decisions among them, where that differs from Unit: a checkout names a
	// block by its reader's key, and a push resolves it to the venue's.
	Block string `json:"block,omitempty"`
	// Variant is the locale (and optional tone/channel) in VariantKey text form.
	Variant string `json:"variant"`
	// Revision is the edition's revision the write left (model.RunsRevision).
	// A venue holding another translation of the unit takes nothing from the
	// record; one holding none takes it as the record of a translation the
	// project keeps.
	Revision string `json:"rev"`
	// Basis is the content hash of the source the translation was made from
	// (state.SourceHash), empty when the write recorded none: an edit made
	// outside kapi, or a translation a pull brought down from a venue.
	Basis string `json:"basis,omitempty"`
	// Writer is the kind of actor that wrote the translation: person, agent,
	// tool, or external for an edit made outside kapi that a read observed.
	Writer string `json:"writer,omitempty"`
	// Origin is the surface the write came through: apply, ksed, mcp,
	// browser, desktop, flow:<name>, merge, pull or observed.
	Origin string `json:"origin,omitempty"`
	// GoverningFingerprint is the governing context a tool's write was
	// produced under (model.Origin.ContextFingerprint), empty otherwise.
	GoverningFingerprint string `json:"governingFingerprint,omitempty"`
}

// The writers an edition write names.
const (
	WriterPerson   = "person"
	WriterAgent    = "agent"
	WriterTool     = "tool"
	WriterExternal = "external"
)

// The origins of an edition write that bring in somebody else's work.
const (
	// OriginPull is a translation kapi pull brought down from a venue, made
	// there from a source the checkout never recorded.
	OriginPull = "pull"
	// OriginMerge is a translation kapi merge returned or materialized.
	OriginMerge = "merge"
	// OriginObserved is an edit made outside kapi that a read observed.
	OriginObserved = "observed"
)

// ByHand reports whether the pusher wrote the translation: a person's or an
// agent's edit made through kapi in the checkout that pushes it. An agent
// works for the person who runs it, as it does on the venue. A translation a
// flow made, one a pull or a merge brought in (another person's work), and an
// edit made outside kapi are writes the push names no author for.
func (w EditionWrite) ByHand() bool {
	if w.Writer != WriterPerson && w.Writer != WriterAgent {
		return false
	}
	switch w.Origin {
	case OriginMerge, OriginPull, OriginObserved:
		return false
	}
	return true
}

// HasBasis reports whether the write names the source the translation was
// made from. A pulled translation names none, whatever the write carries: the
// venue made it, and the venue's own record holds its source.
func (w EditionWrite) HasBasis() bool {
	return w.Basis != "" && w.Origin != OriginPull
}

// Produced reports whether a tool made the translation from a recorded
// source, the write a venue counts as drafted against that source.
func (w EditionWrite) Produced() bool {
	return w.Writer == WriterTool && w.HasBasis()
}

// KnowsNoBasis reports whether the write replaced a translation with one made
// from no recorded source: an edit by hand or one made outside kapi. A record
// of what a pull or a merge brought in, with no basis, says nothing new about
// the source.
func (w EditionWrite) KnowsNoBasis() bool {
	return w.Basis == "" && (w.ByHand() || w.Writer == WriterExternal)
}

// Edition names the translation the write is about within its item: the unit
// and the variant.
func (w EditionWrite) Edition() string {
	return w.Unit + "\x00" + w.Variant
}

// Identity is what the write says, so a producer can tell whether the write it
// last sent for a translation has changed.
func (w EditionWrite) Identity() string {
	return ref.Identity(w.Block, w.Revision, w.Basis, w.Writer, w.Origin, w.GoverningFingerprint)
}
