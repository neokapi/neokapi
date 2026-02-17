package com.gokapi.bridge.util;

import com.gokapi.bridge.model.FragmentDTO;
import com.gokapi.bridge.model.SpanDTO;
import net.sf.okapi.common.resource.Code;
import net.sf.okapi.common.resource.TextFragment;

import java.util.ArrayList;
import java.util.List;

/**
 * Converts between Okapi's coded text (marker + index pairs) and gokapi's
 * coded text (single marker characters with sequential spans).
 *
 * <h3>Marker character mapping:</h3>
 * <table>
 *     <tr><th></th><th>Okapi</th><th>gokapi</th></tr>
 *     <tr><td>Opening</td><td>\uE101 + index</td><td>\uE001</td></tr>
 *     <tr><td>Closing</td><td>\uE102 + index</td><td>\uE002</td></tr>
 *     <tr><td>Placeholder</td><td>\uE103 + index</td><td>\uE003</td></tr>
 * </table>
 *
 * Okapi uses marker + index pairs (2 chars per code reference).
 * gokapi uses single marker chars (spans are sequential in the Spans array).
 */
public class OkapiCodeConverter {

    // Okapi marker characters
    private static final char OKAPI_OPENING = '\uE101';
    private static final char OKAPI_CLOSING = '\uE102';
    private static final char OKAPI_PLACEHOLDER = '\uE103';

    // gokapi marker characters
    private static final char GOKAPI_OPENING = '\uE001';
    private static final char GOKAPI_CLOSING = '\uE002';
    private static final char GOKAPI_PLACEHOLDER = '\uE003';

    // gokapi SpanType values
    private static final int SPAN_OPENING = 0;
    private static final int SPAN_CLOSING = 1;
    private static final int SPAN_PLACEHOLDER = 2;

    /**
     * Convert an Okapi TextFragment to a gokapi FragmentDTO.
     */
    public static FragmentDTO toFragmentDTO(TextFragment tf) {
        if (tf == null) {
            return null;
        }

        FragmentDTO dto = new FragmentDTO();
        String codedText = tf.getCodedText();
        List<Code> codes = tf.getCodes();
        List<SpanDTO> spans = new ArrayList<>();
        StringBuilder gokapiText = new StringBuilder();

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
                        gokapiText.append(GOKAPI_OPENING);
                        span.setSpanType(SPAN_OPENING);
                    } else if (c == OKAPI_CLOSING) {
                        gokapiText.append(GOKAPI_CLOSING);
                        span.setSpanType(SPAN_CLOSING);
                    } else {
                        gokapiText.append(GOKAPI_PLACEHOLDER);
                        span.setSpanType(SPAN_PLACEHOLDER);
                    }

                    if (code != null) {
                        span.setId(String.valueOf(code.getId()));
                        span.setData(code.getData());
                        span.setOuterData(code.getOuterData());
                        span.setType(code.getType());
                        span.setDeletable(code.isDeleteable());
                        span.setCloneable(code.isCloneable());
                    }

                    spans.add(span);
                    i += 2; // Skip marker + index
                } else {
                    // Malformed: marker without index. Append as text.
                    gokapiText.append(c);
                    i++;
                }
            } else {
                gokapiText.append(c);
                i++;
            }
        }

        dto.setCodedText(gokapiText.toString());
        dto.setSpans(spans.isEmpty() ? null : spans);
        return dto;
    }

    /**
     * Convert a gokapi FragmentDTO to an Okapi TextFragment.
     */
    public static TextFragment toTextFragment(FragmentDTO dto) {
        if (dto == null || dto.getCodedText() == null) {
            return new TextFragment();
        }

        String gokapiText = dto.getCodedText();
        List<SpanDTO> spans = dto.getSpans();
        int spanIndex = 0;

        StringBuilder okapiText = new StringBuilder();
        List<Code> codes = new ArrayList<>();

        for (int i = 0; i < gokapiText.length(); i++) {
            char c = gokapiText.charAt(i);

            if (c == GOKAPI_OPENING || c == GOKAPI_CLOSING || c == GOKAPI_PLACEHOLDER) {
                SpanDTO span = (spans != null && spanIndex < spans.size()) ? spans.get(spanIndex) : null;
                spanIndex++;

                Code code;
                String codeData = (span != null && span.getData() != null) ? span.getData() : "";
                int codeId = 0;
                if (span != null && span.getId() != null && !span.getId().isEmpty()) {
                    try {
                        codeId = Integer.parseInt(span.getId());
                    } catch (NumberFormatException ignored) {
                    }
                }

                if (c == GOKAPI_OPENING) {
                    code = new Code(TextFragment.TagType.OPENING, codeData);
                    okapiText.append(OKAPI_OPENING);
                } else if (c == GOKAPI_CLOSING) {
                    code = new Code(TextFragment.TagType.CLOSING, codeData);
                    okapiText.append(OKAPI_CLOSING);
                } else {
                    code = new Code(TextFragment.TagType.PLACEHOLDER, codeData);
                    okapiText.append(OKAPI_PLACEHOLDER);
                }

                code.setId(codeId);
                if (span != null) {
                    if (span.getOuterData() != null) {
                        code.setOuterData(span.getOuterData());
                    }
                    if (span.getType() != null) {
                        code.setType(span.getType());
                    }
                    code.setDeleteable(span.isDeletable());
                    code.setCloneable(span.isCloneable());
                }

                int idx = codes.size();
                codes.add(code);
                okapiText.append(TextFragment.toChar(idx));
            } else {
                okapiText.append(c);
            }
        }

        TextFragment tf = new TextFragment();
        tf.setCodedText(okapiText.toString(), codes);
        return tf;
    }

    /**
     * Check if a character is a gokapi marker.
     */
    public static boolean isGokapiMarker(char c) {
        return c == GOKAPI_OPENING || c == GOKAPI_CLOSING || c == GOKAPI_PLACEHOLDER;
    }
}
