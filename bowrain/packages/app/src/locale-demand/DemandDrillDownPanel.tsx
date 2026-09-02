import { useMemo } from "react";
import { X } from "lucide-react";
import { Area, AreaChart, ResponsiveContainer, XAxis, YAxis } from "recharts";
import {
  Button,
  LocaleLabel,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
  cn,
  resolveLocaleName,
} from "@neokapi/ui";
import { CoverageBadge } from "./CoverageBadge";
import { SampleDataMark } from "./SampleDataNotice";
import {
  countryByCode,
  demandProvenanceLabel,
  formatSessions,
  formatShare,
  formatTokens,
  languageByCode,
  type DemandSnapshot,
  type LanguageDemand,
  type PlanEstimate,
} from "./locale-demand-fixtures";

export type DrillDownSelection =
  | { kind: "country"; code: string }
  | { kind: "language"; code: string };

export interface DemandDrillDownPanelProps {
  snapshot: DemandSnapshot;
  selection: DrillDownSelection;
  onClose?: () => void;
  className?: string;
}

const DEMAND_COLOR = "var(--chart-1, oklch(0.646 0.222 41.116))";

function TrendAreaChart({ trend }: { trend: number[] }) {
  if (trend.length === 0) {
    return (
      <div
        className="flex h-36 w-full items-center justify-center rounded-lg border border-dashed text-xs text-muted-foreground/60"
        data-testid="trend-chart-no-data"
      >
        No trend data from this source
      </div>
    );
  }
  const data = trend.map((sessions, i) => ({ week: `W${i + 1}`, sessions }));
  return (
    <div className="h-36 w-full">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: 4 }}>
          <defs>
            <linearGradient id="demand-trend-fill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={DEMAND_COLOR} stopOpacity={0.35} />
              <stop offset="100%" stopColor={DEMAND_COLOR} stopOpacity={0.04} />
            </linearGradient>
          </defs>
          <XAxis
            dataKey="week"
            tickLine={false}
            axisLine={false}
            interval={3}
            tick={{ fontSize: 10, fill: "var(--muted-foreground)" }}
          />
          <YAxis hide domain={["dataMin", "dataMax"]} />
          <Area
            type="monotone"
            dataKey="sessions"
            stroke={DEMAND_COLOR}
            strokeWidth={1.5}
            fill="url(#demand-trend-fill)"
            isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

function StatPair({ share, sessions }: { share: number; sessions: number }) {
  return (
    <div className="grid grid-cols-2 gap-3">
      <div className="rounded-lg border bg-card px-3 py-2">
        <div className="text-xs text-muted-foreground">Demand share</div>
        <div className="text-lg font-semibold tabular-nums">{formatShare(share)}</div>
      </div>
      <div className="rounded-lg border bg-card px-3 py-2">
        <div className="text-xs text-muted-foreground">Sessions</div>
        <div className="text-lg font-semibold tabular-nums">{formatSessions(sessions)}</div>
      </div>
    </div>
  );
}

function BreakdownBar({ share }: { share: number }) {
  return (
    <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
      <div
        className="h-full rounded-full"
        style={{ width: `${share * 100}%`, background: DEMAND_COLOR }}
      />
    </div>
  );
}

function ServedOutcome({ servedRate }: { servedRate: number | null }) {
  return (
    <section>
      <h3 className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Served-locale outcome
      </h3>
      <div className="rounded-lg border bg-card px-3 py-2">
        <div className="flex items-baseline justify-between">
          <span className="text-sm">Got their language today</span>
          {servedRate === null ? (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span
                    className="text-lg font-semibold text-muted-foreground/60"
                    data-testid="served-rate-unavailable"
                  >
                    ·
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  Not derivable from this source. The served locale can't be read from your URL
                  paths yet.
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ) : (
            <span className="text-lg font-semibold tabular-nums">{formatShare(servedRate)}</span>
          )}
        </div>
        {servedRate !== null && (
          <div className="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full"
              style={{ width: `${servedRate * 100}%`, background: DEMAND_COLOR }}
            />
          </div>
        )}
      </div>
    </section>
  );
}

function AddLocaleBlock({ code, estimate }: { code: string; estimate?: PlanEstimate }) {
  return (
    <section
      className="rounded-lg border border-primary/30 bg-primary/5 p-3"
      data-testid="add-locale-block"
    >
      <div className="text-sm font-medium">
        Close the gap: <LocaleLabel locale={code} />
      </div>
      {estimate && (
        <dl className="mt-2 grid grid-cols-3 gap-2 text-xs">
          <div>
            <dt className="text-muted-foreground">Units</dt>
            <dd className="font-medium tabular-nums">~{estimate.units.toLocaleString("en-US")}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Content memory</dt>
            <dd className="font-medium tabular-nums">{estimate.memoryCoveragePercent}%</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Volume</dt>
            <dd className="font-medium tabular-nums">{formatTokens(estimate.tokens)}</dd>
          </div>
        </dl>
      )}
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="sm" className="mt-3 w-full">
              Add {resolveLocaleName(code)} to the project
            </Button>
          </TooltipTrigger>
          <TooltipContent>Prototype, not wired up yet</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </section>
  );
}

