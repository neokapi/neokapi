// The scope tuple, rendered as the filter bar (Apache-2.0).
//
// Slicing is a query, never a graph traversal: nodes carry their scope tuple as
// fields, so narrowing by project or locale is the same read with two more
// predicates. That is why one bar can drive every pane — it edits the tuple, and
// the tuple is the question.

import { useMemo } from "react";
import {
  Badge,
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
} from "@neokapi/ui-primitives";
import { useResource } from "@neokapi/concept-ui";
import { t } from "@neokapi/i18n-react/runtime";
import { X } from "lucide-react";
import { useScope } from "./ScopeProvider";
import { FILTERS, LADDER } from "./scope";
import type { Dimension } from "./scope";

const DIMENSION_LABEL: Record<Dimension, () => string> = {
  workspace: () => t("Workspace"),
  project: () => t("Project"),
  stream: () => t("Stream"),
  collection: () => t("Collection"),
  path: () => t("Path"),
  coordinate: () => t("Coordinate"),
  locale: () => t("Language"),
};

/** The label of one dimension, for a picker or a chip. */
export function dimensionLabel(d: Dimension): string {
  return DIMENSION_LABEL[d]();
}

/** The sentinel a select uses for "no value", since an empty string is invalid. */
const ANY = "__any__";

function DimensionSelect({ dimension }: { dimension: Dimension }) {
  const { scope, setDimension, source } = useScope();
  const value = scope[dimension];

  const { data, loading } = useResource(
    () => (source.options ? source.options(scope, dimension) : []),
    // Coarser rungs constrain the options; finer ones do not.
    [dimension, scope.workspace, scope.project, scope.stream, scope.collection],
  );
  const options = data ?? [];

  if (!source.options) {
    return (
      <input
        type="text"
        aria-label={dimensionLabel(dimension)}
        placeholder={dimensionLabel(dimension)}
        defaultValue={value ?? ""}
        data-dimension={dimension}
        className="h-8 w-36 rounded-md border bg-background px-2 text-sm"
        onBlur={(e) => setDimension(dimension, e.target.value.trim() || undefined)}
      />
    );
  }

  return (
    <Select
      value={value ?? ANY}
      onValueChange={(next) => setDimension(dimension, next === ANY ? undefined : next)}
    >
      <SelectTrigger
        size="sm"
        className="w-40"
        aria-label={dimensionLabel(dimension)}
        data-dimension={dimension}
      >
        <SelectValue placeholder={dimensionLabel(dimension)} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ANY}>
          {loading ? t("Loading…") : t("Any {dimension}", { dimension: dimensionLabel(dimension) })}
        </SelectItem>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label ?? option.value}
            {option.count !== undefined && (
              <span className="ml-2 text-xs text-muted-foreground">{option.count}</span>
            )}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export interface ScopeFilterBarProps {
  className?: string;
}

export function ScopeFilterBar({ className }: ScopeFilterBarProps) {
  const { scope, free, pinned, setDimension } = useScope();

  // The ladder's free rungs first, then the two filters — the same order a
  // reader walks when narrowing from "everything" to "this point in this
  // language".
  const movable = useMemo(
    () => [...LADDER, ...FILTERS].filter((d) => free.includes(d) && d !== "path"),
    [free.join(",")],
  );

  const pinnedChips = pinned.filter((d) => scope[d]);
  const active = movable.filter((d) => scope[d]);

  return (
    <div
      className={cn("flex flex-wrap items-center gap-2", className)}
      data-testid="scope-filter-bar"
    >
      {pinnedChips.map((d) => (
        <Badge key={d} variant="secondary" className="font-normal" data-pinned={d}>
          <span className="text-muted-foreground">{dimensionLabel(d)}</span>
          <span className="ml-1 font-medium">{scope[d]}</span>
        </Badge>
      ))}

      {movable.map((d) => (
        <DimensionSelect key={d} dimension={d} />
      ))}

      {active.length > 0 && (
        <Button
          variant="ghost"
          size="sm"
          data-testid="clear-filters"
          onClick={() => active.forEach((d) => setDimension(d, undefined))}
        >
          <X className="size-3.5" />
          {t("Clear")}
        </Button>
      )}
    </div>
  );
}
