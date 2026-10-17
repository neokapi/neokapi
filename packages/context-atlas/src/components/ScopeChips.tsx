import { t } from "@neokapi/i18n-react/runtime";
import { Button, CoordinateChip } from "@neokapi/ui-primitives";
import { X } from "lucide-react";
import { axisLabel, formatCount, valueLabel } from "../labels";
import type { Atlas } from "../model/atlas";
import { type AtlasQuery, hasFilters, withoutFilter, withoutFilters } from "../model/query";

// CoordinateChip knows four axes by hue; a locale reads as a language.
const CHIP_AXIS: Record<string, string> = { locale: "language" };

export interface ScopeChipsProps {
  atlas: Atlas;
  query: AtlasQuery;
  onQuery: (next: AtlasQuery) => void;
}

/**
 * What the query retains, one removable chip per value. Values on one axis
 * are alternatives (OR); axes combine (AND), and the chips say so by grouping.
 */
export function ScopeChips({ atlas, query, onQuery }: ScopeChipsProps) {
  const entries = Object.entries(query.filters).filter(([, v]) => v.length > 0);
  return (
    <div
      className="flex min-h-7 flex-wrap items-center gap-x-3 gap-y-1.5"
      data-testid="atlas-scope"
    >
      <span className="text-[11px] text-muted-foreground uppercase">Scope</span>
      {!hasFilters(query) && (
        <span className="text-sm text-muted-foreground">Everything recorded by the snapshot</span>
      )}
      {entries.map(([axis, values]) => (
        <span
          key={axis}
          role="group"
          aria-label={axisLabel(atlas, axis)}
          className="flex flex-wrap items-center gap-1"
        >
          <span aria-hidden className="text-[11px] text-muted-foreground">
            {axisLabel(atlas, axis)}
          </span>
          {values.map((value, i) => (
            <span key={value} className="flex items-center gap-1">
              {i > 0 && <span className="text-[11px] text-muted-foreground">or</span>}
              <CoordinateChip
                axis={CHIP_AXIS[axis] ?? axis}
                label={axisLabel(atlas, axis)}
                value={valueLabel(atlas, axis, value)}
                onRemove={() => onQuery(withoutFilter(query, axis, value))}
              />
            </span>
          ))}
        </span>
      ))}
      {query.ids.length > 0 && (
        <span className="inline-flex h-5 items-center gap-1 rounded-4xl bg-axis-unknown pr-1 pl-2 text-xs font-medium text-axis-unknown-foreground">
          {t("{count} chosen records", "scope chip: an explicit set of records", {
            count: formatCount(query.ids.length),
          })}
          <button
            type="button"
            aria-label={t("Remove the chosen records", "scope chip")}
            className="cursor-pointer rounded-full p-0.5 opacity-70 hover:opacity-100"
            onClick={() => onQuery({ ...query, ids: [] })}
          >
            <X aria-hidden className="size-3" />
          </button>
        </span>
      )}
      {hasFilters(query) && (
        <Button variant="ghost" size="xs" onClick={() => onQuery(withoutFilters(query))}>
          Clear filters
        </Button>
      )}
    </div>
  );
}
