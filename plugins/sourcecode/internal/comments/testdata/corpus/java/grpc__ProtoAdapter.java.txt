package neokapi.bridge.grpc;

import neokapi.bridge.model.*;
import neokapi.bridge.proto.*;

import com.google.protobuf.ByteString;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Converts between existing DTO types and proto-generated message types.
 * This thin adapter layer allows the existing EventConverter/PartDTOConverter
 * to remain unchanged while the transport switches from NDJSON to gRPC.
 */
public class ProtoAdapter {

    // ── PartDTO ↔ PartMessage ───────────────────────────────────────────────

    public static PartMessage toProto(PartDTO dto) {
        PartMessage.Builder b = PartMessage.newBuilder()
                .setPartType(dto.getPartType());

        switch (dto.getPartType()) {
            case PartDTO.TYPE_LAYER_START:
            case PartDTO.TYPE_LAYER_END:
                if (dto.getLayer() != null) {
                    b.setLayer(toProto(dto.getLayer()));
                }
                break;
            case PartDTO.TYPE_GROUP_START:
                if (dto.getGroupStart() != null) {
                    b.setGroupStart(toProto(dto.getGroupStart()));
                }
                break;
            case PartDTO.TYPE_GROUP_END:
                if (dto.getGroupEnd() != null) {
                    b.setGroupEnd(toProto(dto.getGroupEnd()));
                }
                break;
            case PartDTO.TYPE_BLOCK:
                if (dto.getBlock() != null) {
                    b.setBlock(toProto(dto.getBlock()));
                }
                break;
            case PartDTO.TYPE_DATA:
                if (dto.getData() != null) {
                    b.setData(toProto(dto.getData()));
                }
                break;
            case PartDTO.TYPE_MEDIA:
                if (dto.getMedia() != null) {
                    b.setMedia(toProto(dto.getMedia()));
                }
                break;
        }

        return b.build();
    }

    public static PartDTO fromProto(PartMessage msg) {
        PartDTO dto = new PartDTO();
        dto.setPartType(msg.getPartType());

        switch (msg.getPartType()) {
            case PartDTO.TYPE_LAYER_START:
            case PartDTO.TYPE_LAYER_END:
                if (msg.hasLayer()) {
                    dto.setLayer(fromProto(msg.getLayer()));
                }
                break;
            case PartDTO.TYPE_GROUP_START:
                if (msg.hasGroupStart()) {
                    dto.setGroupStart(fromProto(msg.getGroupStart()));
                }
                break;
            case PartDTO.TYPE_GROUP_END:
                if (msg.hasGroupEnd()) {
                    dto.setGroupEnd(fromProto(msg.getGroupEnd()));
                }
                break;
            case PartDTO.TYPE_BLOCK:
                if (msg.hasBlock()) {
                    dto.setBlock(fromProto(msg.getBlock()));
                }
                break;
            case PartDTO.TYPE_DATA:
                if (msg.hasData()) {
                    dto.setData(fromProto(msg.getData()));
                }
                break;
            case PartDTO.TYPE_MEDIA:
                if (msg.hasMedia()) {
                    dto.setMedia(fromProto(msg.getMedia()));
                }
                break;
        }

        return dto;
    }

    // ── BlockDTO ↔ BlockMessage ─────────────────────────────────────────────

