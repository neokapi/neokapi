// The blast-radius hero (AD-021): the one number that turns a what-if into a
// measured experiment — how many stored blocks a change-set's draft would newly
// flag or resolve — with a per-project / per-locale break-down and a sample of
// the affected content. Pure presentation over a ChangeSetImpact; reused by the
// experiment detail view, the merge confirmation, and the live wizard preview.
import { useMemo, useState } from "react";
import {
  Badge,
  Card,
  CardContent,
  ChartContainer,
  Skeleton,
  Tabs,
  TabsList,
  TabsTrigger,
  cn,
  type ChartConfig,
} from "@neokapi/ui-primitives";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis, Tooltip } from "recharts";
import { AlertTriangle, CircleCheck, FileText, Clock, Layers } from "../../components/icons";
import type { ChangeSetImpact, BlockSample } from "../../types/brand-graph";
import {
  byLocale,
  byProject,
  formatCompact,
  formatEffort,
  formatPercent,
  affectedShare,
  netViolationDelta,
  type ImpactBar,
} from "./blastRadius";

const chartConfig: ChartConfig = {
  new_violations: { label: "New flags", color: "var(--destructive)" },
  resolved: { label: "Resolved", color: "var(--success)" },
};

export interface BlastRadiusPanelProps {
  impact?: ChangeSetImpact;
  isLoading?: boolean;
  /** A short caption shown above the chart (e.g. "Live preview"). */
  caption?: string;
  /** Hide the sample-blocks list (e.g. in the compact merge confirmation). */
  hideSamples?: boolean;
  className?: string;
}

export function BlastRadiusPanel({
  impact,
  isLoading,
  caption,
  hideSamples,
  className,
}: BlastRadiusPanelProps) {
  if (isLoading) {
    return (
      <div className={cn("space-y-4", className)}>
        <Skeleton className="h-28 w-full" />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }
  if (!impact) {
    return (
      <p className={cn("text-sm text-muted-foreground", className)}>
        No impact computed yet. Add an operation to measure it.
      </p>
    );
  }

  const empty = impact.affected_blocks === 0 && impact.total_blocks > 0;

  return (
    <div className={cn("space-y-5", className)}>
      <Hero impact={impact} caption={caption} />
      <StatRow impact={impact} />
      {empty ? (
        <p className="rounded-lg border border-dashed bg-muted/20 px-4 py-6 text-center text-sm text-muted-foreground">
          This draft touches none of the {impact.total_blocks.toLocaleString()} stored blocks — no
          measurable impact on published content.
        </p>
      ) : (
        <BreakdownChart impact={impact} />
      )}
      {!hideSamples && (impact.samples?.length ?? 0) > 0 && <Samples samples={impact.samples} />}
    </div>
  );
}

// ── Hero number ──────────────────────────────────────────────────────────────

function Hero({ impact, caption }: { impact: ChangeSetImpact; caption?: string }) {
  const share = affectedShare(impact);
  return (
    <div className="relative overflow-hidden rounded-xl border bg-gradient-to-br from-primary/5 via-card to-card p-5">
      <div className="pointer-events-none absolute -right-6 -top-6 text-primary/10">
        <Layers className="size-28" />
      </div>
      <div className="relative">
        {caption && (
          <p className="mb-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
            {caption}
          </p>
        )}
        <div className="flex items-baseline gap-2">
          <span className="text-4xl font-semibold tabular-nums tracking-tight text-foreground">
            {impact.affected_blocks.toLocaleString()}
          </span>
          <span className="text-sm text-muted-foreground">
            affected block{impact.affected_blocks === 1 ? "" : "s"}
          </span>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          {formatPercent(share)} of {impact.total_blocks.toLocaleString()} stored blocks across{" "}
          {impact.projects.length} project{impact.projects.length === 1 ? "" : "s"}.
        </p>
      </div>
    </div>
  );
}

// ── Headline stats ───────────────────────────────────────────────────────────

function StatRow({ impact }: { impact: ChangeSetImpact }) {
  const net = netViolationDelta(impact);
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
      <Stat
        icon={<AlertTriangle />}
        label="New flags"
        value={impact.new_violations}
        tone={impact.new_violations > 0 ? "destructive" : "default"}
      />
      <Stat
        icon={<CircleCheck />}
        label="Resolved"
        value={impact.resolved}
        tone={impact.resolved > 0 ? "success" : "default"}
      />
      <Stat
        icon={<FileText />}
        label="Words"
        value={impact.words}
        hint={net !== 0 ? `${net > 0 ? "+" : ""}${net} net flags` : "no net change"}
      />
      <Stat icon={<Clock />} label="Review effort" valueText={formatEffort(impact.words)} />
    </div>
  );
}

function Stat({
  icon,
  label,
  value,
  valueText,
  hint,
  tone = "default",
}: {
  icon: React.ReactNode;
  label: string;
  value?: number;
  valueText?: string;
  hint?: string;
  tone?: "default" | "destructive" | "success";
}) {
  return (
    <div className="rounded-lg border bg-card p-3">
      <div className="flex items-center gap-1.5 text-muted-foreground [&_svg]:size-3.5">
        {icon}
        <span className="text-[11px] font-medium">{label}</span>
      </div>
      <div
        className={cn(
          "mt-1 text-xl font-semibold tabular-nums",
          tone === "destructive" && "text-destructive",
          tone === "success" && "text-success",
        )}
      >
        {valueText ?? (value ?? 0).toLocaleString()}
      </div>
      {hint && <div className="text-[11px] text-muted-foreground">{hint}</div>}
    </div>
  );
}

