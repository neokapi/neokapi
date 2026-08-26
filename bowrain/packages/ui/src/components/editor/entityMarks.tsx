import { Fragment } from "react";
import { byteToCharOffset } from "@neokapi/ui-primitives";
import type { EntityInfo } from "../../types/api";

/**
 * Entity underlines over a block's plain source text, shared by every read-only
 * source renderer so a marked entity looks the same in the grid, the card and
 * the reviewer.
 *
 * Positions arrive as UTF-8 *byte* offsets — the server reports them from
 * `model.Anchor.ByteSpan` over the block's plain source — so they are resolved
 * to code-point offsets before anything is sliced. Treating them as string
 * indices underlines the wrong words as soon as a source carries one non-ASCII
 * character ahead of the entity.
 */

/** An entity located over the plain text, in code-point offsets. */
export interface EntityMark {
  start: number;
  end: number;
  entity: EntityInfo;
}

/** Color per entity type, from the app theme's entity tokens. */
const entityColors: Record<string, { bg: string; border: string }> = {
  "entity:person": { bg: "var(--entity-person-bg)", border: "var(--entity-person-border)" },
  "entity:organization": { bg: "var(--entity-org-bg)", border: "var(--entity-org-border)" },
  "entity:location": { bg: "var(--entity-location-bg)", border: "var(--entity-location-border)" },
  "entity:date": { bg: "var(--entity-date-bg)", border: "var(--entity-date-border)" },
  "entity:product": { bg: "var(--entity-product-bg)", border: "var(--entity-product-border)" },
};

/** The color pair for an entity type, falling back to the neutral accent. */
export function entityTypeColors(entityType: string): { bg: string; border: string } {
  return (
    entityColors[entityType] ?? {
      bg: "var(--entity-default-bg)",
      border: "var(--entity-default-border)",
    }
  );
}

/**
 * Resolve entity annotations onto `text` (the block's plain source), dropping
 * the ones that do not locate. Sorted by position and non-overlapping — the
 * first mark to claim a range wins, so a renderer can walk them in one pass.
 */
export function entityMarks(text: string, entities: EntityInfo[] | undefined): EntityMark[] {
  if (!entities || entities.length === 0) return [];
  const marks: EntityMark[] = [];
  for (const entity of entities) {
    if (entity.start < 0 || entity.end <= entity.start) continue;
    const start = byteToCharOffset(text, entity.start);
    const end = byteToCharOffset(text, entity.end);
    if (end > start) marks.push({ start, end, entity });
  }
  marks.sort((a, b) => a.start - b.start || b.end - a.end);
  const resolved: EntityMark[] = [];
  let lastEnd = 0;
  for (const mark of marks) {
    if (mark.start < lastEnd) continue;
    resolved.push(mark);
    lastEnd = mark.end;
  }
  return resolved;
}

/**
 * Render one slice of the plain text with the entity marks covering it
 * underlined. `offset` is the slice's own code-point offset into that text, so
 * a renderer that has already split the text (around inline codes, say) can
 * hand over its pieces one at a time.
 */
export function markEntities(slice: string, offset: number, marks: EntityMark[]): React.ReactNode {
  if (marks.length === 0 || !slice) return slice;
  const chars = Array.from(slice);
  const end = offset + chars.length;
  const at = (from: number, to: number) => chars.slice(from - offset, to - offset).join("");

  const parts: React.ReactNode[] = [];
  let cursor = offset;
  for (const mark of marks) {
    if (mark.end <= cursor || mark.start >= end) continue;
    const from = Math.max(mark.start, cursor);
    const to = Math.min(mark.end, end);
    if (to <= from) continue;
    if (from > cursor) parts.push(<Fragment key={`t${cursor}`}>{at(cursor, from)}</Fragment>);
    const colors = entityTypeColors(mark.entity.type);
    parts.push(
      <span
        key={`e${from}`}
        className="rounded-sm px-px"
        style={{ backgroundColor: colors.bg, borderBottom: `2px solid ${colors.border}` }}
        title={`${mark.entity.type.replace("entity:", "")}${mark.entity.dnt ? " (DNT)" : ""}`}
        data-entity-key={mark.entity.key}
      >
        {at(from, to)}
      </span>,
    );
    cursor = to;
  }
  if (parts.length === 0) return slice;
  if (cursor < end) parts.push(<Fragment key={`t${cursor}`}>{at(cursor, end)}</Fragment>);
  return parts;
}
