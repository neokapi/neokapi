import { useState, useCallback } from "react";
import type { BlockTermMatch, EntityInfo } from "../../types/api";
import { EntityPopover } from "./EntityPopover";

/** Color config per entity type. */
const entityColors: Record<string, { bg: string; border: string }> = {
  "entity:person": { bg: "var(--entity-person-bg)", border: "var(--entity-person-border)" },
  "entity:organization": { bg: "var(--entity-org-bg)", border: "var(--entity-org-border)" },
  "entity:location": { bg: "var(--entity-location-bg)", border: "var(--entity-location-border)" },
  "entity:date": { bg: "var(--entity-date-bg)", border: "var(--entity-date-border)" },
  "entity:product": { bg: "var(--entity-product-bg)", border: "var(--entity-product-border)" },
};

function getEntityColors(entityType: string) {
  return (
    entityColors[entityType] ?? {
      bg: "var(--entity-default-bg)",
      border: "var(--entity-default-border)",
    }
  );
}

/** Short label for an entity type. */
export function entityLabel(entityType: string): string {
  const suffix = entityType.replace("entity:", "");
  return suffix.charAt(0).toUpperCase() + suffix.slice(1);
}

/** A highlight range with source metadata. */
interface HighlightRange {
  start: number;
  end: number;
  kind: "term" | "entity";
  term?: BlockTermMatch;
  entity?: EntityInfo;
}

interface HighlightedSourceProps {
  text: string;
  termMatches: BlockTermMatch[];
  entities?: EntityInfo[];
  onEntityUpdate?: (entity: EntityInfo) => void;
  onEntityDelete?: (entityKey: string) => void;
  onEntityPromote?: (entityKey: string) => void;
}

/** Highlights matched terminology and entities in source text. */
export function HighlightedSource({
  text,
  termMatches,
  entities = [],
  onEntityUpdate,
  onEntityDelete,
  onEntityPromote,
}: HighlightedSourceProps) {
  const [activePopover, setActivePopover] = useState<string | null>(null);

  const handleEntityClick = useCallback((key: string) => {
    setActivePopover((prev) => (prev === key ? null : key));
  }, []);

  if (termMatches.length === 0 && entities.length === 0) return <>{text}</>;

  // Merge term matches and entities into highlight ranges.
  const ranges: HighlightRange[] = [];

  for (const m of termMatches) {
    if (m.start >= 0 && m.end > m.start && m.end <= text.length) {
      ranges.push({ start: m.start, end: m.end, kind: "term", term: m });
    }
  }
  for (const e of entities) {
    if (e.start >= 0 && e.end > e.start && e.end <= text.length) {
      ranges.push({ start: e.start, end: e.end, kind: "entity", entity: e });
    }
  }

  // Sort by start position; entities first when overlapping at same position.
  ranges.sort((a, b) => a.start - b.start || (a.kind === "entity" ? -1 : 1));

  // Remove overlaps — earlier ranges win.
  const resolved: HighlightRange[] = [];
  let lastEnd = 0;
  for (const r of ranges) {
    if (r.start < lastEnd) continue;
    resolved.push(r);
    lastEnd = r.end;
  }

  const parts: React.ReactNode[] = [];
  lastEnd = 0;

  for (const r of resolved) {
    if (r.start > lastEnd) {
      parts.push(<span key={`t-${lastEnd}`}>{text.slice(lastEnd, r.start)}</span>);
    }

    if (r.kind === "term" && r.term) {
      parts.push(
        <span
          key={`h-${r.start}`}
          className="underline decoration-dotted decoration-orange-600 underline-offset-2 cursor-help"
          title={`${r.term.source_term} \u2192 ${r.term.target_terms?.join(", ") || "?"} (${r.term.status})`}
        >
          {text.slice(r.start, r.end)}
        </span>,
      );
    } else if (r.kind === "entity" && r.entity) {
      const colors = getEntityColors(r.entity.type);
      const isActive = activePopover === r.entity.key;
      parts.push(
        <span key={`e-${r.start}`} className="relative inline">
          <span
            className="cursor-pointer rounded-sm px-px"
            style={{
              backgroundColor: colors.bg,
              borderBottom: `2px solid ${colors.border}`,
            }}
            title={`${entityLabel(r.entity.type)}${r.entity.dnt ? " (DNT)" : ""}`}
            onClick={() => handleEntityClick(r.entity!.key)}
          >
            {text.slice(r.start, r.end)}
          </span>
          {isActive && (
            <EntityPopover
              entity={r.entity}
              onClose={() => setActivePopover(null)}
              onUpdate={onEntityUpdate}
              onDelete={onEntityDelete}
              onPromote={onEntityPromote}
            />
          )}
        </span>,
      );
    }

    lastEnd = r.end;
  }

  if (lastEnd < text.length) {
    parts.push(<span key={`t-${lastEnd}`}>{text.slice(lastEnd)}</span>);
  }

  return <>{parts}</>;
}
