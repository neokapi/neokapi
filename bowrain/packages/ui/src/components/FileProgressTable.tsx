import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  cn,
  LocaleLabel,
} from "@neokapi/ui-primitives";
import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";
import { useState, useMemo } from "react";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import type { DashboardItemSort, ItemTranslationStats } from "../types/api";

import { FormattedFileName } from "./FormattedFileName";
import { ListCapRow } from "./ListCapRow";
import { itemDisplayPath, relativeItemName } from "./collections/itemBase";
import { MonitorPlay } from "./icons";

type SortField = DashboardItemSort;
type SortDir = "asc" | "desc";

/**
 * Server-side paging/sorting seam. When present the table is controlled: rows
 * arrive pre-sorted and pre-paged from the dashboard endpoint, header clicks
 * request a new server sort, and "Load more" grows the page — the full
 * O(files × locales) list never reaches the client. When absent the table
 * sorts its full `itemStats` client-side (stories, adapters without paging).
 */
export interface FileProgressPaging {
  /** Full item count across all pages (item_total). */
  total: number;
  sortField: SortField;
  sortDir: SortDir;
  /** Request a different server-side sort (resets to the first page). */
  onSortChange: (field: SortField, dir: SortDir) => void;
  /** More rows exist beyond the loaded page. */
  hasMore: boolean;
  /** Load the next page of rows. */
  onLoadMore: () => void;
  /** A sort/page fetch is in flight. */
  isLoading?: boolean;
}

interface FileProgressTableProps {
  itemStats: ItemTranslationStats[];
  locales: string[];
  localeDisplayNames?: Record<string, string>;
  /**
   * Directory prefix shared by every item in scope (`item_base`), trailing
   * slash included. Names render relative to it — a collection declared with a
   * `base:` otherwise repeats that base on every row, which is the one part of
   * the path that cannot tell two files apart. The full name stays in the row's
   * tooltip and is what every callback still carries.
   */
  itemBase?: string;
  /** Server-side paging/sort; omit for client-side sorting of the full list. */
  paging?: FileProgressPaging;
  /**
   * Open one item. Given, the name cell becomes the way into that item's
   * content; omitted, the table stays a read-only report.
   */
  onOpenItem?: (item: ItemTranslationStats) => void;
  /**
   * Which rows can be read inside the component that ships them
   * (`useInContextItems`), asked of the item's source path. A collection
   * declaring a preview host does not mean every item in it has a view — a
   * Storybook renders what someone wrote a story for — so the rows that do are
   * marked, and the rest left plain. Omitted, no row is marked.
   */
  inContext?: (sourcePath?: string) => boolean;
}

/** Hard render cap — large projects can hold thousands of files. */
const MAX_ROWS = 500;

/**
 * A sortable column heading. The direction is an icon rather than an arrow
 * glued onto the label — a string concatenated into the heading becomes part of
 * the column's name, so "Words" read as "Words ↑" to anything computing an
 * accessible name, and as "Wordsnull" wherever the inactive state's absent
 * indicator was stringified instead of skipped. `aria-sort` carries the state
 * where it belongs, and the button makes the heading reachable by keyboard.
 */
function SortHeader({
  label,
  field,
  sortField,
  sortDir,
  onSort,
  className,
}: {
  label: string;
  field: SortField;
  sortField: SortField;
  sortDir: SortDir;
  onSort: (field: SortField) => void;
  className?: string;
}) {
  const active = sortField === field;
  const Icon = active ? (sortDir === "asc" ? ArrowUp : ArrowDown) : ArrowUpDown;
  return (
    <th
      className={cn("py-2 font-medium text-muted-foreground", className)}
      aria-sort={active ? (sortDir === "asc" ? "ascending" : "descending") : "none"}
    >
      <button
        type="button"
        onClick={() => onSort(field)}
        className="inline-flex cursor-pointer items-center gap-1 bg-transparent p-0 text-inherit hover:text-foreground"
      >
        {label}
        <Icon aria-hidden="true" className={cn("size-3", active ? "opacity-100" : "opacity-40")} />
      </button>
    </th>
  );
}

/**
 * Column widths, in pixels, shared by every surface that shows this table.
 *
 * They are declared rather than derived because an auto-laid-out table sizes
 * its columns from the rows it happens to be showing: the same table read
 * "Format | Words | Avg %" at one width on a project's overview and another on
 * a collection, and shifted again on the next page of the same collection.
 * Columns that move between two views of one list read as two different tables.
 *
 * The name column is deliberately absent — it takes whatever is left, so the
 * part that varies in length is the part with room to vary.
 */