    public static BlockMessage toProto(BlockDTO dto) {
        BlockMessage.Builder b = BlockMessage.newBuilder()
                .setId(nullSafe(dto.getId()))
                .setName(nullSafe(dto.getName()))
                .setType(nullSafe(dto.getType()))
                .setMimeType(nullSafe(dto.getMimeType()))
                .setTranslatable(dto.isTranslatable())
                .setPreserveWhitespace(dto.isPreserveWhitespace())
                .setIsReferent(dto.isReferent());

        if (dto.getSource() != null) {
            for (SegmentDTO seg : dto.getSource()) {
                b.addSource(toProto(seg));
            }
        }

        if (dto.getTargets() != null) {
            for (TargetDTO target : dto.getTargets()) {
                TargetEntry.Builder te = TargetEntry.newBuilder()
                        .setLocale(nullSafe(target.getLocale()));
                if (target.getSegments() != null) {
                    for (SegmentDTO seg : target.getSegments()) {
                        te.addSegments(toProto(seg));
                    }
                }
                b.addTargets(te);
            }
        }

        if (dto.getProperties() != null) {
            b.putAllProperties(sanitizeProperties(dto.getProperties()));
        }

        if (dto.getDisplayHint() != null) {
            b.setDisplayHint(toProto(dto.getDisplayHint()));
        }

        if (dto.getSkeleton() != null) {
            b.setSkeleton(toProto(dto.getSkeleton()));
        }

        if (dto.getAnnotations() != null) {
            for (Map.Entry<String, AnnotationEntryDTO> entry : dto.getAnnotations().entrySet()) {
                AnnotationEntryDTO ae = entry.getValue();
                neokapi.bridge.proto.AnnotationEntry.Builder ab =
                        neokapi.bridge.proto.AnnotationEntry.newBuilder()
                                .setType(nullSafe(ae.getType()));
                if (ae.getData() != null) {
                    ab.setData(ByteString.copyFrom(ae.getData()));
                }
                b.putAnnotations(entry.getKey(), ab.build());
            }
        }

        return b.build();
    }

    public static BlockDTO fromProto(BlockMessage msg) {
        BlockDTO dto = new BlockDTO();
        dto.setId(msg.getId());
        dto.setName(msg.getName());
        dto.setType(msg.getType());
        dto.setMimeType(msg.getMimeType());
        dto.setTranslatable(msg.getTranslatable());
        dto.setPreserveWhitespace(msg.getPreserveWhitespace());
        dto.setReferent(msg.getIsReferent());

        if (msg.getSourceCount() > 0) {
            List<SegmentDTO> source = new ArrayList<>();
            for (SegmentMessage seg : msg.getSourceList()) {
                source.add(fromProto(seg));
            }
            dto.setSource(source);
        }

        if (msg.getTargetsCount() > 0) {
            List<TargetDTO> targets = new ArrayList<>();
            for (TargetEntry te : msg.getTargetsList()) {
                TargetDTO target = new TargetDTO();
                target.setLocale(te.getLocale());
                List<SegmentDTO> segs = new ArrayList<>();
                for (SegmentMessage seg : te.getSegmentsList()) {
                    segs.add(fromProto(seg));
                }
                target.setSegments(segs);
                targets.add(target);
            }
            dto.setTargets(targets);
        }

        if (msg.getPropertiesCount() > 0) {
            dto.setProperties(new LinkedHashMap<>(msg.getPropertiesMap()));
        }

        if (msg.hasDisplayHint()) {
            dto.setDisplayHint(fromProto(msg.getDisplayHint()));
        }

        if (msg.hasSkeleton()) {
            dto.setSkeleton(fromProto(msg.getSkeleton()));
        }

        if (msg.getAnnotationsCount() > 0) {
            Map<String, AnnotationEntryDTO> annotations = new LinkedHashMap<>();
            for (Map.Entry<String, neokapi.bridge.proto.AnnotationEntry> entry : msg.getAnnotationsMap().entrySet()) {
                neokapi.bridge.proto.AnnotationEntry ae = entry.getValue();
                annotations.put(entry.getKey(), new AnnotationEntryDTO(ae.getType(), ae.getData().toByteArray()));
            }
            dto.setAnnotations(annotations);
        }

        return dto;
    }

    // ── LayerDTO ↔ LayerMessage ─────────────────────────────────────────────

    public static LayerMessage toProto(LayerDTO dto) {
        LayerMessage.Builder b = LayerMessage.newBuilder()
                .setId(nullSafe(dto.getId()))
                .setName(nullSafe(dto.getName()))
                .setFormat(nullSafe(dto.getFormat()))
                .setLocale(nullSafe(dto.getLocale()))
                .setEncoding(nullSafe(dto.getEncoding()))
                .setMimeType(nullSafe(dto.getMimeType()))
                .setLineBreak(nullSafe(dto.getLineBreak()))
                .setIsMultilingual(dto.isMultilingual())
                .setParentId(nullSafe(dto.getParentId()))
                .setHasBom(dto.isHasBom());

        if (dto.getProperties() != null) {
            b.putAllProperties(sanitizeProperties(dto.getProperties()));
        }

        return b.build();
    }

