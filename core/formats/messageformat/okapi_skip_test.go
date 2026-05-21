package messageformat_test

// This file documents upstream Okapi (Java) messageformat filter tests that
// are deliberately NOT ported to the neokapi native reader/writer, with an
// honest reason for each. The neokapi native messageformat package is an
// EXTRACTION reader/writer: it parses an ICU MessageFormat pattern into
// translatable segments (one per plural/select branch) plus inline placeholder
// runs, and reconstructs the pattern on write (byte-exact via the skeleton
// store). It deliberately does NOT implement:
//
//   - ICU runtime rendering (MessageFormatParser.toFormatted): substituting
//     sample argument values and producing locale-formatted output. That is a
//     formatter, not an extraction filter.
//   - Locale-aware CLDR plural-form expansion/pruning (addPluralForms /
//     removePluralForms): adding or removing plural branches to match a target
//     locale's CLDR plural categories.
//   - The CLDR PluralRules diff utility (PluralRulesUtil.diffPluralRules).
//   - Message normalization (MessageFormatParser.normalize): hoisting
//     select/plural to the outermost level and distributing surrounding text
//     into each branch.
//   - Subfilter wiring (messageformat invoked inside JSON/YAML) plus the
//     normalization that the Okapi subfilter applies; the native reader is fed
//     the inline pattern directly and extracts branches as-is.
//   - The inline HTML/code finder (Parameters.setUseCodeFinder) that turns
//     literal markup like <b>…</b> into inline codes; the native reader keeps
//     such markup as literal text.
//
// Skip markers below are standalone documentation scanned by
// scripts/contract-audit; they intentionally have no Go test bodies.

// --- MessageFormatToFormattedTest: ICU runtime rendering (toFormatted) ---
// Every test below asserts the output of MessageFormatParser.toFormatted(locale),
// which substitutes sample argument values (count=3, name="Foo", etc.) and
// renders locale-formatted text. The native reader is extraction-only and has
// no ICU message renderer, so this observable behavior is not applicable.
//
// okapi-skip: MessageFormatToFormattedTest#testSkipSyntaxApostrophe — toFormatted() ICU rendering; native reader is extraction-only, no message renderer
// okapi-skip: MessageFormatToFormattedTest#testSkipSyntaxEmbedded — toFormatted() ICU rendering; native reader is extraction-only, no message renderer
// okapi-skip: MessageFormatToFormattedTest#testSkipSyntaxEscapedBraces — toFormatted() ICU rendering; native reader is extraction-only, no message renderer
// okapi-skip: MessageFormatToFormattedTest#testSkipSyntaxQuotedText — toFormatted() ICU rendering; native reader is extraction-only, no message renderer
// okapi-skip: MessageFormatToFormattedTest#testSkipSyntaxComplexPattern — toFormatted() ICU rendering; native reader is extraction-only, no message renderer
// okapi-skip: MessageFormatToFormattedTest#testOffset — toFormatted() renders plural with offset+sample count; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testRussian — toFormatted() renders Russian plural form for a sample count; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithPlural — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithSimpleMessage — toFormatted() substitutes {name}; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithPluralMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithNestedMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithComplexGenderMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithComplexPluralMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithNestedGenderAndPluralMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithMixedGenderAndPluralMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithEmbeddedPluralMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testSelectMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testSelectOrdinalMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testShortPluralMessage — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithNumber — toFormatted() renders {count, number}; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testWithSelect — toFormatted() ICU rendering; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testCurrency — toFormatted() renders {price, number, currency} via ICU; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testPercent — toFormatted() renders {discount, number, percent} via ICU; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testSpellout — toFormatted() renders {place, spellout} via ICU; native reader is extraction-only
// okapi-skip: MessageFormatToFormattedTest#testDuration — toFormatted() renders {duration, duration} via ICU; native reader is extraction-only

// --- MessageFormatPluralTest: locale-aware CLDR plural-form expansion/pruning ---
// Every test below calls addPluralForms()/removePluralForms() and asserts the
// re-serialized pattern. This adds or removes plural branches to match a target
// locale's CLDR plural categories (e.g. EN→RU adds few/many). The native
// reader/writer has no CLDR plural engine and does not rewrite branch sets.
//
// okapi-skip: MessageFormatPluralTest#testWithPluralMessage2 — addPluralForms() CLDR expansion (EN→AR); native reader/writer has no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testWithSelectMessage — addPluralForms() CLDR expansion; native reader/writer has no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddPluralForms_Cardinal — addPluralForms() CLDR cardinal expansion; native reader/writer has no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddMultiplePluralForms — addPluralForms() CLDR expansion across nested plurals; no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddPluralForms_Ordinal — addPluralForms() CLDR ordinal pruning; native reader/writer has no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddPluralForms_OrdinalAndPlural — addPluralForms() CLDR plural+ordinal rewrite; no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddPluralForms_Ordinal2 — addPluralForms() CLDR ordinal pruning; native reader/writer has no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddPluralForms_Ordinal3 — addPluralForms() CLDR selectordinal rewrite; native reader/writer has no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddPluralForms_Gender — addPluralForms() on a gender select (no-op via CLDR); no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddPluralForms_Mixed — addPluralForms() CLDR expansion in mixed select/plural; no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddPluralForms_MixedDeep — addPluralForms() CLDR expansion in deeply nested plurals; no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testAddPluralForms_MixedNested — addPluralForms() CLDR expansion in nested select/plural/selectordinal; no CLDR plural engine
// okapi-skip: MessageFormatPluralTest#testRemovePluralForms — removePluralForms() CLDR pruning (AR→EN); native reader/writer has no CLDR plural engine

