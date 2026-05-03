package xliff

// OkapiCompatConfig is an opt-in bag of writer behaviors that mimic the
// Okapi Framework XLIFFFilter / XLIFFWriter (Java reference engine) on
// round-trip. Each flag exists so the parity test harness can drive
// neokapi's xliff writer to byte-equivalent output against the okapi
// reference — even where okapi's behavior is debatable, spec-divergent,
// or outright buggy. **None of these flags are on by default.** neokapi's
// default xliff writer follows the XLIFF 1.2 spec and intuitive,
// least-surprise output choices.
//
// When adding a new flag here, document:
//   - what okapi does (the observable byte-level behavior)
//   - the fixture that exercises it
//   - the relevant XLIFF 1.2 spec section
//   - whether matching okapi is spec-aligned, spec-tolerant, or
//     spec-divergent
//   - a link/path reference to the okapi source (when known) so future
//     readers can verify the behavior didn't change upstream
//
// The longer-term plan is to investigate the most questionable flags
// (note hoisting, attribute stripping, encoding bugs) and decide
// whether each one stays opt-in forever or gets retired once okapi
// fixes its end. Tracked in docs/internals/research/xliff-okapi-compat-quirks.md.
type OkapiCompatConfig struct {
	// LowercaseLangSubtag emits language subtags in canonical BCP-47
	// lowercase form ("de" not "DE") on synthesized <target xml:lang="…">
	// elements. The okapi writer applies a BCP-47 normalizer to xml:lang
	// values that originate from a target-language declaration.
	//
	// Spec basis: BCP-47 / RFC 5646 §2.1.1 — language subtag canonical
	// form is lowercase. xml:lang values per W3C XML 1.0 §2.12 are
	// case-insensitive but the recommended canonical form follows
	// BCP-47. So this is **spec-aligned** and a defensible default —
	// it's only opt-in here because changing default writer behavior
	// is out of scope for the parity work.
	//
	// Fixture: Typo3Draft.xlf (file declares target-language="DE";
	// okapi emits xml:lang="de" on synthesized targets).
	//
	// Okapi source: net.sf.okapi.common.LocaleId stores locale subtags
	// already lowercased; the writer reads from that internal form.
	LowercaseLangSubtag bool

	// UnwrapSingleSegMrk strips the <seg-source>…</seg-source> wrapper
	// and the corresponding <mrk mtype="seg">…</mrk> wrappers in the
	// <target> when the trans-unit has exactly one segment. The mrks
	// add no information in the single-segment case — they only mark
	// segmentation boundaries that don't exist.
	//
	// Spec basis: XLIFF 1.2 §2.4.1 — <seg-source> is **optional** and a
	// trans-unit without it is unsegmented. Single-mrk segmentation is
	// semantically equivalent to no segmentation. **Spec-tolerant**.
	//
	// Fixture: about_the.htm.xlf (every trans-unit was segmented into
	// one mrk by the upstream tool; okapi unwraps on write so the
	// output is leaner). Also affects MQ-12-Test01.xlf and
	// Test_Context_and_PH.xlf.
	UnwrapSingleSegMrk bool

	// StripTransUnitApprovedAttr drops the approved="…" attribute from
	// <trans-unit>. okapi's writer does not preserve this attribute
	// even though the spec defines it; the assumption appears to be
	// that the active translation workflow tracks approval state
	// separately and re-emitting the source's value is misleading.
	//
	// Spec basis: XLIFF 1.2 §2.4.7 defines approved as a valid
	// attribute on trans-unit (yes|no). **Spec-divergent**: okapi is
	// dropping spec-defined data. We're more spec-faithful by default;
	// this flag exists only for parity comparison.
	//
	// Fixtures: SF-12-Test03.xlf (every trans-unit has approved="no").
	StripTransUnitApprovedAttr bool

	// StripPhaseDateAttr drops the date="…" attribute from <phase>
	// elements inside the <header><phase-group>. okapi's writer
	// discards this attribute — likely because it would otherwise
	// require the writer to update the date (or stamp it as "now") and
	// preserving the source date is misleading once the file has been
	// processed.
	//
	// Spec basis: XLIFF 1.2 §2.3.1 defines date as a valid attribute
	// on phase. **Spec-divergent**: okapi is dropping spec-defined
	// data. Same reasoning as StripTransUnitApprovedAttr.
	//
	// Fixture: altTrans-100.xlf (phase has date="2011-01-13T12:00:19Z").
	StripPhaseDateAttr bool

	// StripCDataCREntities removes &#xD; (carriage return) numeric
	// character references from text content. okapi's writer does not
	// preserve these on round-trip — appears to be a side effect of
	// the way okapi's TextFragment normalizes line endings to LF
	// internally and never re-emits CR.
	//
	// Spec basis: XML 1.0 §2.11 — &#xD; inside element content is
	// **significant data** that must be preserved. Stripping changes
	// the content. **Spec-divergent**: okapi is silently corrupting
	// data on round-trip. We're more spec-faithful by default.
	//
	// Fixture: altTrans-100.xlf (contains 21 &#xD; entities inside
	// <markup-seg> CDATA-style escaped HTML).
	StripCDataCREntities bool

	// HoistAltTransNotes pulls <note> elements out of <alt-trans> and
	// emits them at the trans-unit level alongside any trans-unit-level
	// notes. okapi's reader collects notes into a single trans-unit
	// property bag without recording where each note came from, so the
	// writer emits them all together at the trans-unit level.
	//
	// Spec basis: XLIFF 1.2 §2.5 — <note> can appear inside many
	// elements including <alt-trans>; the placement is meaningful (a
	// note inside alt-trans is *about* that alternate translation, not
	// the trans-unit). **Spec-divergent**: okapi loses the contextual
	// association on round-trip.
	//
	// Fixture: Test_Context_and_PH.xlf (alt-trans has its own note
	// "alt trans note 1"; trans-unit has note "trans unit note 1";
	// okapi emits both at trans-unit level before <alt-trans>).
	HoistAltTransNotes bool

	// EscapeNonASCIIAsEntities emits non-ASCII characters as XML
	// numeric character references (`&#xNNNN;`) rather than as their
	// UTF-8 byte sequences. okapi's writer does this for all chars
	// above U+007F, regardless of the file's declared encoding.
	//
	// Spec basis: XML 1.0 §2.2 — numeric entities are equivalent to
	// the chars they reference. Both forms are XML-valid. **Spec-
	// tolerant**: this is purely a stylistic encoding choice. UTF-8
	// is more readable and 4× more compact for Latin-extended.
	//
	// Fixture: SF-12-Test03.xlf (pseudo-translates source into
	// Latin-extended chars; okapi emits `&#x015a;` etc, neokapi emits
	// `Ś`).
	EscapeNonASCIIAsEntities bool

	// SimulateBrokenWindows1252Read replaces non-ASCII bytes from a
	// declared windows-1252 input file with U+FFFD (REPLACEMENT
	// CHARACTER) instead of correctly transcoding them to UTF-8.
	// okapi's xliff filter has a bug where certain windows-1252 byte
	// sequences for accented Latin chars (E1=á, F3=ó, ED=í) end up
	// as U+FFFD in the output instead of being preserved. The exact
	// trigger is in the parser's character-data path and hasn't been
	// fully traced.
	//
	// Spec basis: there is no spec-justification for this; **it is
	// simply an okapi bug**. We preserve the chars correctly by
	// default; this flag exists only for byte-equivalent parity
	// comparison against the broken reference.
	//
	// Fixture: SF-12-Test03.xlf (declares encoding="windows-1252",
	// has bytes E1 F3 ED for á ó í).
	SimulateBrokenWindows1252Read bool

	// ReorderHeaderToolToEnd moves <tool> elements within <header> to
	// appear after <note> siblings rather than wherever the source
	// placed them. okapi's reader collects header children into typed
	// bags (notes, tools, props, etc.) and the writer emits them in a
	// fixed order, losing the source's authored order.
	//
	// Spec basis: XLIFF 1.2 §2.3 doesn't mandate strict ordering of
	// header children. **Spec-tolerant**, but a source-order-preserving
	// writer is more faithful to the original file's authoring intent.
	//
	// Fixture: Typo3Draft.xlf (source has count-group → tool → note;
	// okapi emits count-group → note → tool).
	ReorderHeaderToolToEnd bool
}
