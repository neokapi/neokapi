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
	// every <trans-unit>, regardless of whether the source had a
	// <target> element. **Rarely useful** — okapi's actual behavior is
	// the conditional one captured by StripApprovedWhenNoSourceTarget.
	// Kept here for completeness in case a fixture surfaces requiring
	// blanket stripping.
	//
	// Spec basis: XLIFF 1.2 §2.4.7 defines approved as a valid
	// attribute on trans-unit (yes|no). **Spec-divergent**: dropping
	// spec-defined data unconditionally is more aggressive than okapi.
	StripTransUnitApprovedAttr bool

	// StripApprovedWhenNoSourceTarget drops the approved="…" attribute
	// from `<trans-unit>` start tags whose source did NOT contain a
	// `<target>` element. This mirrors okapi's actual behavior:
	// XLIFFFilter.java:2475 only sets the APPROVED target-property
	// when the target-processing branch runs (i.e. when a `<target>`
	// is present in the source); XLIFFSkeletonWriter.java:756 then
	// emits no `approved="…"` when the property is absent.
	//
	// Trans-units that had a `<target>` in the source keep their
	// `approved` attribute on round-trip — okapi-correct.
	//
	// Spec basis: XLIFF 1.2 §2.4.7 defines approved on trans-unit.
	// okapi's "drop when no source target" rule is **spec-divergent**
	// (the attribute is meaningful regardless of whether a target
	// element exists), but we replicate it for parity. neokapi's
	// default writer preserves the attribute as-is.
	//
	// Fixture: SF-12-Test03.xlf (944 trans-units have approved="no";
	// only TU id="1" has both source AND target → keeps approved on
	// round-trip; the other 943 have only source → approved stripped).
	StripApprovedWhenNoSourceTarget bool

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

	// StripAltTransSegSource removes any `<seg-source>…</seg-source>`
	// wrapper inside an `<alt-trans>` element on round-trip. okapi's
	// XLIFFFilter treats alt-trans as a translation-memory match — a
	// flat (source, target) pair with no segmentation envelope — so
	// when the source happened to authoring-time include a seg-source
	// inside alt-trans (legal per the schema, rare in practice), the
	// writer drops it. The accompanying alt-trans `<target>` is also
	// re-emitted in unwrapped form by okapi; this flag intentionally
	// does NOT touch the target so existing target shapes round-trip.
	//
	// Spec basis: XLIFF 1.2 §2.5 allows seg-source inside alt-trans;
	// dropping it is **spec-divergent** but matches okapi.
	//
	// Fixture: segmentation2.xlf trans-unit id=3 alt-trans contains a
	// seg-source — okapi strips it on write while preserving the
	// surrounding source/target.
	StripAltTransSegSource bool

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

	// EscapeBeyondLatin1AsEntities turns on encoder-aware entity
	// escaping in text bodies — chars the source-declared encoding
	// cannot represent are emitted as `&#xNNNN;` numeric references,
	// other chars stay literal. ONLY active when the source declared a
	// non-UTF-8 encoding (windows-1252, ISO-8859-1, …). For UTF-8
	// sources (the common case) this flag is a no-op.
	//
	// Mirrors okapi XMLEncoder.setOptions + _encode (XMLEncoder.java:
	// 101-110, 191-213): the encoder is only constructed when the
	// output encoding is non-UTF-8/16, and the per-char check
	// `!chsEnc.canEncode(value)` decides whether to escape. We use
	// `golang.org/x/text/encoding`'s Encoder.canEncode for an exact
	// match — windows-1252's "Windows extension" chars in U+0152-U+2122
	// (e.g. U+0192 ƒ, U+2026 …, U+20AC €) stay literal, while Latin
	// Extended-A/B beyond Latin-1 gets escaped.
	//
	// The writer reads the source encoding from the layer's
	// `xliff:source-encoding` property (set by the reader when the XML
	// declaration named a non-UTF-8 charset). If that property is
	// missing the flag is a no-op. The flag name uses "Latin1" because
	// that's the typical practical effect, but the actual rule is
	// encoder-driven.
	//
	// Spec basis: XML 1.0 §2.2 — numeric entities are equivalent to
	// the chars they reference. Both forms are XML-valid. **Spec-
	// tolerant**: purely a stylistic encoding choice; UTF-8 is more
	// readable and 4× more compact for Latin-extended chars.
	//
	// Fixture: SF-12-Test03.xlf (declared windows-1252; pseudo-output
	// `Ţàĉƒ` → okapi emits `&#x0162;à&#x0109;ƒ` keeping ƒ literal because
	// it's in windows-1252; neokapi default emits all four literal).
	EscapeBeyondLatin1AsEntities bool

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
