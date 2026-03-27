import { useState, useMemo } from "react";
import type { ItemTranslationStats } from "../types/api";
import { Card, CardHeader, CardTitle, CardContent } from "./ui/card";
import { cn } from "../lib/utils";

interface FileProgressTableProps {
  itemStats: ItemTranslationStats[];
  locales: string[];
  localeDisplayNames?: Record<string, string>;
}

type SortField = "name" | "words" | "completion";
type SortDir = "asc" | "desc";

function completionBarColor(pct: number): string {
  if (pct >= 90) return "bg-green-500";
  if (pct >= 50) return "bg-yellow-500";
  if (pct > 0) return "bg-orange-500";
  return "bg-muted";
}

export function FileProgressTable({ itemStats, locales, localeDisplayNames }: FileProgressTableProps) {
  const [sortField, setSortField] = useState<SortField>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");

  const toggleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortField(field);
      setSortDir("asc");
    }
  };

  const sorted = useMemo(() => {
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
  }, [itemStats, sortField, sortDir]);

  const sortIndicator = (field: SortField) => {
    if (sortField !== field) return null;
    return sortDir === "asc" ? " \u2191" : " \u2193";
  };

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">File Progress</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
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
                    {localeDisplayNames?.[l] ?? l}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {sorted.map((item) => {
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
                      {item.item_name}
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
        </div>
      </CardContent>
    </Card>
  );
}