    public static LayerDTO fromProto(LayerMessage msg) {
        LayerDTO dto = new LayerDTO();
        dto.setId(msg.getId());
        dto.setName(msg.getName());
        dto.setFormat(msg.getFormat());
        dto.setLocale(msg.getLocale());
        dto.setEncoding(msg.getEncoding());
        dto.setMimeType(msg.getMimeType());
        dto.setLineBreak(msg.getLineBreak());
        dto.setMultilingual(msg.getIsMultilingual());
        dto.setParentId(msg.getParentId());
        dto.setHasBom(msg.getHasBom());

        if (msg.getPropertiesCount() > 0) {
            dto.setProperties(new LinkedHashMap<>(msg.getPropertiesMap()));
        }

        return dto;
    }

    // ── DataDTO ↔ DataMessage ───────────────────────────────────────────────

    public static DataMessage toProto(DataDTO dto) {
        DataMessage.Builder b = DataMessage.newBuilder()
                .setId(nullSafe(dto.getId()))
                .setName(nullSafe(dto.getName()))
                .setIsReferent(dto.isReferent());

        if (dto.getProperties() != null) {
            b.putAllProperties(sanitizeProperties(dto.getProperties()));
        }

        if (dto.getSkeleton() != null) {
            b.setSkeleton(toProto(dto.getSkeleton()));
        }

        return b.build();
    }

    public static DataDTO fromProto(DataMessage msg) {
        DataDTO dto = new DataDTO();
        dto.setId(msg.getId());
        dto.setName(msg.getName());
        dto.setReferent(msg.getIsReferent());
        if (msg.getPropertiesCount() > 0) {
            dto.setProperties(new LinkedHashMap<>(msg.getPropertiesMap()));
        }
        if (msg.hasSkeleton()) {
            dto.setSkeleton(fromProto(msg.getSkeleton()));
        }
        return dto;
    }

    // ── GroupStartDTO ↔ GroupStartMessage ────────────────────────────────────

    public static GroupStartMessage toProto(GroupStartDTO dto) {
        GroupStartMessage.Builder b = GroupStartMessage.newBuilder()
                .setId(nullSafe(dto.getId()))
                .setName(nullSafe(dto.getName()))
                .setType(nullSafe(dto.getType()));

        if (dto.getProperties() != null) {
            b.putAllProperties(sanitizeProperties(dto.getProperties()));
        }

        return b.build();
    }

    public static GroupStartDTO fromProto(GroupStartMessage msg) {
        GroupStartDTO dto = new GroupStartDTO();
        dto.setId(msg.getId());
        dto.setName(msg.getName());
        dto.setType(msg.getType());
        if (msg.getPropertiesCount() > 0) {
            dto.setProperties(new LinkedHashMap<>(msg.getPropertiesMap()));
        }
        return dto;
    }

    // ── GroupEndDTO ↔ GroupEndMessage ────────────────────────────────────────

    public static GroupEndMessage toProto(GroupEndDTO dto) {
        return GroupEndMessage.newBuilder()
                .setId(nullSafe(dto.getId()))
                .build();
    }

    public static GroupEndDTO fromProto(GroupEndMessage msg) {
        GroupEndDTO dto = new GroupEndDTO();
        dto.setId(msg.getId());
        return dto;
    }

    // ── MediaDTO ↔ MediaMessage ─────────────────────────────────────────────

    public static MediaMessage toProto(MediaDTO dto) {
        MediaMessage.Builder b = MediaMessage.newBuilder()
                .setId(nullSafe(dto.getId()))
                .setMimeType(nullSafe(dto.getMimeType()))
                .setUri(nullSafe(dto.getUri()))
                .setAltText(nullSafe(dto.getAltText()));

        if (dto.getProperties() != null) {
            b.putAllProperties(sanitizeProperties(dto.getProperties()));
        }

        return b.build();
    }

