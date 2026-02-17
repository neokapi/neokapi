package com.gokapi.bridge;

import com.gokapi.bridge.model.*;
import com.gokapi.bridge.util.OkapiCodeConverter;

import net.sf.okapi.common.Event;
import net.sf.okapi.common.EventType;
import net.sf.okapi.common.LocaleId;
import net.sf.okapi.common.resource.*;

import java.util.*;

/**
 * Applies translations from PartDTOs back to Okapi Events during the write path.
 * Matches text units by ID and applies target translations.
 */
public class PartDTOConverter {

    private final Map<String, BlockDTO> blocksByID;
    private final LocaleId targetLocale;

    /**
     * Create a converter with a list of parts containing translations.
     *
     * @param parts        the translated parts from the Go side
     * @param targetLocale the target locale to apply
     */
    public PartDTOConverter(List<PartDTO> parts, LocaleId targetLocale) {
        this.targetLocale = targetLocale;
        this.blocksByID = new LinkedHashMap<>();

        for (PartDTO part : parts) {
            if (part.getPartType() == PartDTO.TYPE_BLOCK && part.getBlock() != null) {
                blocksByID.put(part.getBlock().getId(), part.getBlock());
            }
        }
    }

    /**
     * Apply translations to an Okapi event if it's a TEXT_UNIT with a matching block.
     * Returns the (possibly modified) event.
     */
    public Event applyTranslations(Event event) {
        if (event.getEventType() != EventType.TEXT_UNIT) {
            return event;
        }

        ITextUnit tu = event.getTextUnit();
        BlockDTO block = blocksByID.get(tu.getId());
        if (block == null || block.getTargets() == null) {
            return event;
        }

        // Find the target for our locale.
        TargetDTO matchingTarget = null;
        for (TargetDTO target : block.getTargets()) {
            if (target.getLocale() != null && target.getLocale().equals(targetLocale.toString())) {
                matchingTarget = target;
                break;
            }
        }

        if (matchingTarget == null || matchingTarget.getSegments() == null) {
            return event;
        }

        // Apply target segments.
        TextContainer targetContainer = new TextContainer();
        tu.setTarget(targetLocale, targetContainer);

        List<SegmentDTO> targetSegments = matchingTarget.getSegments();

        if (targetSegments.size() == 1) {
            // Single segment: set as first content.
            TextFragment tf = OkapiCodeConverter.toTextFragment(targetSegments.get(0).getContent());
            targetContainer.setContent(tf);
        } else {
            // Multiple segments: set each one.
            // First, ensure the target has the same segment structure.
            TextContainer source = tu.getSource();
            targetContainer.setContent(source.getUnSegmentedContentCopy());

            Iterator<Segment> targetSegs = targetContainer.getSegments().iterator();
            int idx = 0;
            while (targetSegs.hasNext() && idx < targetSegments.size()) {
                Segment seg = targetSegs.next();
                SegmentDTO segDTO = targetSegments.get(idx);
                TextFragment tf = OkapiCodeConverter.toTextFragment(segDTO.getContent());
                seg.setContent(tf);
                idx++;
            }
        }

        return event;
    }
}