// ── Break-down chart ─────────────────────────────────────────────────────────

type Dimension = "project" | "locale";

function BreakdownChart({ impact }: { impact: ChangeSetImpact }) {
  const [dim, setDim] = useState<Dimension>("project");
  const locales = useMemo(() => byLocale(impact), [impact]);
  const projects = useMemo(() => byProject(impact), [impact]);
  const bars = dim === "project" ? projects : locales;
  const data = bars.slice(0, 8);

  return (
    <Card>
      <CardContent className="space-y-3 p-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-medium">Where it lands</h3>
          <Tabs value={dim} onValueChange={(v) => setDim(v as Dimension)}>
            <TabsList className="h-7">
              <TabsTrigger value="project" className="text-xs">
                By project
              </TabsTrigger>
              <TabsTrigger value="locale" className="text-xs" disabled={locales.length === 0}>
                By locale
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>

        {data.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            No {dim} break-down available.
          </p>
        ) : (
          <>
            <ChartContainer config={chartConfig} className="aspect-auto h-[220px] w-full">
              <BarChart data={data} margin={{ left: -8, right: 8, top: 4 }} barGap={2}>
                <CartesianGrid vertical={false} strokeDasharray="3 3" />
                <XAxis
                  dataKey="label"
                  tickLine={false}
                  axisLine={false}
                  tick={{ fontSize: 11 }}
                  interval={0}
                  tickFormatter={(v: string) => (v.length > 12 ? `${v.slice(0, 11)}…` : v)}
                />
                <YAxis
                  tickLine={false}
                  axisLine={false}
                  width={32}
                  tick={{ fontSize: 11 }}
                  tickFormatter={(v: number) => formatCompact(v)}
                />
                <Tooltip
                  cursor={{ fillOpacity: 0.08 }}
                  content={({ active, payload, label }) => {
                    if (!active || !payload?.length) return null;
                    const row = payload[0]?.payload as ImpactBar | undefined;
                    return (
                      <div className="rounded-lg border border-border/50 bg-background px-2.5 py-1.5 text-xs shadow-xl">
                        <div className="mb-1 font-medium">{label}</div>
                        {payload.map((item) => (
                          <div key={String(item.dataKey)} className="flex justify-between gap-4">
                            <span className="text-muted-foreground">
                              {item.dataKey === "new_violations" ? "New flags" : "Resolved"}
                            </span>
                            <span className="font-mono tabular-nums">
                              {Number(item.value ?? 0).toLocaleString()}
                            </span>
                          </div>
                        ))}
                        {row && (
                          <div className="mt-1 flex justify-between gap-4 border-t pt-1 text-muted-foreground">
                            <span>Affected</span>
                            <span className="font-mono tabular-nums">
                              {row.affected_blocks.toLocaleString()}
                            </span>
                          </div>
                        )}
                      </div>
                    );
                  }}
                />
                <Bar
                  dataKey="new_violations"
                  name="New flags"
                  fill="var(--color-new_violations)"
                  radius={[3, 3, 0, 0]}
                  maxBarSize={28}
                />
                <Bar
                  dataKey="resolved"
                  name="Resolved"
                  fill="var(--color-resolved)"
                  radius={[3, 3, 0, 0]}
                  maxBarSize={28}
                />
              </BarChart>
            </ChartContainer>
            <Legend />
          </>
        )}
      </CardContent>
    </Card>
  );
}

function Legend() {
  return (
    <div className="flex items-center justify-center gap-4 text-xs text-muted-foreground">
      <span className="flex items-center gap-1.5">
        <span className="size-2.5 rounded-[2px] bg-destructive" />
        New flags
      </span>
      <span className="flex items-center gap-1.5">
        <span className="size-2.5 rounded-[2px] bg-success" />
        Resolved
      </span>
    </div>
  );
}

// ── Sample blocks ────────────────────────────────────────────────────────────

function Samples({ samples }: { samples: BlockSample[] }) {
  const shown = samples.slice(0, 4);
  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium">Sample affected blocks</h3>
      <ul className="space-y-2">
        {shown.map((s) => (
          <li key={s.block_id} className="rounded-lg border bg-card p-3">
            <div className="mb-1 flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground">
              <span className="truncate font-medium text-foreground">{s.item_name}</span>
              <Badge variant="outline" className="font-mono text-[10px]">
                {s.locale}
              </Badge>
              {s.collection_name && <span>· {s.collection_name}</span>}
              {(s.new_violations ?? 0) > 0 && (
                <span className="text-destructive">+{s.new_violations} flags</span>
              )}
              {(s.resolved ?? 0) > 0 && (
                <span className="text-success">−{s.resolved} resolved</span>
              )}
            </div>
            <p className="line-clamp-2 text-sm text-foreground">{s.text}</p>
          </li>
        ))}
      </ul>
      {samples.length > shown.length && (
        <p className="text-xs text-muted-foreground">
          and {samples.length - shown.length} more sample
          {samples.length - shown.length === 1 ? "" : "s"}.
        </p>
      )}
    </div>
  );
}
