import { useMemo, useState } from "react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui";
import {
  TIME_RANGES,
  sampleDemandDataSource,
  type DemandDataSource,
  type TimeRange,
} from "./locale-demand-fixtures";
import { WorldDemandMap } from "./WorldDemandMap";
import { LanguageDemandTable } from "./LanguageDemandTable";
import { DemandDrillDownPanel, type DrillDownSelection } from "./DemandDrillDownPanel";

export interface LocaleDemandViewProps {
  /** Swap in the real ingest-backed source when it exists. */
  dataSource?: DemandDataSource;
  initialRange?: TimeRange;
  initialSelection?: DrillDownSelection | null;
}

/**
 * Locale demand — DESIGN PROTOTYPE (mock data only).
 *
 * Answers "which market/language should we localize for next?" by joining
 * end-user language demand against the project's current locale coverage and
 * pricing the gap. Router-free so Storybook can render the whole page.
 */
export function LocaleDemandView({
  dataSource = sampleDemandDataSource,
  initialRange = "30d",
  initialSelection = null,
}: LocaleDemandViewProps) {
  const [range, setRange] = useState<TimeRange>(initialRange);
  const [selection, setSelection] = useState<DrillDownSelection | null>(initialSelection);

  const snapshot = useMemo(() => dataSource.getSnapshot(range), [dataSource, range]);

  return (
    <div className="mx-auto w-full max-w-7xl p-6">
      <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">Locale demand</h1>
          <p className="text-sm text-muted-foreground">
            Where your users are asking for languages you don't ship yet.
          </p>
        </div>
        <Select value={range} onValueChange={(v) => setRange(v as TimeRange)}>
          <SelectTrigger className="w-40" data-testid="time-range-select">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIME_RANGES.map((r) => (
              <SelectItem key={r.value} value={r.value}>
                {r.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex items-start gap-4">
        <div className="min-w-0 flex-1 space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Demand by market</CardTitle>
              <CardDescription>
                Share of {snapshot.totalSessions.toLocaleString("en-US")} sessions by country —
                click a market to drill down.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <WorldDemandMap
                countries={snapshot.countries}
                selectedCountry={selection?.kind === "country" ? selection.code : null}
                onSelectCountry={(code) => setSelection(code ? { kind: "country", code } : null)}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Demand vs coverage</CardTitle>
              <CardDescription>
                Demanded languages against the project's current locales — click a row to drill
                down.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <LanguageDemandTable
                languages={snapshot.languages}
                selectedLanguage={selection?.kind === "language" ? selection.code : null}
                onSelectLanguage={(code) => setSelection(code ? { kind: "language", code } : null)}
              />
            </CardContent>
          </Card>
        </div>

        {selection && (
          <DemandDrillDownPanel
            snapshot={snapshot}
            selection={selection}
            onClose={() => setSelection(null)}
            className="sticky top-4"
          />
        )}
      </div>
    </div>
  );
}
