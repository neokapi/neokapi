package com.gokapi.bridge;

import com.gokapi.bridge.model.*;
import com.gokapi.bridge.util.OkapiCodeConverter;

import net.sf.okapi.common.Event;
import net.sf.okapi.common.EventType;
import net.sf.okapi.common.LocaleId;
import net.sf.okapi.common.resource.*;

import java.util.*;

/**
 * Converts Okapi Events to gokapi PartDTOs.
 *
 * <h3>Event type mapping:</h3>
 * <table>
 *     <tr><th>Okapi EventType</th><th>PartType</th></tr>
 *     <tr><td>START_DOCUMENT</td><td>0 (LayerStart)</td></tr>
 *     <tr><td>END_DOCUMENT</td><td>1 (LayerEnd)</td></tr>
 *     <tr><td>START_GROUP</td><td>2 (GroupStart)</td></tr>
 *     <tr><td>END_GROUP</td><td>3 (GroupEnd)</td></tr>
 *     <tr><td>TEXT_UNIT</td><td>4 (Block)</td></tr>
 *     <tr><td>DOCUMENT_PART</td><td>5 (Data)</td></tr>
 *     <tr><td>START_SUBDOCUMENT</td><td>0 (LayerStart, child)</td></tr>
 *     <tr><td>END_SUBDOCUMENT</td><td>1 (LayerEnd, child)</td></tr>
 *     <tr><td>START_SUBFILTER</td><td>0 (LayerStart, child)</td></tr>
 *     <tr><td>END_SUBFILTER</td><td>1 (LayerEnd, child)</td></tr>
 * </table>
 */
public class EventConverter {

    /**
     * Convert an Okapi Event to a gokapi PartDTO.
     * Returns null for event types that don't map to gokapi parts.
     */
    public static PartDTO convert(Event event) {
        if (event == null) {
            return null;
        }

        EventType type = event.getEventType();

        switch (type) {
            case START_DOCUMENT:
                return convertStartDocument(event);
            case END_DOCUMENT:
                return convertEndDocument(event);
            case START_GROUP:
                return convertStartGroup(event);
            case END_GROUP:
                return convertEndGroup(event);
            case TEXT_UNIT:
                return convertTextUnit(event);
            case DOCUMENT_PART:
                return convertDocumentPart(event);
            case START_SUBDOCUMENT:
                return convertStartSubDocument(event);
            case END_SUBDOCUMENT:
                return convertEndSubDocument(event);
            case START_SUBFILTER:
                return convertStartSubFilter(event);
            case END_SUBFILTER:
                return convertEndSubFilter(event);
            default:
                // Unhandled event types (e.g., NO_OP, CANCELED, CUSTOM, etc.)
                return null;
        }
    }

    private static PartDTO convertStartDocument(Event event) {
        PartDTO part = new PartDTO();
        part.setPartType(PartDTO.TYPE_LAYER_START);

        StartDocument sd = (StartDocument) event.getResource();
        LayerDTO layer = new LayerDTO();
        layer.setId(sd.getId());
        layer.setName(sd.getName());
        layer.setLocale(sd.getLocale() != null ? sd.getLocale().toString() : "");
        layer.setEncoding(sd.getEncoding());
        layer.setMimeType(sd.getMimeType());
        layer.setLineBreak(sd.getLineBreak());
        layer.setMultilingual(sd.isMultilingual());
        part.setLayer(layer);
        return part;
    }

    private static PartDTO convertEndDocument(Event event) {
        PartDTO part = new PartDTO();
        part.setPartType(PartDTO.TYPE_LAYER_END);

        Ending ed = (Ending) event.getResource();
        LayerDTO layer = new LayerDTO();
        layer.setId(ed.getId());
        part.setLayer(layer);
        return part;
    }

    private static PartDTO convertStartGroup(Event event) {
        PartDTO part = new PartDTO();
        part.setPartType(PartDTO.TYPE_GROUP_START);

        StartGroup sg = (StartGroup) event.getResource();
        GroupStartDTO gs = new GroupStartDTO();
        gs.setId(sg.getId());
        gs.setName(sg.getName());
        gs.setType(sg.getType());
        part.setGroupStart(gs);
        return part;
    }

    private static PartDTO convertEndGroup(Event event) {
        PartDTO part = new PartDTO();
        part.setPartType(PartDTO.TYPE_GROUP_END);

        Ending eg = (Ending) event.getResource();
        GroupEndDTO ge = new GroupEndDTO();
        ge.setId(eg.getId());
        part.setGroupEnd(ge);
        return part;
    }

