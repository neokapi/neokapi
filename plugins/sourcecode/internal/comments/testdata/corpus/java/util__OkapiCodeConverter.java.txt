package neokapi.bridge.util;

import neokapi.bridge.model.FragmentDTO;
import neokapi.bridge.model.SpanDTO;
import net.sf.okapi.common.resource.Code;
import net.sf.okapi.common.resource.TextFragment;

import java.util.ArrayList;
import java.util.List;

/**
 * Converts between Okapi's coded text (marker + index pairs) and neokapi's
 * coded text (single marker characters with sequential spans).
 *
 * <h3>Marker character mapping:</h3>
 * <table>
 *     <tr><th></th><th>Okapi</th><th>neokapi</th></tr>
 *     <tr><td>Opening</td><td>\uE101 + index</td><td>\uE001</td></tr>
 *     <tr><td>Closing</td><td>\uE102 + index</td><td>\uE002</td></tr>
 *     <tr><td>Placeholder</td><td>\uE103 + index</td><td>\uE003</td></tr>
 * </table>
 *
 * Okapi uses marker + index pairs (2 chars per code reference).
 * neokapi uses single marker chars (spans are sequential in the Spans array).
 */
public class OkapiCodeConverter {

    // Okapi marker characters
    private static final char OKAPI_OPENING = '\uE101';
    private static final char OKAPI_CLOSING = '\uE102';
    private static final char OKAPI_PLACEHOLDER = '\uE103';

    // neokapi marker characters
    private static final char NEOKAPI_OPENING = '\uE001';
    private static final char NEOKAPI_CLOSING = '\uE002';
    private static final char NEOKAPI_PLACEHOLDER = '\uE003';

    // neokapi SpanType values
    private static final int SPAN_OPENING = 0;
    private static final int SPAN_CLOSING = 1;
    private static final int SPAN_PLACEHOLDER = 2;

    // neokapi SpanFlag values (must match model.SpanFlag* constants)
    private static final int SPAN_FLAG_HAS_REF = 1;

    /**
     * Convert an Okapi TextFragment to a neokapi FragmentDTO.
     */
    public static FragmentDTO toFragmentDTO(TextFragment tf) {
        if (tf == null) {
            return null;
        }

        FragmentDTO dto = new FragmentDTO();
        String codedText = tf.getCodedText();
        List<Code> codes = tf.getCodes();
        List<SpanDTO> spans = new ArrayList<>();
        StringBuilder neokapiText = new StringBuilder();

        int i = 0;
        while (i < codedText.length()) {
            char c = codedText.charAt(i);

            if (c == OKAPI_OPENING || c == OKAPI_CLOSING || c == OKAPI_PLACEHOLDER) {
                // Next char is the index into the codes list.
                if (i + 1 < codedText.length()) {
                    int codeIndex = TextFragment.toIndex(codedText.charAt(i + 1));
                    Code code = (codeIndex >= 0 && codeIndex < codes.size()) ? codes.get(codeIndex) : null;

                    SpanDTO span = new SpanDTO();
                    if (c == OKAPI_OPENING) {
                        neokapiText.append(NEOKAPI_OPENING);
                        span.setSpanType(SPAN_OPENING);
                    } else if (c == OKAPI_CLOSING) {
                        neokapiText.append(NEOKAPI_CLOSING);
                        span.setSpanType(SPAN_CLOSING);
                    } else {
                        neokapiText.append(NEOKAPI_PLACEHOLDER);
                        span.setSpanType(SPAN_PLACEHOLDER);
                    }

                    if (code != null) {
                        span.setId(String.valueOf(code.getId()));
                        span.setData(code.getData());
                        span.setOuterData(code.getOuterData());
                        span.setType(code.getType());
                        span.setDeletable(code.isDeleteable());
                        span.setCloneable(code.isCloneable());

                        // Enriched fields
                        String displayText = code.getDisplayText();
                        if (displayText != null && !displayText.isEmpty()) {
                            span.setDisplayText(displayText);
                        }

                        String originalId = code.getOriginalId();
                        if (originalId != null && !originalId.isEmpty()) {
                            span.setOriginalId(originalId);
                        }

                        // Flags
                        int flags = 0;
                        if (code.hasReference()) {
                            flags |= SPAN_FLAG_HAS_REF;
                        }
                        if (flags != 0) {
                            span.setFlags(flags);
                        }
                    }

                    spans.add(span);
                    i += 2; // Skip marker + index
                } else {
                    // Malformed: marker without index. Append as text.
                    neokapiText.append(c);
                    i++;
                }
            } else {
                neokapiText.append(c);
                i++;
            }
        }

        dto.setCodedText(neokapiText.toString());
        dto.setSpans(spans.isEmpty() ? null : spans);
        return dto;
    }