const COL_WIDTH = { format: 96, words: 96, avg: 80, locale: 120 } as const;

/**
 * Floor for the name column, below which a path truncates to nothing useful.
 *
 * Kept tight on purpose: it is the point at which the card starts scrolling
 * sideways, and a floor set to what a comfortable name wants rather than what a
 * legible one needs put a horizontal scrollbar under tables that had room to
 * spare. Above it the column takes everything left over.
 */
const NAME_MIN_WIDTH = 160;

function completionBarColor(pct: number): string {
  if (pct >= 90) return "bg-success";
  if (pct >= 50) return "bg-warning";
  if (pct > 0) return "bg-warning";
  return "bg-muted";
}

export function FileProgressTable({
  itemStats,
  locales,
  localeDisplayNames,
  itemBase,
  paging,
  onOpenItem,
  inContext,
}: FileProgressTableProps) {
  const [localSortField, setLocalSortField] = useState<SortField>("name");
  const [localSortDir, setLocalSortDir] = useState<SortDir>("asc");

  const sortField = paging ? paging.sortField : localSortField;
  const sortDir = paging ? paging.sortDir : localSortDir;

  const toggleSort = (field: SortField) => {
    const nextDir: SortDir = sortField === field ? (sortDir === "asc" ? "desc" : "asc") : "asc";
    if (paging) {
      paging.onSortChange(field, nextDir);
      return;
    }
    setLocalSortField(field);
    setLocalSortDir(nextDir);
  };

  const sorted = useMemo(() => {
    // Controlled mode: rows are already server-sorted and paged.
    if (paging) return itemStats;
    const items = [...itemStats];
    items.sort((a, b) => {
      let cmp = 0;
      switch (sortField) {
        case "name":
          cmp = a.item_name.localeCompare(b.item_name);
          break;
        case "words":
          cmp = a.word_count - b.word_count;
          break;
        case "completion": {
          const avgA =
            a.locales.length > 0
              ? a.locales.reduce((s, l) => s + l.percentage, 0) / a.locales.length
              : 0;
          const avgB =
            b.locales.length > 0
              ? b.locales.reduce((s, l) => s + l.percentage, 0) / b.locales.length
              : 0;
          cmp = avgA - avgB;
          break;
        }
      }
      return sortDir === "desc" ? -cmp : cmp;
    });
    return items;
  }, [itemStats, sortField, sortDir, paging]);

  // How a row reads: the source file when the item is a generated catalog,
  // otherwise the item itself — under the base the whole scope shares. A
  // reading of the name and never a change to it: `item_name` stays in the
  // tooltip and is what every callback carries.
  const displayName = (item: ItemTranslationStats) =>
    relativeItemName(itemDisplayPath(item.item_name, item.source_path), itemBase ?? "");

  // Hard cap so a project with thousands of files never floods the DOM; the
  // ListCapRow below makes the cut honest. Server paging already bounds rows.
  const visibleRows = sorted.length > MAX_ROWS ? sorted.slice(0, MAX_ROWS) : sorted;
  const totalRows = paging ? paging.total : sorted.length;

  // The declared columns plus a floor for the name. Below this the card scrolls
  // sideways rather than compressing the columns, so a project with many target
  // locales reads the same as one with two.
  const minTableWidth =
    NAME_MIN_WIDTH +
    COL_WIDTH.format +
    COL_WIDTH.words +
    COL_WIDTH.avg +
    locales.length * COL_WIDTH.locale;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">File Progress</CardTitle>
      </CardHeader>
      <CardContent>
        {/* Scroll containment: the table scrolls inside the card, never the page. */}
        <div className="max-h-[70vh] overflow-auto">
          <table className="w-full table-fixed text-xs" style={{ minWidth: minTableWidth }}>
            {/* Widths live here, once, rather than on the cells — every surface
                showing this table then lays it out identically. */}
            <colgroup>
              <col />
              <col style={{ width: COL_WIDTH.format }} />
              <col style={{ width: COL_WIDTH.words }} />
              <col style={{ width: COL_WIDTH.avg }} />
              {locales.map((l) => (
                <col key={l} style={{ width: COL_WIDTH.locale }} />
              ))}
            </colgroup>
            <thead className="sticky top-0 z-10 bg-card">
              <tr className="border-b">
                <SortHeader
                  label="File"
                  field="name"
                  sortField={sortField}
                  sortDir={sortDir}
                  onSort={toggleSort}
                  className="pr-3 text-left"
                />
                <th className="px-2 py-2 text-left font-medium text-muted-foreground">Format</th>
                <SortHeader
                  label="Words"
                  field="words"
                  sortField={sortField}
                  sortDir={sortDir}
                  onSort={toggleSort}
                  className="px-2 text-right"
                />
                <SortHeader
                  label="Avg %"
                  field="completion"
                  sortField={sortField}
                  sortDir={sortDir}
                  onSort={toggleSort}
                  className="px-2 text-right"
                />
                {locales.map((l) => (
                  <th key={l} className="px-1 py-2 text-center font-medium text-muted-foreground">
                    <LocaleLabel
                      locale={l}
                      displayName={localeDisplayNames?.[l]}
                      variant="short"
                      hideCode
                    />
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className={cn(paging?.isLoading && "opacity-60 transition-opacity")}>
              {visibleRows.map((item) => {
                const localeMap = new Map(item.locales.map((l) => [l.locale, l]));
                const avgPct =
                  item.locales.length > 0
                    ? Math.round(
                        item.locales.reduce((s, l) => s + l.percentage, 0) / item.locales.length,
                      )
                    : 0;

                return (
                  <tr key={item.item_id} className="border-b last:border-0">
                    <td className="py-2 pr-3 font-medium" title={item.item_name}>
                      <span className="flex min-w-0 items-center gap-1.5">
                        {onOpenItem ? (
                          <button
                            type="button"
                            onClick={() => onOpenItem(item)}
                            className="min-w-0 flex-1 truncate text-left hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          >
                            <FormattedFileName
                              name={displayName(item)}
                              format={item.format}
                              iconClassName="w-3.5 h-3.5 shrink-0"
                            />
                          </button>
                        ) : (
                          <span className="min-w-0 flex-1 truncate">
                            <FormattedFileName
                              name={displayName(item)}
                              format={item.format}
                              iconClassName="w-3.5 h-3.5 shrink-0"
                            />
                          </span>
                        )}
                        {/* Marked, never dimmed: a row without a story is not
                            worse content, it is content with no component
                            published beside it. */}
                        {inContext?.(item.source_path) && (
                          <MonitorPlay
                            className="size-3.5 shrink-0 text-muted-foreground"
                            aria-label="Can be read in context"
                            data-testid="file-in-context"
                          />
                        )}
                      </span>
                    </td>
                    <td className="truncate px-2 py-2 text-muted-foreground">{item.format}</td>
                    <td className="px-2 py-2 text-right tabular-nums text-muted-foreground">
                      {item.word_count.toLocaleString()}
                    </td>
                    <td className="px-2 py-2 text-right tabular-nums">{avgPct}%</td>
                    {locales.map((locale) => {
                      const ls = localeMap.get(locale);
                      const pct = ls ? Math.round(ls.percentage) : 0;
                      return (
                        <td key={locale} className="px-1 py-2">
                          <div className="flex items-center gap-1.5">
                            <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                              <div
                                className={cn(
                                  "h-full rounded-full transition-all",
                                  completionBarColor(pct),
                                )}
                                style={{ width: `${pct}%` }}
                              />
                            </div>
                            <span className="w-8 text-right text-[10px] tabular-nums text-muted-foreground">
                              {pct}%
                            </span>
                          </div>
                        </td>
                      );
                    })}
                  </tr>
                );
              })}
            </tbody>
          </table>
          {paging ? (
            <div className="flex items-center justify-between gap-3 py-2 text-[11px] text-muted-foreground">
              <span data-testid="file-progress-count">
                <Plural count={totalRows}>
                  <One>
                    Showing {visibleRows.length} of {totalRows} file
                  </One>
                  <Other>
                    Showing {visibleRows.length} of {totalRows} files
                  </Other>
                </Plural>
              </span>
              {paging.hasMore && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={paging.onLoadMore}
                  disabled={paging.isLoading}
                  data-testid="file-progress-load-more"
                >
                  {paging.isLoading ? "Loading…" : "Load more"}
                </Button>
              )}
            </div>
          ) : (
            <ListCapRow
              shown={visibleRows.length}
              total={sorted.length}
              noun="files"
              hint="Sort or narrow the project to see the rest."
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
}