    public static MediaDTO fromProto(MediaMessage msg) {
        MediaDTO dto = new MediaDTO();
        dto.setId(msg.getId());
        dto.setMimeType(msg.getMimeType());
        dto.setUri(msg.getUri());
        dto.setAltText(msg.getAltText());
        if (msg.getPropertiesCount() > 0) {
            dto.setProperties(new LinkedHashMap<>(msg.getPropertiesMap()));
        }
        return dto;
    }

    // ── SegmentDTO ↔ SegmentMessage ─────────────────────────────────────────

    public static SegmentMessage toProto(SegmentDTO dto) {
        SegmentMessage.Builder b = SegmentMessage.newBuilder()
                .setId(nullSafe(dto.getId()));

        if (dto.getContent() != null) {
            for (RunMessage run : runsFromFragment(dto.getContent())) {
                b.addRuns(run);
            }
        }

        if (dto.getProperties() != null) {
            b.putAllProperties(sanitizeProperties(dto.getProperties()));
        }

        return b.build();
    }

    public static SegmentDTO fromProto(SegmentMessage msg) {
        SegmentDTO dto = new SegmentDTO();
        dto.setId(msg.getId());
        if (msg.getRunsCount() > 0) {
            dto.setContent(runsToFragment(msg.getRunsList()));
        }
        if (msg.getPropertiesCount() > 0) {
            dto.setProperties(new LinkedHashMap<>(msg.getPropertiesMap()));
        }
        return dto;
    }

    // ── FragmentDTO ↔ List<RunMessage> (RFC 0001 Run model) ─────────────────
    //
    // Mirrors core/model/coded_text.go (FragmentToRuns / RunsToFragment).
    // The bridge's internal Java model is unchanged (FragmentDTO with
    // PUA-marker codedText + SpanDTO list), only the wire boundary uses Runs.
    //
    // PUA marker convention (from neokapi core/model):
    //   '' = opening (PcOpen)
    //   '' = closing (PcClose)
    //   '' = placeholder (Ph / Sub)

    private static final char MARKER_OPENING = '';
    private static final char MARKER_CLOSING = '';
    private static final char MARKER_PLACEHOLDER = '';

    private static boolean isMarker(char c) {
        return c >= MARKER_OPENING && c <= MARKER_PLACEHOLDER;
    }

    /**
     * Walk a FragmentDTO (codedText + spans) and emit the equivalent Run sequence.
     * Non-marker runes accumulate into TextRuns; markers consume one SpanDTO and
     * emit the appropriate PcOpen / PcClose / Placeholder run.
     */
    public static List<RunMessage> runsFromFragment(FragmentDTO frag) {
        List<RunMessage> runs = new ArrayList<>();
        if (frag == null) {
            return runs;
        }
        String coded = frag.getCodedText();
        if (coded == null) {
            coded = "";
        }
        List<SpanDTO> spans = frag.getSpans();
        if (spans == null || spans.isEmpty()) {
            if (!coded.isEmpty()) {
                runs.add(textRun(coded));
            }
            return runs;
        }
        StringBuilder text = new StringBuilder();
        int spanIdx = 0;
        // Iterate by code point to keep supplementary characters intact.
        int i = 0;
        while (i < coded.length()) {
            int cp = coded.codePointAt(i);
            int cpLen = Character.charCount(cp);
            if (cp <= 0xFFFF && isMarker((char) cp)) {
                if (text.length() > 0) {
                    runs.add(textRun(text.toString()));
                    text.setLength(0);
                }
                if (spanIdx < spans.size()) {
                    SpanDTO span = spans.get(spanIdx++);
                    runs.add(spanToRun(span, (char) cp));
                }
                // If span list is shorter than markers in coded text,
                // silently drop the orphan marker (defensive — matches Go).
            } else {
                text.appendCodePoint(cp);
            }
            i += cpLen;
        }
        if (text.length() > 0) {
            runs.add(textRun(text.toString()));
        }
        return runs;
    }

