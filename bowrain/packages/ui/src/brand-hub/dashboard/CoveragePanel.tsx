// Coverage panels for the Brand dashboard (AD-021): how much brand language
// exists and how completely it spans the workspace. Three self-contained cards —
// vocabulary by lifecycle status, per-locale completeness, and the markets a
// workspace has defined.
import { Badge, Card, CardContent, Skeleton, cn } from "@neokapi/ui-primitives";
import { Tag, Languages, Globe } from "../../components/icons";
import type { TermStatus } from "../../types/brand-graph";
import { useConcepts } from "../../hooks/useConceptsApi";
import { useMarkets } from "../../hooks/useMarketsApi";
import { TermStatusBadge, EmptyState } from "../shell/atoms";
import { computeLocaleCoverage } from "./metrics";

// ── Vocabulary by status ──────────────────────────────────────────────────────

const STATUS_BAR: Record<TermStatus, string> = {
  preferred: "bg-success",
  approved: "bg-primary",
  admitted: "bg-warning",
  proposed: "bg-muted-foreground/40",
  deprecated: "bg-muted-foreground/30",
  forbidden: "bg-destructive",
};

const SHOWN_STATUSES: TermStatus[] = ["preferred", "approved", "admitted", "proposed", "forbidden"];

function PanelShell({
  icon,
  title,
  description,
  children,
}: {
  icon: React.ReactNode;
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <Card className="h-full">
      <CardContent className="flex h-full flex-col gap-3 p-4">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-medium">
            <span className="text-muted-foreground [&_svg]:size-4">{icon}</span>
            {title}
          </h3>
          <p className="text-xs text-muted-foreground">{description}</p>
        </div>
        <div className="flex-1">{children}</div>
      </CardContent>
    </Card>
  );
}

export function VocabularyByStatus() {
  // total_count comes back regardless of limit, so the count queries stay cheap.
  const total = useConcepts({ limit: 1 });
  const preferred = useConcepts({ status: "preferred", limit: 1 });
  const approved = useConcepts({ status: "approved", limit: 1 });
  const admitted = useConcepts({ status: "admitted", limit: 1 });
  const proposed = useConcepts({ status: "proposed", limit: 1 });
  const forbidden = useConcepts({ status: "forbidden", limit: 1 });

  const counts: Record<TermStatus, number> = {
    preferred: preferred.data?.total_count ?? 0,
    approved: approved.data?.total_count ?? 0,
    admitted: admitted.data?.total_count ?? 0,
    proposed: proposed.data?.total_count ?? 0,
    deprecated: 0,
    forbidden: forbidden.data?.total_count ?? 0,
  };
  const totalCount = total.data?.total_count ?? 0;
  const max = Math.max(totalCount, ...SHOWN_STATUSES.map((s) => counts[s]), 1);
  const loading = total.isLoading;

  return (
    <PanelShell
      icon={<Tag />}
      title="Vocabulary by status"
      description="Concepts carrying a term at each lifecycle stage."
    >
      {loading ? (
        <Skeleton className="h-32 w-full" />
      ) : totalCount === 0 ? (
        <EmptyState title="No concepts yet" className="py-6" />
      ) : (
        <ul className="space-y-2.5">
          {SHOWN_STATUSES.map((status) => (
            <li key={status} className="space-y-1">
              <div className="flex items-center justify-between">
                <TermStatusBadge status={status} className="text-[10px]" />
                <span className="text-sm tabular-nums text-muted-foreground">{counts[status]}</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  className={cn("h-full rounded-full", STATUS_BAR[status])}
                  style={{ width: `${Math.round((counts[status] / max) * 100)}%` }}
                />
              </div>
            </li>
          ))}
        </ul>
      )}
    </PanelShell>
  );
}

// ── Locale coverage ───────────────────────────────────────────────────────────

export function LocaleCoveragePanel({ limit = 8 }: { limit?: number }) {
  const { data, isLoading } = useConcepts({ limit: 100 });
  const concepts = data?.concepts ?? [];
  const coverage = computeLocaleCoverage(concepts).slice(0, limit);

  return (
    <PanelShell
      icon={<Languages />}
      title="Locale coverage"
      description="Share of concepts with a term in each locale."
    >
      {isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : coverage.length === 0 ? (
        <EmptyState title="No localized terms yet" className="py-6" />
      ) : (
        <ul className="space-y-2.5">
          {coverage.map((row) => (
            <li key={row.locale} className="space-y-1">
              <div className="flex items-center justify-between text-xs">
                <span className="font-medium text-foreground">{row.locale}</span>
                <span className="tabular-nums text-muted-foreground">
                  {row.present}/{row.total} · {row.pct}%
                </span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  className={cn(
                    "h-full rounded-full",
                    row.pct >= 80 ? "bg-success" : row.pct >= 40 ? "bg-primary" : "bg-warning",
                  )}
                  style={{ width: `${row.pct}%` }}
                />
              </div>
            </li>
          ))}
        </ul>
      )}
    </PanelShell>
  );
}

// ── Markets ───────────────────────────────────────────────────────────────────

export function MarketsPanel() {
  const { data: markets, isLoading } = useMarkets();
  const list = markets ?? [];

  return (
    <PanelShell
      icon={<Globe />}
      title="Markets"
      description="Workspace scopes that give validity tags a stable vocabulary."
    >
      {isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : list.length === 0 ? (
        <EmptyState
          title="No markets defined"
          description="Define a market — a name plus the locales it covers — to scope terms by region."
          className="py-6"
        />
      ) : (
        <ul className="space-y-2">
          {list.map((market) => (
            <li
              key={market.id}
              className="flex items-center justify-between gap-2 rounded-md border bg-card/50 px-2.5 py-2"
            >
              <div className="min-w-0">
                <div className="truncate text-sm font-medium capitalize">{market.name}</div>
                {market.description && (
                  <div className="truncate text-xs text-muted-foreground">{market.description}</div>
                )}
              </div>
              <Badge variant="outline" className="shrink-0 text-[10px]">
                {market.locales.length} locale{market.locales.length === 1 ? "" : "s"}
              </Badge>
            </li>
          ))}
        </ul>
      )}
    </PanelShell>
  );
}
