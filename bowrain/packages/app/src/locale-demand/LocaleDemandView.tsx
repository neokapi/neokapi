import { useEffect, useState, type ReactNode } from "react";
import {
  Badge,
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
} from "@neokapi/ui";
import {
  TIME_RANGES,
  demandProvenanceLabel,
  sampleDemandDataSource,
  type DemandDataSource,
  type DemandSnapshot,
  type TimeRange,
} from "./locale-demand-fixtures";
import { WorldDemandMap } from "./WorldDemandMap";
import { LanguageDemandTable } from "./LanguageDemandTable";
import { DemandDrillDownPanel, type DrillDownSelection } from "./DemandDrillDownPanel";
import { SampleDataMark, SampleDataNotice } from "./SampleDataNotice";

export interface LocaleDemandViewProps {
  /** Sample fixtures by default; the route swaps in PostHogDemandSource when connected. */
  dataSource?: DemandDataSource;
  initialRange?: TimeRange;
  initialSelection?: DrillDownSelection | null;
  /** Connect affordance for the sample-data notice; the page supplies the button. */
  connectAction?: ReactNode;
  /** Set when a configured PostHog source failed and the view fell back to the fixture. */
  degradedReason?: string;
  /** Called when an async source fails; the previous snapshot stays on screen. */
  onSnapshotError?: (err: unknown) => void;
}

/**
 * Locale demand — answers "which market or language should we ship next?" by
 * joining end-user language demand against the project's current locale
 * coverage. Runs on the sample dataset until an analytics source
 * (PostHog) is connected; the provenance footer always says which one is on
 * screen. Router-free so Storybook can render the whole page.
 *
 * The sample-data label follows the snapshot, not the caller: whatever puts
 * the fixture on screen also puts the notice and the per-surface marks there.
 */
export function LocaleDemandView({
  dataSource = sampleDemandDataSource,
  initialRange = "30d",
  initialSelection = null,
  connectAction,
  degradedReason,
  onSnapshotError,
}: LocaleDemandViewProps) {
  const [range, setRange] = useState<TimeRange>(initialRange);
  const [selection, setSelection] = useState<DrillDownSelection | null>(initialSelection);
  // null while an async source loads its first snapshot; sync sources are
  // settled by the effect before first paint.
  const [snapshot, setSnapshot] = useState<DemandSnapshot | null>(null);

  useEffect(() => {
    let cancelled = false;
    const result = dataSource.getSnapshot(range);
    if (result instanceof Promise) {
      result.then(
        (snap) => {
          if (!cancelled) setSnapshot(snap);
        },
        (err) => {
          if (!cancelled) onSnapshotError?.(err);
        },
      );
    } else {
      setSnapshot(result);
    }
    return () => {
      cancelled = true;
    };
    // onSnapshotError is intentionally not a dependency: callers pass inline
    // closures and re-fetching on their identity would loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dataSource, range]);

  const isSample = snapshot?.provenance.kind === "sample";

  return (
    <div className="mx-auto w-full max-w-7xl p-6">
      <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            Locale demand
            <Badge variant="secondary" className="font-normal" data-testid="labs-preview-badge">
              Labs preview
            </Badge>
            {snapshot?.provenance.kind === "posthog" && (
              <Badge
                variant="outline"
                className="border-transparent bg-emerald-500/15 font-normal text-emerald-700 dark:text-emerald-400"
                data-testid="live-data-badge"
              >
                Live · PostHog
              </Badge>
            )}
          </h1>
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

      {isSample && <SampleDataNotice action={connectAction} degradedReason={degradedReason} />}

      {!snapshot ? (
        <div className="space-y-4" data-testid="demand-loading">
          <Skeleton className="h-[560px] w-full rounded-xl" />
          <Skeleton className="h-72 w-full rounded-xl" />
        </div>
      ) : (
        <div className="flex items-start gap-4">
          <div className="min-w-0 flex-1 space-y-4">
            <Card>
              <CardHeader>
                <CardTitle>Demand by market</CardTitle>
                <CardDescription>
                  Share of {snapshot.totalSessions.toLocaleString("en-US")} sessions by country.
                  Click a market to drill down.
                </CardDescription>
              </CardHeader>
              <CardContent>
                {/* The mark rides on the map itself: a crop of the graphic carries it. */}
                <div className="relative">
                  <WorldDemandMap
                    countries={snapshot.countries}
                    selectedCountry={selection?.kind === "country" ? selection.code : null}
                    onSelectCountry={(code) =>
                      setSelection(code ? { kind: "country", code } : null)
                    }
                  />
                  {isSample && <SampleDataMark className="absolute left-2 top-2" />}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Demand vs coverage</CardTitle>
                <CardDescription>
                  Demanded languages against the project's current locales. Click a row to drill
                  down.
                </CardDescription>
                {isSample && (
                  <CardAction>
                    <SampleDataMark />
                  </CardAction>
                )}
              </CardHeader>
              <CardContent>
                <LanguageDemandTable
                  languages={snapshot.languages}
                  selectedLanguage={selection?.kind === "language" ? selection.code : null}
                  onSelectLanguage={(code) =>
                    setSelection(code ? { kind: "language", code } : null)
                  }
                />
              </CardContent>
            </Card>

            <p
              className="px-1 text-[11px] text-muted-foreground"
              data-testid="page-demand-provenance"
            >
              {demandProvenanceLabel(snapshot.provenance)}
            </p>
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
      )}
    </div>
  );
}
