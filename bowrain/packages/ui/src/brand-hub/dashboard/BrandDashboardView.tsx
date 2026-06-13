// Dashboard — the brand control room (AD-021). It answers the questions a brand
// steward actually asks: what needs my decision, is compliance holding, how much
// brand language exists and how completely does it span the workspace, and what
// moved recently. The governance inbox leads because it is what a steward acts
// on; coverage, compliance, and recent change fan out below it. Built entirely
// on the real concept, change-set, market, and brand-profile hooks.
import { useMemo } from "react";
import { Button, Card, CardContent, Skeleton, cn } from "@neokapi/ui-primitives";
import { Network, FlaskConical, Palette, Shield, ArrowRight, Pencil } from "../../components/icons";
import type { ChangeSet } from "../../types/brand-graph";
import type { ConceptInfo } from "../../types/api";
import { useChangesets } from "../../hooks/useChangesetsApi";
import { useConcepts } from "../../hooks/useConceptsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { useBrandProfiles } from "../../hooks/useBrandApi";
import { BrandHub } from "../shell/BrandHub";
import { ChangeSetStatusBadge, EmptyState, formatRelative } from "../shell/atoms";
import { activeExperiments, pendingDecisions, sortByRecent } from "./metrics";
import { PendingDecisions } from "./PendingDecisions";
import { ComplianceOverview } from "./ComplianceOverview";
import { VocabularyByStatus, LocaleCoveragePanel, MarketsPanel } from "./CoveragePanel";

export interface BrandDashboardViewProps {
  onOpenExperiment?: (changesetId: string) => void;
  onViewExperiments?: () => void;
  onViewConcepts?: () => void;
  onViewVoice?: () => void;
  onOpenConcept?: (conceptId: string) => void;
}

