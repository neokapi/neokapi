// Dashboard — the brand control room (AD-021). It answers the questions a brand
// steward actually asks: what needs my decision, is compliance holding, how much
// brand language exists and how completely does it span the workspace, and what
// moved recently. The governance inbox leads because it is what a steward acts
// on; coverage, compliance, and recent change fan out below it. Built entirely
// on the real concept, change-set, market, and voice profile hooks.
import { useMemo } from "react";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import { Button, Card, CardContent, Skeleton, cn } from "@neokapi/ui-primitives";
import { Network, FlaskConical, Palette, Shield, ArrowRight, Pencil } from "../../components/icons";
import type { ChangeSet } from "../../types/brand-graph";
import type { ConceptInfo } from "../../types/api";
import { useChangesetCounts, useChangesets } from "../../hooks/useChangesetsApi";
import { useConcepts, useConceptStatusCounts } from "../../hooks/useConceptsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { useVoiceProfiles } from "../../hooks/useVoiceApi";
import { TERMINAL_CHANGESET_STATUSES } from "../../types/brand-graph";
import { ContextHub } from "../shell/ContextHub";
import { ChangeSetStatusBadge, EmptyState, formatRelative } from "../shell/atoms";
import { sortByRecent } from "./metrics";
import { PendingDecisions } from "./PendingDecisions";
import { ComplianceOverview } from "./ComplianceOverview";
import { RollupMatrix } from "./RollupMatrix";
import { VocabularyByStatus, LocaleCoveragePanel, MarketsPanel } from "./CoveragePanel";

export interface VoiceDashboardViewProps {
  onOpenExperiment?: (changesetId: string) => void;
  onViewExperiments?: () => void;
  onViewConcepts?: () => void;
  onViewVoice?: () => void;
  onOpenConcept?: (conceptId: string) => void;
  /** Opens a project's own brand view from the compliance rollup. */
  onOpenProject?: (projectId: string) => void;
}

export function VoiceDashboardView({
  onOpenExperiment,
  onViewExperiments,
  onViewConcepts,
  onViewVoice,
  onOpenConcept,
  onOpenProject,
}: VoiceDashboardViewProps) {
  const { data: conceptCounts, isLoading: conceptsLoading } = useConceptStatusCounts();
  const { data: csCounts, isLoading: countsLoading } = useChangesetCounts();
  const { data: changesets, isLoading: csLoading } = useChangesets();
  const { data: profiles } = useVoiceProfiles();

  const changesetList = useMemo(() => changesets ?? [], [changesets]);
  const recentExperiments = useMemo(() => sortByRecent(changesetList).slice(0, 5), [changesetList]);

  // The counts span every change-set, not the page the list returned. Pending
  // is what a steward must decide (in review, or approved awaiting merge);
  // active is everything the lifecycle has not settled.
  const pendingCount = countFor(csCounts?.by_status, ["in_review", "approved"]);
  const activeCount = activeFromCounts(csCounts?.total, csCounts?.by_status);

  return (
    <ContextHub
      title="Dashboard"
      description="The state of your brand language at a glance: pending decisions, compliance, coverage, and recent change."
      width="wide"
    >
      <div className="space-y-6">
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <MetricCard
            icon={<Network />}
            label="Concepts"
            value={conceptsLoading ? undefined : (conceptCounts?.total ?? 0)}
            onClick={onViewConcepts}
          />
          <MetricCard
            icon={<Shield />}
            label="Pending decisions"
            value={countsLoading ? undefined : pendingCount}
            emphasis={pendingCount > 0}
            onClick={onViewExperiments}
          />
          <MetricCard
            icon={<FlaskConical />}
            label="Active experiments"
            value={countsLoading ? undefined : activeCount}
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

        <RollupMatrix onOpenProject={onOpenProject} />

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
    </ContextHub>
  );
}

// ── Change-set counts ─────────────────────────────────────────────────────────

/** The sum of the named buckets, treating an absent bucket as empty. */
function countFor(byStatus: Record<string, number> | undefined, statuses: string[]): number {
  if (!byStatus) return 0;
  return statuses.reduce((sum, status) => sum + (byStatus[status] ?? 0), 0);
}

/**
 * Change-sets the lifecycle has not settled, as total minus the terminal
 * buckets. Counting the non-terminal statuses instead would undercount a status
 * this build does not know — and `total` already includes those.
 */
function activeFromCounts(
  total: number | undefined,
  byStatus: Record<string, number> | undefined,
): number {
  if (total === undefined) return 0;
  return Math.max(0, total - countFor(byStatus, [...TERMINAL_CHANGESET_STATUSES]));
}

// ── Recent concepts ───────────────────────────────────────────────────────────

function RecentConcepts({
  onViewConcepts,
  onOpenConcept,
}: {
  onViewConcepts?: () => void;
  onOpenConcept?: (conceptId: string) => void;
}) {
  // Ordered by the server; a page of six sorted client-side would only be the
  // six most recent of whichever six the relevance ordering happened to return.
  const { data, isLoading } = useConcepts({ sort: "updated_at", limit: 6 });
  const concepts = data?.concepts ?? [];

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
            <Plural count={concept.terms.length}>
              <One>{concept.terms.length} term</One>
              <Other>{concept.terms.length} terms</Other>
            </Plural>{" "}
            · {formatRelative(concept.updated_at || concept.created_at)}
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