/** Pick the biggest not-fully-covered language for a country's payoff block. */
function topGapLanguage(
  snapshot: DemandSnapshot,
  splits: { code: string; share: number }[],
): LanguageDemand | undefined {
  for (const split of splits) {
    const lang = languageByCode(snapshot, split.code);
    if (lang && lang.coverage.status !== "covered") return lang;
  }
  return undefined;
}

/**
 * Drill-down for a country (from the map) or a language (from the table):
 * demand trend, breakdown, served-locale outcome and the add-locale payoff.
 * Rendered inline as a side panel to match the app's panel conventions.
 */
export function DemandDrillDownPanel({
  snapshot,
  selection,
  onClose,
  className,
}: DemandDrillDownPanelProps) {
  const country =
    selection.kind === "country" ? countryByCode(snapshot, selection.code) : undefined;
  const language =
    selection.kind === "language" ? languageByCode(snapshot, selection.code) : undefined;

  const gapLanguage = useMemo(() => {
    if (selection.kind === "language") {
      return language && language.coverage.status !== "covered" ? language : undefined;
    }
    return country ? topGapLanguage(snapshot, country.languages) : undefined;
  }, [selection.kind, language, country, snapshot]);

  if (!country && !language) return null;

  return (
    <aside
      className={cn("flex w-80 shrink-0 flex-col rounded-xl border bg-card shadow-sm", className)}
      data-testid="demand-drilldown"
    >
      <header className="flex items-start justify-between gap-2 border-b p-4">
        <div>
          {country ? (
            <h2 className="text-base font-semibold" data-testid="drilldown-title">
              {country.flag} {country.name}
            </h2>
          ) : (
            <h2 className="text-base font-semibold" data-testid="drilldown-title">
              <LocaleLabel locale={language!.code} />
            </h2>
          )}
          <p className="text-xs text-muted-foreground">
            {country ? "Market drill-down" : "Language drill-down"}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {snapshot.provenance.kind === "sample" && <SampleDataMark />}
          {onClose && (
            <Button variant="ghost" size="icon" onClick={onClose} aria-label="Close drill-down">
              <X className="size-4" />
            </Button>
          )}
        </div>
      </header>

      <div className="flex-1 space-y-4 overflow-y-auto p-4">
        <StatPair share={(country ?? language!).share} sessions={(country ?? language!).sessions} />

        <section>
          <h3 className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Weekly sessions (12w)
          </h3>
          <TrendAreaChart trend={(country ?? language!).trend} />
        </section>

        {country && (
          <section>
            <h3 className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Languages in {country.name}
            </h3>
            <ul className="space-y-1.5">
              {country.languages.map((split) => {
                const lang = languageByCode(snapshot, split.code);
                return (
                  <li key={split.code} className="flex items-center justify-between gap-2 text-sm">
                    <span className="min-w-0 truncate">{resolveLocaleName(split.code)}</span>
                    <span className="flex items-center gap-2">
                      {lang && <CoverageBadge coverage={lang.coverage} />}
                      <BreakdownBar share={split.share} />
                      <span className="w-10 text-right tabular-nums text-muted-foreground">
                        {formatShare(split.share)}
                      </span>
                    </span>
                  </li>
                );
              })}
            </ul>
          </section>
        )}

        {language && (
          <section>
            <h3 className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Where the demand comes from
            </h3>
            <ul className="space-y-1.5">
              {language.countries.slice(0, 6).map((split) => {
                const c = countryByCode(snapshot, split.code);
                return (
                  <li key={split.code} className="flex items-center justify-between gap-2 text-sm">
                    <span className="min-w-0 truncate">
                      {c ? `${c.flag} ${c.name}` : split.code}
                    </span>
                    <span className="flex items-center gap-2">
                      <BreakdownBar share={split.share} />
                      <span className="w-10 text-right tabular-nums text-muted-foreground">
                        {formatShare(split.share)}
                      </span>
                    </span>
                  </li>
                );
              })}
            </ul>
          </section>
        )}

        <ServedOutcome servedRate={(country ?? language!).servedRate} />

        {gapLanguage && (
          <AddLocaleBlock code={gapLanguage.code} estimate={gapLanguage.planEstimate} />
        )}
      </div>

      <footer
        className="border-t px-4 py-2.5 text-[11px] text-muted-foreground"
        data-testid="demand-provenance"
      >
        {demandProvenanceLabel(snapshot.provenance)}
      </footer>
    </aside>
  );
}