    /**
     * Convert a neokapi FragmentDTO to an Okapi TextFragment.
     *
     * <p>Equivalent to {@link #toTextFragment(FragmentDTO, TextFragment)} with a
     * null source — used when there's no source TextFragment to align against
     * (e.g. translations submitted standalone, not as part of a round-trip).</p>
     */
    public static TextFragment toTextFragment(FragmentDTO dto) {
        return toTextFragment(dto, null);
    }

    /**
     * Convert a neokapi FragmentDTO to an Okapi TextFragment, hydrating
     * Okapi-internal Code metadata from the source TextFragment when
     * available.
     *
     * <p><b>Why this exists.</b> Okapi's {@link Code} carries three fields
     * that the proto Run schema doesn't model — {@code outerData},
     * {@code originalId}, and {@code referenceFlag}. Many writers
     * (HtmlSkeletonWriter, OpenXML, IDML, MIF, …) re-emit a code by looking
     * up its {@code originalId} or by emitting its {@code outerData} verbatim;
     * if those fields are null the writer drops the code from the output.</p>
     *
     * <p><b>What we do.</b> The bridge daemon's pipeline keeps the original
     * Event in-process from reader to writer. By the time {@code applyTranslations}
     * is called, the source TextFragment still holds the full original Codes —
     * those Okapi-internal fields never had to cross the wire. For each
     * incoming SpanDTO we look up a matching source Code by {@code (id, tagType)},
     * clone it, then overlay the wire-carried fields ({@code data}, {@code type},
     * {@code equiv}, {@code disp}, {@code constraints}, {@code id}) from the
     * SpanDTO — Go is the authority for things that did cross the wire, source
     * is the authority for things that didn't.</p>
     *
     * <p><b>What it handles.</b></p>
     * <ul>
     *   <li>Pseudo-translation (Go preserves codes structurally) — exact source-code reuse.</li>
     *   <li>Real translation that mutates {@code data} (e.g. {@code alt="hello"} → {@code alt="hola"})
     *       — source metadata preserved, Go's {@code data} change respected.</li>
     *   <li>Go reorders codes — lookup is by id, not position.</li>
     *   <li>Go drops a code — just doesn't appear in target.</li>
     *   <li>Go inserts a brand-new code (id not in source) — built from SpanDTO with no
     *       source-side metadata (unavoidable: the bridge has nothing to hydrate from).</li>
     * </ul>
     *
     * @param dto    the FragmentDTO carrying the rewritten target content
     * @param source the source TextFragment from the same TextUnit, or null
     *               when no source is available
     */
    public static TextFragment toTextFragment(FragmentDTO dto, TextFragment source) {
        return toTextFragment(dto, source, null);
    }