    /**
     * Inverse of {@link #runsFromFragment}: walk a Run sequence and rebuild
     * a FragmentDTO with PUA-marker coded text + SpanDTO list.
     */
    public static FragmentDTO runsToFragment(List<RunMessage> runs) {
        FragmentDTO frag = new FragmentDTO();
        StringBuilder coded = new StringBuilder();
        List<SpanDTO> spans = new ArrayList<>();
        if (runs != null) {
            for (RunMessage run : runs) {
                appendRun(run, coded, spans);
            }
        }
        frag.setCodedText(coded.toString());
        if (!spans.isEmpty()) {
            frag.setSpans(spans);
        }
        return frag;
    }

    private static void appendRun(RunMessage run, StringBuilder coded, List<SpanDTO> spans) {
        switch (run.getKindCase()) {
            case TEXT:
                coded.append(run.getText().getText());
                break;
            case PH:
                coded.append(MARKER_PLACEHOLDER);
                spans.add(runToSpan(run));
                break;
            case PC_OPEN:
                coded.append(MARKER_OPENING);
                spans.add(runToSpan(run));
                break;
            case PC_CLOSE:
                coded.append(MARKER_CLOSING);
                spans.add(runToSpan(run));
                break;
            case SUB:
                coded.append(MARKER_PLACEHOLDER);
                spans.add(runToSpan(run));
                break;
            case PLURAL:
            case SELECT:
                // TODO(neokapi#450 follow-up): Plural/Select have no Java DTO
                // analogue. Emit a placeholder span carrying the pivot so the
                // slot is preserved on writers that don't speak Run natively.
                coded.append(MARKER_PLACEHOLDER);
                spans.add(pivotPlaceholderSpan(run));
                break;
            case KIND_NOT_SET:
            default:
                // Skip empty runs.
                break;
        }
    }

    private static RunMessage textRun(String text) {
        // Use bytes-based setter to preserve supplementary Unicode characters
        // (emoji, codepoints >= U+10000) the same way the old FragmentMessage
        // setter did.
        TextRunMessage trm = TextRunMessage.newBuilder()
                .setTextBytes(ByteString.copyFromUtf8(text))
                .build();
        return RunMessage.newBuilder().setText(trm).build();
    }

    /**
     * Lift a legacy SpanDTO + marker into the equivalent RunMessage.
     * Mirrors core/model/coded_text.go spanToRun.
     *
     * NOTE: SpanDTO has no SubType field today (the Java internal model
     * predates it); we send "" for sub_type. SpanDTO.outerData, originalId
     * and flags also have no Run-side equivalent and are dropped on the wire.
     * TODO(neokapi#450 follow-up): annotations gap — legacy SpanMessage
     * carried an annotations map; new RunMessage variants don't, so any
     * span annotations are dropped on the wire.
     */
    static RunMessage spanToRun(SpanDTO span, char marker) {
        RunConstraints constraints = RunConstraints.newBuilder()
                .setDeletable(span.isDeletable())
                .setCloneable(span.isCloneable())
                .setReorderable(span.isCanReorder())
                .build();
        switch (marker) {
            case MARKER_OPENING: {
                PcOpenRunMessage.Builder b = PcOpenRunMessage.newBuilder()
                        .setId(nullSafe(span.getId()))
                        .setType(nullSafe(span.getType()))
                        .setSubType("")
                        .setConstraints(constraints);
                b.setDataBytes(ByteString.copyFromUtf8(nullSafe(span.getData())));
                b.setEquivBytes(ByteString.copyFromUtf8(nullSafe(span.getEquivText())));
                b.setDispBytes(ByteString.copyFromUtf8(nullSafe(span.getDisplayText())));
                return RunMessage.newBuilder().setPcOpen(b.build()).build();
            }
            case MARKER_CLOSING: {
                PcCloseRunMessage.Builder b = PcCloseRunMessage.newBuilder()
                        .setId(nullSafe(span.getId()))
                        .setType(nullSafe(span.getType()))
                        .setSubType("");
                b.setDataBytes(ByteString.copyFromUtf8(nullSafe(span.getData())));
                b.setEquivBytes(ByteString.copyFromUtf8(nullSafe(span.getEquivText())));
                return RunMessage.newBuilder().setPcClose(b.build()).build();
            }
            case MARKER_PLACEHOLDER:
            default: {
                // SpanDTO with type="sub" is treated as a SubRun in the
                // inverse direction; preserve that round-trip by emitting
                // a SubRunMessage when the SpanDTO type is "sub". Otherwise
                // emit a PlaceholderRunMessage.
                if ("sub".equals(span.getType())) {
                    SubRunMessage.Builder sb = SubRunMessage.newBuilder()
                            .setId(nullSafe(span.getId()));
                    sb.setRefBytes(ByteString.copyFromUtf8(nullSafe(span.getData())));
                    sb.setEquivBytes(ByteString.copyFromUtf8(nullSafe(span.getEquivText())));
                    return RunMessage.newBuilder().setSub(sb.build()).build();
                }
                PlaceholderRunMessage.Builder b = PlaceholderRunMessage.newBuilder()
                        .setId(nullSafe(span.getId()))
                        .setType(nullSafe(span.getType()))
                        .setSubType("")
                        .setConstraints(constraints);
                b.setDataBytes(ByteString.copyFromUtf8(nullSafe(span.getData())));
                b.setEquivBytes(ByteString.copyFromUtf8(nullSafe(span.getEquivText())));
                b.setDispBytes(ByteString.copyFromUtf8(nullSafe(span.getDisplayText())));
                return RunMessage.newBuilder().setPh(b.build()).build();
            }
        }
    }