export function BrandDashboardView({
  onOpenExperiment,
  onViewExperiments,
  onViewConcepts,
  onViewVoice,
  onOpenConcept,
}: BrandDashboardViewProps) {
  const { data: allConcepts, isLoading: conceptsLoading } = useConcepts({ limit: 1 });
  const { data: changesets, isLoading: csLoading } = useChangesets();
  const { data: profiles } = useBrandProfiles();

  const changesetList = useMemo(() => changesets ?? [], [changesets]);
  const pending = useMemo(() => pendingDecisions(changesetList), [changesetList]);
  const active = useMemo(() => activeExperiments(changesetList), [changesetList]);
  const recentExperiments = useMemo(() => sortByRecent(changesetList).slice(0, 5), [changesetList]);

  return (
    <BrandHub
      title="Dashboard"
      description="The state of your brand language at a glance — pending decisions, compliance, coverage, and recent change."
      width="wide"
    >
      <div className="space-y-6">
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <MetricCard
            icon={<Network />}
            label="Concepts"
            value={conceptsLoading ? undefined : (allConcepts?.total_count ?? 0)}
            onClick={onViewConcepts}
          />
          <MetricCard
            icon={<Shield />}
            label="Pending decisions"
            value={csLoading ? undefined : pending.length}
            emphasis={pending.length > 0}
            onClick={onViewExperiments}
          />
          <MetricCard
            icon={<FlaskConical />}
            label="Active experiments"
            value={csLoading ? undefined : active.length}
            onClick={onViewExperiments}
          />
          <MetricCard
            icon={<Palette />}
            label="Voice profiles"
            value={profiles?.length}
            onClick={onViewVoice}
          />
        </div>

        <PendingDecisions
          changesets={changesetList}
          loading={csLoading}
          onOpen={onOpenExperiment}
        />

        <ComplianceOverview />

        <div className="grid gap-3 lg:grid-cols-3">
          <VocabularyByStatus />
          <LocaleCoveragePanel />
          <MarketsPanel />
        </div>

        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardContent className="p-4">
              <SectionHeader title="Recent experiments" onMore={onViewExperiments} />
              {csLoading ? (
                <Skeleton className="h-24 w-full" />
              ) : recentExperiments.length === 0 ? (
                <EmptyState
                  title="No experiments yet"
                  description="Open a change-set to propose a governed edit to your brand language."
                  className="py-8"
                />
              ) : (
                <ul className="divide-y">
                  {recentExperiments.map((cs) => (
                    <ChangeSetRow
                      key={cs.id}
                      changeset={cs}
                      onOpen={onOpenExperiment ? () => onOpenExperiment(cs.id) : undefined}
                    />
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>

          <RecentConcepts onViewConcepts={onViewConcepts} onOpenConcept={onOpenConcept} />
        </div>
      </div>
    </BrandHub>
  );
}

// ── Recent concepts ───────────────────────────────────────────────────────────

function RecentConcepts({
  onViewConcepts,
  onOpenConcept,
}: {
  onViewConcepts?: () => void;
  onOpenConcept?: (conceptId: string) => void;
}) {
  const { data, isLoading } = useConcepts({ limit: 6 });
  const concepts = useMemo(
    () =>
      [...(data?.concepts ?? [])].sort(
        (a, b) =>
          new Date(b.updated_at || b.created_at).getTime() -
          new Date(a.updated_at || a.created_at).getTime(),
      ),
    [data],
  );

  return (
    <Card>
      <CardContent className="p-4">
        <SectionHeader title="Recently changed concepts" onMore={onViewConcepts} />
        {isLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : concepts.length === 0 ? (
          <EmptyState
            title="No concepts yet"
            description="Capture your first brand concept to start the graph."
            className="py-8"
          />
        ) : (
          <ul className="divide-y">
            {concepts.map((concept) => (
              <ConceptRow
                key={concept.id}
                concept={concept}
                onOpen={onOpenConcept ? () => onOpenConcept(concept.id) : undefined}
              />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

// ── Small presentational helpers ──────────────────────────────────────────────

function MetricCard({
  icon,
  label,
  value,
  emphasis,
  onClick,
}: {
  icon: React.ReactNode;
  label: string;
  value?: number;
  emphasis?: boolean;
  onClick?: () => void;
}) {
  const Tag = onClick ? "button" : "div";
  return (
    <Tag
      type={onClick ? "button" : undefined}
      onClick={onClick}
      className={cn(
        "flex items-center gap-3 rounded-lg border bg-card p-4 text-left",
        onClick && "transition-colors hover:border-primary/40 hover:bg-muted/30",
      )}
    >
      <span
        className={cn(
          "flex size-9 shrink-0 items-center justify-center rounded-md [&_svg]:size-5",
          emphasis ? "bg-primary/10 text-primary" : "bg-muted text-muted-foreground",
        )}
      >
        {icon}
      </span>
      <div className="min-w-0">
        {value === undefined ? (
          <Skeleton className="h-6 w-10" />
        ) : (
          <div
            className={cn(
              "text-2xl font-semibold leading-tight tabular-nums",
              emphasis && "text-primary",
            )}
          >
            {value.toLocaleString()}
          </div>
        )}
        <div className="text-xs text-muted-foreground">{label}</div>
      </div>
    </Tag>
  );
}

function SectionHeader({ title, onMore }: { title: string; onMore?: () => void }) {
  return (
    <div className="mb-3 flex items-center justify-between gap-2">
      <h3 className="text-sm font-medium">{title}</h3>
      {onMore && (
        <Button
          size="sm"
          variant="ghost"
          className="h-7 text-xs text-muted-foreground"
          onClick={onMore}
        >
          View all
          <ArrowRight />
        </Button>
      )}
    </div>
  );
}

function ChangeSetRow({ changeset, onOpen }: { changeset: ChangeSet; onOpen?: () => void }) {
  const { nameOf } = useUserDisplayNames();
  const Tag = onOpen ? "button" : "div";
  return (
    <li>
      <Tag
        type={onOpen ? "button" : undefined}
        onClick={onOpen}
        className={cn(
          "flex w-full items-center gap-3 py-2.5 text-left",
          onOpen && "transition-colors hover:bg-muted/30",
        )}
      >
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium text-foreground">{changeset.name}</div>
          <div className="text-xs text-muted-foreground">
            {nameOf(changeset.created_by)} · {formatRelative(changeset.updated_at)}
          </div>
        </div>
        <ChangeSetStatusBadge status={changeset.status} />
      </Tag>
    </li>
  );
}

function ConceptRow({ concept, onOpen }: { concept: ConceptInfo; onOpen?: () => void }) {
  const Tag = onOpen ? "button" : "div";
  return (
    <li>
      <Tag
        type={onOpen ? "button" : undefined}
        onClick={onOpen}
        className={cn(
          "flex w-full items-center gap-3 py-2.5 text-left",
          onOpen && "transition-colors hover:bg-muted/30",
        )}
      >
        <span className="flex size-8 shrink-0 items-center justify-center rounded-full border bg-muted/40 text-muted-foreground [&_svg]:size-4">
          <Pencil />
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium text-foreground">
            {conceptDisplayName(concept)}
          </div>
          <div className="text-xs text-muted-foreground">
            {concept.domain ? `${concept.domain} · ` : ""}
            {concept.terms.length} term{concept.terms.length === 1 ? "" : "s"} ·{" "}
            {formatRelative(concept.updated_at || concept.created_at)}
          </div>
        </div>
        <ArrowRight className="size-4 shrink-0 text-muted-foreground" />
      </Tag>
    </li>
  );
}

function conceptDisplayName(concept: ConceptInfo): string {
  if (concept.terms.length === 0) return concept.domain || concept.id;
  const preferred = concept.terms.find((t) => t.status === "preferred");
  const english = concept.terms.find((t) => t.locale.startsWith("en"));
  return (preferred ?? english ?? concept.terms[0]).text;
}
