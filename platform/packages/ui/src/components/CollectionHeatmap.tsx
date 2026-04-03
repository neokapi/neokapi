import type { CollectionTranslationStats } from "../types/api";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
} from "@neokapi/ui-primitives/components/ui/card";
import { cn } from "@neokapi/ui-primitives";

interface CollectionHeatmapProps {
  collectionStats: CollectionTranslationStats[];
  locales: string[];
}

function completionColor(pct: number): string {
  if (pct >= 90) return "bg-green-500/20 text-green-700 dark:text-green-400";
  if (pct >= 50) return "bg-yellow-500/20 text-yellow-700 dark:text-yellow-400";
  if (pct > 0) return "bg-orange-500/20 text-orange-700 dark:text-orange-400";
  return "bg-muted text-muted-foreground";
}

export function CollectionHeatmap({ collectionStats, locales }: CollectionHeatmapProps) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Collection Progress</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b">
                <th className="py-2 pr-3 text-left font-medium text-muted-foreground">
                  Collection
                </th>
                <th className="px-2 py-2 text-right font-medium text-muted-foreground">Files</th>
                <th className="px-2 py-2 text-right font-medium text-muted-foreground">Words</th>
                {locales.map((l) => (
                  <th
                    key={l}
                    className="min-w-[60px] px-1 py-2 text-center font-medium text-muted-foreground"
                  >
                    {l}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {collectionStats.map((coll) => {
                const localeMap = new Map(coll.locales.map((l) => [l.locale, l]));
                return (
                  <tr key={coll.collection_id} className="border-b last:border-0">
                    <td className="py-2 pr-3 font-medium">{coll.collection_name || "Default"}</td>
                    <td className="px-2 py-2 text-right tabular-nums text-muted-foreground">
                      {coll.item_count}
                    </td>
                    <td className="px-2 py-2 text-right tabular-nums text-muted-foreground">
                      {coll.word_count.toLocaleString()}
                    </td>
                    {locales.map((locale) => {
                      const ls = localeMap.get(locale);
                      const pct = ls ? Math.round(ls.percentage) : 0;
                      return (
                        <td key={locale} className="px-1 py-2 text-center">
                          <span
                            className={cn(
                              "inline-block min-w-[48px] rounded px-1.5 py-0.5 text-[10px] font-medium tabular-nums",
                              completionColor(pct),
                            )}
                          >
                            {pct}%
                          </span>
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