    /**
     * Inverse of {@link #spanToRun}: turn a Run back into a SpanDTO. Span
     * type code (0=Opening, 1=Closing, 2=Placeholder) is set from the
     * RunMessage variant.
     *
     * TODO(neokapi#450 follow-up): annotations gap — RunMessage variants
     * do not carry an annotations map, so SpanDTO has no annotations to
     * populate from the wire.
     */
    static SpanDTO runToSpan(RunMessage run) {
        SpanDTO dto = new SpanDTO();
        switch (run.getKindCase()) {
            case PH: {
                PlaceholderRunMessage ph = run.getPh();
                dto.setSpanType(2); // Placeholder
                dto.setId(ph.getId());
                dto.setType(ph.getType());
                dto.setData(ph.getData());
                dto.setEquivText(ph.getEquiv());
                dto.setDisplayText(ph.getDisp());
                applyConstraints(dto, ph.hasConstraints() ? ph.getConstraints() : null);
                break;
            }
            case PC_OPEN: {
                PcOpenRunMessage pc = run.getPcOpen();
                dto.setSpanType(0); // Opening
                dto.setId(pc.getId());
                dto.setType(pc.getType());
                dto.setData(pc.getData());
                dto.setEquivText(pc.getEquiv());
                dto.setDisplayText(pc.getDisp());
                applyConstraints(dto, pc.hasConstraints() ? pc.getConstraints() : null);
                break;
            }
            case PC_CLOSE: {
                PcCloseRunMessage pc = run.getPcClose();
                dto.setSpanType(1); // Closing
                dto.setId(pc.getId());
                dto.setType(pc.getType());
                dto.setData(pc.getData());
                dto.setEquivText(pc.getEquiv());
                // No constraints / disp on PcClose by design.
                break;
            }
            case SUB: {
                SubRunMessage sub = run.getSub();
                dto.setSpanType(2); // Placeholder
                dto.setId(sub.getId());
                dto.setType("sub");
                dto.setData(sub.getRef());
                dto.setEquivText(sub.getEquiv());
                dto.setDisplayText(sub.getEquiv());
                break;
            }
            case PLURAL:
            case SELECT:
                // Defensive: caller flow should already have routed through
                // pivotPlaceholderSpan, but if we land here treat the same.
                return pivotPlaceholderSpan(run);
            case TEXT:
            case KIND_NOT_SET:
            default:
                dto.setSpanType(2);
                break;
        }
        return dto;
    }

