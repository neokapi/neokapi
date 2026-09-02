import { useMemo, useState } from "react";
import { ArrowDown, ArrowUp, ArrowUpDown } from "lucide-react";
import { LanguageLabel, cn } from "@neokapi/ui";
import { CoverageBadge } from "./CoverageBadge";
import { TrendSparkline } from "./TrendSparkline";
import {
  formatGapEstimate,
  formatSessions,
  formatShare,
  type LanguageDemand,
} from "./locale-demand-fixtures";

export type LanguageSortKey = "share" | "sessions" | "coverage";
type SortDir = "asc" | "desc";

function coverageRank(l: LanguageDemand): number {
  switch (l.coverage.status) {
    case "covered":
      return 2;
    case "partial":
      return 1;
    case "not-covered":
      return 0;
  }
}

/** Pure sort helper — exported for tests. */
export function sortLanguages(
  languages: LanguageDemand[],
  key: LanguageSortKey,
  dir: SortDir,
): LanguageDemand[] {
  const sign = dir === "desc" ? -1 : 1;
  return [...languages].sort((a, b) => {
    const va = key === "coverage" ? coverageRank(a) : a[key];
    const vb = key === "coverage" ? coverageRank(b) : b[key];
    if (va === vb) return b.sessions - a.sessions;
    return va < vb ? -sign : sign;
  });
}

export interface LanguageDemandTableProps {
  languages: LanguageDemand[];
  selectedLanguage?: string | null;
  onSelectLanguage?: (code: string | null) => void;
  className?: string;
}

function SortHeader({
  label,
  sortKey,
  active,
  dir,
  onSort,
  className,
}: {
  label: string;
  sortKey: LanguageSortKey;
  active: boolean;
  dir: SortDir;
  onSort: (key: LanguageSortKey) => void;
  className?: string;
}) {
  const Icon = active ? (dir === "desc" ? ArrowDown : ArrowUp) : ArrowUpDown;
  return (
    <th className={cn("px-3 py-2 text-left font-medium text-muted-foreground", className)}>
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className="inline-flex cursor-pointer items-center gap-1 bg-transparent p-0 text-inherit hover:text-foreground"
      >
        {label}
        <Icon className={cn("size-3.5", active ? "opacity-100" : "opacity-40")} />
      </button>
    </th>
  );
}

/**
 * Demand vs coverage: one row per demanded language, sortable by demand share,
 * sessions or coverage. Uncovered rows show the mocked plan estimate for
 * closing the gap.
 */
export function LanguageDemandTable({
  languages,
  selectedLanguage,
  onSelectLanguage,
  className,
}: LanguageDemandTableProps) {
  const [sortKey, setSortKey] = useState<LanguageSortKey>("share");
  const [sortDir, setSortDir] = useState<SortDir>("desc");

  const handleSort = (key: LanguageSortKey) => {
    if (key === sortKey) {
      setSortDir((d) => (d === "desc" ? "asc" : "desc"));
    } else {
      setSortKey(key);
      setSortDir("desc");
    }
  };

  const sorted = useMemo(
    () => sortLanguages(languages, sortKey, sortDir),
    [languages, sortKey, sortDir],
  );
  const maxShare = useMemo(
    () => languages.reduce((max, l) => Math.max(max, l.share), 0) || 1,
    [languages],
  );

  return (
    <div className={cn("overflow-x-auto", className)}>
      <table className="w-full border-collapse text-sm" data-testid="language-demand-table">
        <thead>
          <tr className="border-b">
            <th className="px-3 py-2 text-left font-medium text-muted-foreground">Language</th>
            <SortHeader
              label="Demand"
              sortKey="share"
              active={sortKey === "share"}
              dir={sortDir}
              onSort={handleSort}
            />
            <SortHeader
              label="Sessions"
              sortKey="sessions"
              active={sortKey === "sessions"}
              dir={sortDir}
              onSort={handleSort}
            />
            <th className="px-3 py-2 text-left font-medium text-muted-foreground">Trend (12w)</th>
            <SortHeader
              label="Coverage"
              sortKey="coverage"
              active={sortKey === "coverage"}
              dir={sortDir}
              onSort={handleSort}
            />
            <th className="px-3 py-2 text-left font-medium text-muted-foreground">Gap</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((lang) => {
            const isSelected = lang.code === selectedLanguage;
            return (
              <tr
                key={lang.code}
                data-testid={`language-row-${lang.code}`}
                onClick={() => onSelectLanguage?.(isSelected ? null : lang.code)}
                className={cn(
                  "border-b border-border/50 transition-colors",
                  onSelectLanguage && "cursor-pointer hover:bg-muted/50",
                  isSelected && "bg-muted/60",
                )}
              >
                <td className="px-3 py-2">
                  <LanguageLabel code={lang.code} />
                </td>
                <td className="px-3 py-2">
                  <div className="flex items-center gap-2">
                    <span className="w-11 tabular-nums">{formatShare(lang.share)}</span>
                    <div className="h-1.5 w-20 overflow-hidden rounded-full bg-muted">
                      <div
                        className="h-full rounded-full"
                        style={{
                          width: `${(lang.share / maxShare) * 100}%`,
                          background: "var(--chart-1, oklch(0.646 0.222 41.116))",
                        }}
                      />
                    </div>
                  </div>
                </td>
                <td className="px-3 py-2 tabular-nums">{formatSessions(lang.sessions)}</td>
                <td className="px-3 py-2">
                  <TrendSparkline data={lang.trend} />
                </td>
                <td className="px-3 py-2">
                  <CoverageBadge coverage={lang.coverage} />
                </td>
                <td className="px-3 py-2 text-muted-foreground">
                  {lang.coverage.status !== "covered" && lang.planEstimate
                    ? formatGapEstimate(lang.planEstimate)
                    : "·"}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
