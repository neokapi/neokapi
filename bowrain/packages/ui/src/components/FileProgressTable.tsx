import { Button, Card, CardContent, CardHeader, CardTitle, cn } from "@neokapi/ui-primitives";
import { useState, useMemo } from "react";
import type { DashboardItemSort, ItemTranslationStats } from "../types/api";
import { LanguageLabel } from "./LanguageLabel";
import { FormattedFileName } from "./FormattedFileName";
import { ListCapRow } from "./ListCapRow";

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
  /** Server-side paging/sort; omit for client-side sorting of the full list. */
  paging?: FileProgressPaging;
}

/** Hard render cap — large projects can hold thousands of files. */
const MAX_ROWS = 500;

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
  paging,
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

  const sortIndicator = (field: SortField) => {
    if (sortField !== field) return null;
    return sortDir === "asc" ? " ↑" : " ↓";
  };

  // Hard cap so a project with thousands of files never floods the DOM; the
  // ListCapRow below makes the cut honest. Server paging already bounds rows.
  const visibleRows = sorted.length > MAX_ROWS ? sorted.slice(0, MAX_ROWS) : sorted;
  const totalRows = paging ? paging.total : sorted.length;

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">File Progress</CardTitle>
      </CardHeader>
      <CardContent>
        {/* Scroll containment: the table scrolls inside the card, never the page. */}
        <div className="max-h-[70vh] overflow-auto">
          <table className="w-full text-xs">
            <thead className="sticky top-0 z-10 bg-card">
              <tr className="border-b">
                <th
                  className="cursor-pointer py-2 pr-3 text-left font-medium text-muted-foreground hover:text-foreground"
                  onClick={() => toggleSort("name")}
                >
                  File{sortIndicator("name")}
                </th>
                <th className="px-2 py-2 text-left font-medium text-muted-foreground">Format</th>
                <th
                  className="cursor-pointer px-2 py-2 text-right font-medium text-muted-foreground hover:text-foreground"
                  onClick={() => toggleSort("words")}
                >
                  Words{sortIndicator("words")}
                </th>
                <th
                  className="cursor-pointer px-2 py-2 text-right font-medium text-muted-foreground hover:text-foreground"
                  onClick={() => toggleSort("completion")}
                >
                  Avg %{sortIndicator("completion")}
                </th>
                {locales.map((l) => (
                  <th
                    key={l}
                    className="min-w-[80px] px-1 py-2 text-center font-medium text-muted-foreground"
                  >
                    <LanguageLabel
                      code={l}
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
                    <td
                      className="max-w-[200px] truncate py-2 pr-3 font-medium"
                      title={item.item_name}
                    >
                      <FormattedFileName
                        name={item.item_name}
                        format={item.format}
                        iconClassName="w-3.5 h-3.5 shrink-0"
                      />
                    </td>
                    <td className="px-2 py-2 text-muted-foreground">{item.format}</td>
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
                Showing {visibleRows.length} of {totalRows} file{totalRows !== 1 ? "s" : ""}
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