    private static void applyConstraints(SpanDTO dto, RunConstraints c) {
        if (c == null) {
            // Match the Go reference defaults (deletable + reorderable true,
            // cloneable false) when constraints are absent.
            dto.setDeletable(true);
            dto.setCloneable(false);
            dto.setCanReorder(true);
            return;
        }
        dto.setDeletable(c.getDeletable());
        dto.setCloneable(c.getCloneable());
        dto.setCanReorder(c.getReorderable());
    }

    /**
     * Build a placeholder SpanDTO carrying the pivot string for an unsupported
     * Plural/Select run. TODO(neokapi#450 follow-up): full plural/select
     * support requires a richer Java DTO model.
     */
    private static SpanDTO pivotPlaceholderSpan(RunMessage run) {
        SpanDTO dto = new SpanDTO();
        dto.setSpanType(2); // Placeholder
        switch (run.getKindCase()) {
            case PLURAL:
                dto.setType("plural");
                dto.setData(run.getPlural().getPivot());
                break;
            case SELECT:
                dto.setType("select");
                dto.setData(run.getSelect().getPivot());
                break;
            default:
                dto.setType("");
                break;
        }
        return dto;
    }

    // ── SkeletonDTO ↔ SkeletonMessage ──────────────────────────────────────

    public static SkeletonMessage toProto(SkeletonDTO dto) {
        SkeletonMessage.Builder b = SkeletonMessage.newBuilder()
                .setStrategy(dto.getStrategy())
                .setSourceUri(nullSafe(dto.getSourceUri()));

        if (dto.getParts() != null) {
            for (SkeletonPartDTO part : dto.getParts()) {
                SkeletonPartMessage.Builder pb = SkeletonPartMessage.newBuilder()
                        .setText(nullSafe(part.getText()))
                        .setResourceId(nullSafe(part.getResourceId()))
                        .setProperty(nullSafe(part.getProperty()))
                        .setLocale(nullSafe(part.getLocale()));
                b.addParts(pb);
            }
        }

        return b.build();
    }

    public static SkeletonDTO fromProto(SkeletonMessage msg) {
        SkeletonDTO dto = new SkeletonDTO();
        dto.setStrategy(msg.getStrategy());
        dto.setSourceUri(msg.getSourceUri());

        if (msg.getPartsCount() > 0) {
            List<SkeletonPartDTO> parts = new ArrayList<>();
            for (SkeletonPartMessage pm : msg.getPartsList()) {
                SkeletonPartDTO part = new SkeletonPartDTO();
                part.setText(pm.getText());
                part.setResourceId(pm.getResourceId());
                part.setProperty(pm.getProperty());
                part.setLocale(pm.getLocale());
                parts.add(part);
            }
            dto.setParts(parts);
        }

        return dto;
    }

    // ── DisplayHintDTO ↔ DisplayHintMessage ─────────────────────────────────

    public static DisplayHintMessage toProto(DisplayHintDTO dto) {
        return DisplayHintMessage.newBuilder()
                .setMaxLength(dto.getMaxLength())
                .setContentType(nullSafe(dto.getContentType()))
                .setContext(nullSafe(dto.getContext()))
                .setPreview(nullSafe(dto.getPreview()))
                .build();
    }

    public static DisplayHintDTO fromProto(DisplayHintMessage msg) {
        DisplayHintDTO dto = new DisplayHintDTO();
        dto.setMaxLength(msg.getMaxLength());
        dto.setContentType(msg.getContentType());
        dto.setContext(msg.getContext());
        dto.setPreview(msg.getPreview());
        return dto;
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    private static String nullSafe(String s) {
        return s != null ? s : "";
    }

    /**
     * Sanitize a properties map for protobuf: replace null values with empty strings.
     * Protobuf map fields reject null values with NullPointerException.
     */
    private static Map<String, String> sanitizeProperties(Map<String, String> map) {
        Map<String, String> clean = new LinkedHashMap<>();
        for (Map.Entry<String, String> entry : map.entrySet()) {
            clean.put(entry.getKey(), entry.getValue() != null ? entry.getValue() : "");
        }
        return clean;
    }
}