    /**
     * Convert a neokapi FragmentDTO to an Okapi TextFragment, hydrating
     * Okapi-internal Code metadata from {@code existingTarget} first, then
     * {@code source}.
     *
     * <p>Why prefer existing-target codes. XLIFF allows target codes to have
     * IDs that differ from source codes (e.g. source has {@code <x id="x1"/>},
     * target has {@code <x id="x3"/>}). Okapi's filter normalises both to
     * sequential integer ids and stashes the original {@code "x1"} /
     * {@code "x3"} on the {@code Code.originalId}. The wire protocol carries
     * only the integer id, so on round-trip the only way to recover the
     * target's original {@code "x3"} is to look it up in the existing target's
     * Code list. Falling back to source codes loses the distinction —
     * ImplementationPlan.docx.xlf surfaced this as the bridge writing
     * {@code <x id="x1"/>} where the okapi reference had {@code <x id="x3"/>}.</p>
     */
    public static TextFragment toTextFragment(FragmentDTO dto, TextFragment source, TextFragment existingTarget) {
        if (dto == null || dto.getCodedText() == null) {
            return new TextFragment();
        }

        String neokapiText = dto.getCodedText();
        List<SpanDTO> spans = dto.getSpans();
        int spanIndex = 0;

        // Track which existing-target / source Codes we've already consumed
        // so two target spans with the same (id, tagType) — e.g. a Go-cloned
        // pair — don't both hydrate from the same slot.
        List<Code> existingTargetCodes = (existingTarget != null) ? existingTarget.getCodes() : null;
        boolean[] existingTargetUsed = (existingTargetCodes != null) ? new boolean[existingTargetCodes.size()] : null;
        List<Code> sourceCodes = (source != null) ? source.getCodes() : null;
        boolean[] sourceUsed = (sourceCodes != null) ? new boolean[sourceCodes.size()] : null;

        StringBuilder okapiText = new StringBuilder();
        List<Code> codes = new ArrayList<>();

        for (int i = 0; i < neokapiText.length(); i++) {
            char c = neokapiText.charAt(i);

            if (c != NEOKAPI_OPENING && c != NEOKAPI_CLOSING && c != NEOKAPI_PLACEHOLDER) {
                okapiText.append(c);
                continue;
            }

            SpanDTO span = (spans != null && spanIndex < spans.size()) ? spans.get(spanIndex) : null;
            spanIndex++;

            TextFragment.TagType tagType;
            char okapiMarker;
            if (c == NEOKAPI_OPENING) {
                tagType = TextFragment.TagType.OPENING;
                okapiMarker = OKAPI_OPENING;
            } else if (c == NEOKAPI_CLOSING) {
                tagType = TextFragment.TagType.CLOSING;
                okapiMarker = OKAPI_CLOSING;
            } else {
                tagType = TextFragment.TagType.PLACEHOLDER;
                okapiMarker = OKAPI_PLACEHOLDER;
            }

            int spanId = parseSpanId(span);
            // Prefer existing-target codes over source codes — the target
            // may carry an originalId distinct from the source's (XLIFF
            // allows target <x id="x3"/> while source has <x id="x1"/>).
            Code match = findUnusedSourceCode(existingTargetCodes, existingTargetUsed, spanId, tagType);
            if (match == null) {
                match = findUnusedSourceCode(sourceCodes, sourceUsed, spanId, tagType);
            }
            Code code = (match != null)
                    ? hydrateFromSource(match, span, spanId)
                    : buildFreshCode(span, tagType);

            int idx = codes.size();
            codes.add(code);
            okapiText.append(okapiMarker);
            okapiText.append(TextFragment.toChar(idx));
        }

        TextFragment tf = new TextFragment();
        tf.setCodedText(okapiText.toString(), codes);
        return tf;
    }

    /**
     * Find the first unused source Code matching this span. Prefers an exact
     * {@code (id, tagType)} match; falls back to id-only when no tagType
     * match exists.
     *
     * <p>The id-only fallback handles Okapi filters whose codedText marker
     * disagrees with the {@code Code.tagType} — e.g. {@code MarkdownFilter}
     * emits a {@code Code} with {@code tagType=CLOSING} for the trailing
     * {@code ]} of a {@code [text][ref]} construct, but writes a PLACEHOLDER
     * marker ({@code }) for it in the codedText. The bridge's
     * outbound conversion uses the codedText marker to set spanType
     * (PLACEHOLDER), so on the way back the (id, PLACEHOLDER) match misses
     * the (id, CLOSING) source code. Falling back to id-only recovers the
     * original Code with its full metadata, including the data the writer
     * needs to emit.</p>
     *
     * <p>Marks the chosen source slot used so a later span with the same id
     * doesn't double-hydrate from it.</p>
     */
    private static Code findUnusedSourceCode(List<Code> sourceCodes, boolean[] used,
                                             int spanId, TextFragment.TagType tagType) {
        if (sourceCodes == null) {
            return null;
        }
        int idOnlyMatch = -1;
        for (int i = 0; i < sourceCodes.size(); i++) {
            if (used[i]) {
                continue;
            }
            Code candidate = sourceCodes.get(i);
            if (candidate.getId() != spanId) {
                continue;
            }
            if (candidate.getTagType() == tagType) {
                used[i] = true;
                return candidate;
            }
            if (idOnlyMatch == -1) {
                idOnlyMatch = i;
            }
        }
        if (idOnlyMatch >= 0) {
            used[idOnlyMatch] = true;
            return sourceCodes.get(idOnlyMatch);
        }
        return null;
    }

