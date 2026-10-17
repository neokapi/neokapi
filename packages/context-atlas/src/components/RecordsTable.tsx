import { useMemo, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  Input,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  cn,
} from "@neokapi/ui-primitives";
import { Search } from "lucide-react";
import { axisLabel, formatCount, formatDay, valueLabel } from "../labels";
import { type Atlas, coordinateOf, preparedStateAt } from "../model/atlas";
import type { EvalContext } from "../model/filter";
import type { AtlasQuery } from "../model/query";
import { dayOf } from "../model/time";
import { StateBadge } from "./StateBadge";

const PAGE = 50;

export interface RecordsTableProps {
  atlas: Atlas;
  query: AtlasQuery;
  ctx: EvalContext;
  foreground: readonly number[];
  selected: string | null;
  onSelect: (id: string) => void;
}

/**
 * The records behind every count, one row each, with their state at the
 * snapshot. The search box narrows these rows only: the counts, the other
 * views and an export keep the full selection.
 */
export function RecordsTable({
  atlas,
  query,
  ctx,
  foreground,
  selected,
  onSelect,
}: RecordsTableProps) {
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const columns = query.flowAxes
    .filter((a) => atlas.axisById.get(a)?.origin !== "view" && a !== "family")
    .slice(0, 2);

  const rows = useMemo(() => {
    const needle = search.trim().toLocaleLowerCase();
    const list = foreground.map((i) => atlas.records[i]);
    const hit = needle
      ? list.filter((p) =>
          [p.record.label, p.record.id, p.record.text ?? "", p.record.short ?? ""].some((s) =>
            s.toLocaleLowerCase().includes(needle),
          ),
        )
      : list;
    return hit.sort((a, b) => b.recordedMs - a.recordedMs || a.index - b.index);
  }, [atlas, foreground, search]);

  const pages = Math.max(1, Math.ceil(rows.length / PAGE));
  const current = Math.min(page, pages - 1);
  const shown = rows.slice(current * PAGE, current * PAGE + PAGE);

  return (
    <div className="flex flex-col gap-3" data-testid="atlas-records">
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative w-64">
          <Search
            aria-hidden
            className="pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            type="search"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(0);
            }}
            placeholder={t("Search this table", "records table search")}
            aria-label={t("Search this table only", "records table search")}
            aria-describedby="atlas-records-search-scope"
            className="h-8 pl-7 text-sm"
          />
        </div>
        <span
          id="atlas-records-search-scope"
          className="text-[11px] text-muted-foreground tabular-nums"
        >
          {search.trim()
            ? t(
                "{shown} of {total} rows match; search narrows this table only",
                "records table count",
                {
                  shown: formatCount(rows.length),
                  total: formatCount(foreground.length),
                },
              )
            : t("{total} rows; search narrows this table only", "records table count", {
                total: formatCount(foreground.length),
              })}
        </span>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Record</TableHead>
            <TableHead>State at snapshot</TableHead>
            {columns.map((a) => (
              <TableHead key={a}>{axisLabel(atlas, a)}</TableHead>
            ))}
            <TableHead className="text-right">Recorded</TableHead>
            {atlas.weighted && <TableHead className="text-right">Units</TableHead>}
          </TableRow>
        </TableHeader>
        <TableBody>
          {shown.map((p) => {
            const active = selected === p.record.id;
            return (
              <TableRow
                key={p.record.id}
                data-state={active ? "selected" : undefined}
                className={cn("cursor-pointer", active && "bg-muted")}
                onClick={() => onSelect(p.record.id)}
              >
                <TableCell className="max-w-80">
                  <button
                    type="button"
                    className="block w-full min-w-0 truncate text-left text-sm outline-none focus-visible:underline"
                    title={p.record.label}
                    onClick={(e) => {
                      e.stopPropagation();
                      onSelect(p.record.id);
                    }}
                  >
                    {p.record.label}
                  </button>
                  <span className="block truncate text-[11px] text-muted-foreground">
                    {valueLabel(atlas, "family", p.record.family)}
                    {p.record.short ? ` · ${p.record.short}` : ""}
                  </span>
                </TableCell>
                <TableCell>
                  <StateBadge state={preparedStateAt(p, ctx.snapshotMs)} compact />
                </TableCell>
                {columns.map((a) => (
                  <TableCell key={a} className="max-w-40 truncate text-sm">
                    {valueLabel(atlas, a, coordinateOf(p, a))}
                  </TableCell>
                ))}
                <TableCell className="text-right text-sm text-muted-foreground tabular-nums">
                  {Number.isNaN(p.recordedMs) ? "–" : formatDay(dayOf(p.recordedMs))}
                </TableCell>
                {atlas.weighted && (
                  <TableCell className="text-right text-sm tabular-nums">
                    {formatCount(p.record.weight || 1)}
                  </TableCell>
                )}
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
      {pages > 1 && (
        <div className="flex items-center justify-end gap-2">
          <span className="text-[11px] text-muted-foreground tabular-nums">
            {t("Page {page} of {pages}", "records table pagination", { page: current + 1, pages })}
          </span>
          <Button
            variant="outline"
            size="xs"
            disabled={current === 0}
            onClick={() => setPage(current - 1)}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            size="xs"
            disabled={current >= pages - 1}
            onClick={() => setPage(current + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