// --- PluralRulesDiffTest: CLDR PluralRules diff utility ---
// These test PluralRulesUtil.diffPluralRules(), a CLDR-data utility computing
// the plural-category difference between two locales. The native reader has no
// CLDR plural-rules engine.
//
// okapi-skip: PluralRulesDiffTest#testRomanian — PluralRulesUtil.diffPluralRules() CLDR utility; native reader has no CLDR plural-rules engine
// okapi-skip: PluralRulesDiffTest#testSpanish — PluralRulesUtil.diffPluralRules() CLDR utility; native reader has no CLDR plural-rules engine
// okapi-skip: PluralRulesDiffTest#testArabic — PluralRulesUtil.diffPluralRules() CLDR utility; native reader has no CLDR plural-rules engine
// okapi-skip: PluralRulesDiffTest#testRussian — PluralRulesUtil.diffPluralRules() CLDR utility; native reader has no CLDR plural-rules engine
// okapi-skip: PluralRulesDiffTest#testGerman — PluralRulesUtil.diffPluralRules() CLDR utility; native reader has no CLDR plural-rules engine

// --- MessageFormatNormalizerTest: message normalization ---
// normalize() hoists select/plural to the outermost level and distributes
// surrounding text into each branch. The native reader extracts branches as-is
// and never restructures the message.
//
// okapi-skip: MessageFormatNormalizerTest#testNormalize — normalize() hoists/distributes select/plural; native reader extracts branches as-is, no normalization

// --- MessageFormatFilterTest: subfilter normalization + inline code finder ---
// The subfilter JSON/YAML tests assert the messageformat subfilter output AFTER
// normalization (surrounding text "Text … {foo} {0} End" distributed into each
// select branch). The native standalone reader is fed the inline pattern and
// does not normalize, so it cannot reproduce the distributed output.
//
// okapi-skip: MessageFormatFilterTest#testMessageFormatSubfilterJson — subfilter + normalization (text distributed into branches); native reader fed inline, no normalization
// okapi-skip: MessageFormatFilterTest#testMessageFormatSubfilterYaml — subfilter + normalization (text distributed into branches); native reader fed inline, no normalization
//
// testInlineCodes enables UseCodeFinder so literal <b>…</b> markup becomes
// inline codes within each branch. The native reader has no code finder and
// keeps such markup as literal segment text.
//
// okapi-skip: MessageFormatFilterTest#testInlineCodes — UseCodeFinder/inline HTML code detection not implemented; native reader keeps markup as literal text

// --- Integration-test (Failsafe) contracts: RoundTrip{JSON,YAML}MessageFormatIT ---
// Both IT classes (roundtrip.integration) run the okf_json / okf_yaml filters
// over the /messageformat corpus, i.e. they exercise ICU MessageFormat as a
// SUBFILTER embedded inside JSON/YAML container documents, with the Okapi
// subfilter normalization applied (see the subfilter skips above). The native
// messageformat package is a standalone .mf extraction reader/writer: it parses
// an inline ICU pattern and reconstructs it byte-exact (verified by
// skeleton_test.go TestSkeletonStore_ByteExact_* and reader_test.go TestRoundTrip),
// but it has no JSON/YAML container reader and no subfilter normalization, so it
// cannot verify the container-driven corpus roundtrip these ITs assert.
//
// okapi-skip: RoundTripJSONMessageFormatIT#messageFiles — okf_json subfilter over the /messageformat JSON corpus (container roundtrip with subfilter normalization); native messageformat reader is standalone .mf, has no JSON container or subfilter wiring
// okapi-skip: RoundTripJSONMessageFormatIT#messageSerializedFiles — okf_json subfilter corpus plus Okapi serialized-skeleton variant; container/subfilter wiring + serialized event format, both unavailable to the standalone native reader
// okapi-skip: RoundTripYAMLMessageFormatIT#messageFiles — okf_yaml subfilter over the /messageformat YAML corpus (container roundtrip with subfilter normalization); native messageformat reader is standalone .mf, has no YAML container or subfilter wiring
// okapi-skip: RoundTripYAMLMessageFormatIT#messageSerializedFiles — okf_yaml subfilter corpus plus Okapi serialized-skeleton variant; container/subfilter wiring + serialized event format, both unavailable to the standalone native reader