    private static PartDTO convertTextUnit(Event event) {
        PartDTO part = new PartDTO();
        part.setPartType(PartDTO.TYPE_BLOCK);

        ITextUnit tu = event.getTextUnit();
        BlockDTO block = new BlockDTO();
        block.setId(tu.getId());
        block.setName(tu.getName());
        block.setType(tu.getType());
        block.setMimeType(tu.getMimeType());
        block.setTranslatable(tu.isTranslatable());

        // Convert source segments.
        TextContainer source = tu.getSource();
        block.setSource(convertTextContainer(source));

        // Convert target locales.
        List<TargetDTO> targets = new ArrayList<>();
        for (LocaleId locale : tu.getTargetLocales()) {
            TextContainer target = tu.getTarget(locale);
            if (target != null) {
                TargetDTO targetDTO = new TargetDTO();
                targetDTO.setLocale(locale.toString());
                targetDTO.setSegments(convertTextContainer(target));
                targets.add(targetDTO);
            }
        }
        if (!targets.isEmpty()) {
            block.setTargets(targets);
        }

        // Convert properties.
        if (tu.getPropertyNames() != null && !tu.getPropertyNames().isEmpty()) {
            Map<String, String> props = new LinkedHashMap<>();
            for (String propName : tu.getPropertyNames()) {
                Property prop = tu.getProperty(propName);
                if (prop != null) {
                    props.put(propName, prop.getValue());
                }
            }
            if (!props.isEmpty()) {
                block.setProperties(props);
            }
        }

        part.setBlock(block);
        return part;
    }

    private static PartDTO convertDocumentPart(Event event) {
        PartDTO part = new PartDTO();
        part.setPartType(PartDTO.TYPE_DATA);

        DocumentPart dp = (DocumentPart) event.getResource();
        DataDTO data = new DataDTO();
        data.setId(dp.getId());
        data.setName(dp.getName());

        // Convert properties.
        if (dp.getPropertyNames() != null && !dp.getPropertyNames().isEmpty()) {
            Map<String, String> props = new LinkedHashMap<>();
            for (String propName : dp.getPropertyNames()) {
                Property prop = dp.getProperty(propName);
                if (prop != null) {
                    props.put(propName, prop.getValue());
                }
            }
            if (!props.isEmpty()) {
                data.setProperties(props);
            }
        }

        part.setData(data);
        return part;
    }

    private static PartDTO convertStartSubDocument(Event event) {
        PartDTO part = new PartDTO();
        part.setPartType(PartDTO.TYPE_LAYER_START);

        StartSubDocument ssd = (StartSubDocument) event.getResource();
        LayerDTO layer = new LayerDTO();
        layer.setId(ssd.getId());
        layer.setName(ssd.getName());
        if (ssd.getParentId() != null) {
            layer.setParentId(ssd.getParentId());
        }
        part.setLayer(layer);
        return part;
    }

    private static PartDTO convertEndSubDocument(Event event) {
        PartDTO part = new PartDTO();
        part.setPartType(PartDTO.TYPE_LAYER_END);

        Ending esd = (Ending) event.getResource();
        LayerDTO layer = new LayerDTO();
        layer.setId(esd.getId());
        part.setLayer(layer);
        return part;
    }

    private static PartDTO convertStartSubFilter(Event event) {
        // Treated the same as StartSubDocument.
        return convertStartSubDocument(event);
    }

    private static PartDTO convertEndSubFilter(Event event) {
        // Treated the same as EndSubDocument.
        return convertEndSubDocument(event);
    }

    /**
     * Convert a TextContainer to a list of SegmentDTOs.
     */
    private static List<SegmentDTO> convertTextContainer(TextContainer tc) {
        List<SegmentDTO> segments = new ArrayList<>();

        if (tc.contentIsOneSegment()) {
            // Single segment: use the whole content.
            SegmentDTO seg = new SegmentDTO();
            seg.setId("0");
            seg.setContent(OkapiCodeConverter.toFragmentDTO(tc.getFirstContent()));
            segments.add(seg);
        } else {
            // Multiple segments.
            for (Segment okapiSeg : tc.getSegments()) {
                SegmentDTO seg = new SegmentDTO();
                seg.setId(okapiSeg.getId() != null ? okapiSeg.getId() : "");
                seg.setContent(OkapiCodeConverter.toFragmentDTO(okapiSeg.getContent()));
                segments.add(seg);
            }
        }

        return segments;
    }
}