    /**
     * Clone a source Code (preserving outerData, originalId, referenceFlag,
     * and any other Okapi-internal fields) then overlay the wire-carried
     * fields from the SpanDTO. Go is the authority for fields that crossed
     * the wire; source is the authority for fields that didn't.
     *
     * <p>Note: Okapi's {@link Code#setData} resets {@code referenceFlag} as
     * a side effect, so we capture {@code hasReference} before calling it
     * and restore it afterwards (subject to a wire-side override via
     * SpanDTO flags).</p>
     */
    private static Code hydrateFromSource(Code source, SpanDTO span, int spanId) {
        Code code = source.clone();
        boolean preservedReference = code.hasReference();
        code.setId(spanId);
        if (span == null) {
            return code;
        }
        // Wire-carried fields: trust Go's value, including empty strings
        // (Go may have intentionally cleared them).
        if (span.getData() != null) {
            code.setData(span.getData()); // side effect: clears referenceFlag
        }
        if (span.getType() != null) {
            code.setType(span.getType());
        }
        code.setDeleteable(span.isDeletable());
        code.setCloneable(span.isCloneable());
        if (span.getDisplayText() != null && !span.getDisplayText().isEmpty()) {
            code.setDisplayText(span.getDisplayText());
        }
        // Wire-lossy fields (outerData, originalId, referenceFlag) are
        // already on the cloned source. SpanDTO's outerData/originalId/flags
        // are populated only on the outbound conversion (toFragmentDTO)
        // for diagnostic round-trips, not on inbound from Go — but if Go
        // did surface them back, honour the override.
        if (span.getOuterData() != null) {
            code.setOuterData(span.getOuterData());
        }
        if (span.getOriginalId() != null && !span.getOriginalId().isEmpty()) {
            code.setOriginalId(span.getOriginalId());
        }
        boolean spanHasRef = (span.getFlags() & SPAN_FLAG_HAS_REF) != 0;
        if (preservedReference || spanHasRef) {
            code.setReferenceFlag(true);
        }
        return code;
    }

    /**
     * Build a Code purely from a SpanDTO when no source-side match exists
     * (Go inserted a code the source didn't have). Wire-lossy fields stay
     * unset — the bridge has no source to hydrate from.
     *
     * <p>Note: Okapi's two-arg {@code Code(TagType, String)} constructor
     * takes {@code (tagType, type)}, NOT {@code (tagType, data)} — the
     * second arg is the named-type label and {@code data} is initialized to
     * empty. Use the three-arg form to set data correctly the first time.</p>
     */
    private static Code buildFreshCode(SpanDTO span, TextFragment.TagType tagType) {
        String codeData = (span != null && span.getData() != null) ? span.getData() : "";
        String codeType = (span != null && span.getType() != null) ? span.getType() : "";
        Code code = new Code(tagType, codeType, codeData);
        if (span == null) {
            return code;
        }
        code.setId(parseSpanId(span));
        if (span.getOuterData() != null) {
            code.setOuterData(span.getOuterData());
        }
        code.setDeleteable(span.isDeletable());
        code.setCloneable(span.isCloneable());
        if (span.getDisplayText() != null && !span.getDisplayText().isEmpty()) {
            code.setDisplayText(span.getDisplayText());
        }
        if (span.getOriginalId() != null && !span.getOriginalId().isEmpty()) {
            code.setOriginalId(span.getOriginalId());
        }
        if ((span.getFlags() & SPAN_FLAG_HAS_REF) != 0) {
            code.setReferenceFlag(true);
        }
        return code;
    }

    private static int parseSpanId(SpanDTO span) {
        if (span == null || span.getId() == null || span.getId().isEmpty()) {
            return 0;
        }
        try {
            return Integer.parseInt(span.getId());
        } catch (NumberFormatException ignored) {
            return 0;
        }
    }

    /**
     * Check if a character is a neokapi marker.
     */
    public static boolean isGokapiMarker(char c) {
        return c == NEOKAPI_OPENING || c == NEOKAPI_CLOSING || c == NEOKAPI_PLACEHOLDER;
    }
}
