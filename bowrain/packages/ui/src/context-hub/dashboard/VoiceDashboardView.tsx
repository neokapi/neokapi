// The workspace's context dashboard (AD-021), in the overview shape Kapi
// Desktop's home has: labelled counts, then What's new (the change-sets a
// steward decides and the rules the team's corrections suggest, each a row
// leading to where it is decided), then the projects as aligned rows. Below
// them sit what the platform knows and the desktop does not: one project's
// compliance over time, terminology by status and by language, the markets,
// and the concepts changed most recently. Nothing here is edited in place.
import { useCallback, useMemo } from "react";
import { useQueries, type UseQueryResult } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  SectionHeading,
  Skeleton,
  StatRow,
  When,
  cn,
  type StatItem,
} from "@neokapi/ui-primitives";
import { ArrowRight } from "../../components/icons";
import type { ConceptInfo } from "../../types/api";
import type { CandidateRule } from "../../voice/types";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { useChangesetCounts, useChangesets } from "../../hooks/useChangesetsApi";
import { useConcepts, useConceptStatusCounts } from "../../hooks/useConceptsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { useVoiceProfiles, useVoiceRollup } from "../../hooks/useVoiceApi";
import { TERMINAL_CHANGESET_STATUSES } from "../../types/brand-graph";
import { ContextHub } from "../shell/ContextHub";
import { ComplianceOverview } from "./ComplianceOverview";
import { RollupMatrix } from "./RollupMatrix";
import { VocabularyByStatus, LocaleCoveragePanel, MarketsPanel } from "./CoveragePanel";
import { WhatsNew } from "./WhatsNew";
import { suggestionTotals, whatsNewRows, type ProfileCandidates } from "./newsRows";

export interface VoiceDashboardViewProps {
  onOpenExperiment?: (changesetId: string) => void;
  onViewExperiments?: () => void;
  onViewConcepts?: () => void;
  onViewVoice?: () => void;
  onOpenConcept?: (conceptId: string) => void;
  /** Opens a project's own page from the projects list. */
  onOpenProject?: (projectId: string) => void;
  /** Opens a voice profile's suggested-rule review. */
  onOpenReview?: (profileId: string) => void;
  /** Opens the workspace's context activity. */
  onViewActivity?: () => void;
  /** Take the keyboard for What's new. */
  keyboard?: boolean;
}

export function VoiceDashboardView({
  onOpenExperiment,
  onViewExperiments,
  onViewConcepts,
  onViewVoice,
  onOpenConcept,
  onOpenProject,
  onOpenReview,
  onViewActivity,
  keyboard = true,
}: VoiceDashboardViewProps) {
  const { data: conceptCounts, isLoading: conceptsLoading } = useConceptStatusCounts();
  const { data: csCounts, isLoading: countsLoading } = useChangesetCounts();
  const { data: changesets, isLoading: csLoading } = useChangesets();
  const { data: profiles, isLoading: profilesLoading } = useVoiceProfiles();
  const { data: rollup, isLoading: rollupLoading } = useVoiceRollup();
  const { nameOf } = useUserDisplayNames();
  const candidates = useProfileCandidates(profiles);

  const rows = useMemo(
    () => whatsNewRows(changesets ?? [], candidates.profiles),
    [changesets, candidates.profiles],
  );
  const totals = useMemo(() => suggestionTotals(candidates.profiles), [candidates.profiles]);

  // The counts span every change-set, not the page the list returned. Pending
  // is what a steward must decide (in review, or approved awaiting merge);
  // active is everything the lifecycle has not settled.
  const pendingCount = countFor(csCounts?.by_status, ["in_review", "approved"]);
  const activeCount = activeFromCounts(csCounts?.total, csCounts?.by_status);
  const needsYou = pendingCount + totals.conflicts;

  const countsLoaded =
    !conceptsLoading && !countsLoading && !profilesLoading && !rollupLoading && !candidates.loading;
  const items: StatItem[] = [
    {
      id: "projects",
      value: rollup?.total ?? 0,
      label: t("{count, plural, one {project} other {projects}}", "a count on the dashboard", {
        count: rollup?.total ?? 0,
      }),
    },
    {
      id: "concepts",
      value: conceptCounts?.total ?? 0,
      label: t("{count, plural, one {concept} other {concepts}}", "a count on the dashboard", {
        count: conceptCounts?.total ?? 0,
      }),
      onClick: onViewConcepts,
    },
    {
      id: "suggestions",
      value: totals.suggestions,
      label: t(
        "{count, plural, one {suggestion} other {suggestions}}",
        "a count on the dashboard",
        { count: totals.suggestions },
      ),
      hint: t(
        "Rules your team's repeated corrections suggest. Each waits for a reviewer to promote or reject it.",
      ),
    },
    {
      id: "needs-you",
      value: needsYou,
      label: t("{count, plural, one {needs you} other {need you}}", "a count on the dashboard", {
        count: needsYou,
      }),
      tone: needsYou > 0 ? "warning" : "muted",
      hint: t(
        "Change-sets in review or approved and waiting to merge, and phrasings your team corrected to more than one form.",
      ),
    },
    {
      id: "experiments",
      value: activeCount,
      label: t(
        "{count, plural, one {active experiment} other {active experiments}}",
        "a count on the dashboard",
        { count: activeCount },
      ),
      tone: "muted",
      onClick: onViewExperiments,
    },
    {
      id: "voice-profiles",
      value: profiles?.length ?? 0,
      label: t(
        "{count, plural, one {voice profile} other {voice profiles}}",
        "a count on the dashboard",
        { count: profiles?.length ?? 0 },
      ),
      tone: "muted",
      onClick: onViewVoice,
    },
  ];

  return (
    <ContextHub
      title="Dashboard"
      description="What your workspace's context holds, and what waits for a decision."
      actions={
        onViewActivity && (
          <Button
            variant="link"
            size="sm"
            data-slot="dashboard-activity"
            className="h-auto px-0"
            onClick={onViewActivity}
          >
            Activity
          </Button>
        )
      }
    >
      <div data-slot="context-dashboard" className="space-y-8">
        {countsLoaded ? (
          <StatRow data-slot="dashboard-numbers" items={items} />
        ) : (
          <Skeleton className="h-11 w-full max-w-xl" />
        )}

        <WhatsNew
          rows={rows}
          loading={csLoading || candidates.loading}
          nameOf={nameOf}
          onOpenChangeset={onOpenExperiment}
          onOpenReview={onOpenReview}
          keyboard={keyboard}
        />

        <RollupMatrix onOpenProject={onOpenProject} />

        <ComplianceOverview />

        <VocabularyByStatus />

        <div className="grid gap-8 lg:grid-cols-2">
          <LocaleCoveragePanel />
          <MarketsPanel />
        </div>

        <RecentConcepts onViewConcepts={onViewConcepts} onOpenConcept={onOpenConcept} />
      </div>
    </ContextHub>
  );
}

// ── Suggested rules, per voice profile ────────────────────────────────────────

/**
 * Every voice profile's open candidates, read through the same query the
 * profile's review reads, so opening a review answers from the cache.
 */
function useProfileCandidates(profiles: { id: string; name: string }[] | undefined): {
  profiles: ProfileCandidates[];
  loading: boolean;
} {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const list = useMemo(() => profiles ?? [], [profiles]);
  // A stable combine lets the query cache hand back the same object while
  // nothing changed, so the rows built from it keep their identity.
  const combine = useCallback(
    (results: UseQueryResult<CandidateRule[]>[]) => ({
      profiles: list.map((profile, i) => ({
        id: profile.id,
        name: profile.name,
        candidates: results[i]?.data ?? [],
      })),
      loading: results.some((r) => r.isLoading),
    }),
    [list],
  );
  return useQueries({
    queries: list.map((profile) => ({
      // The key useVoiceCandidates gives a profile's open candidates.
      queryKey: ["voice-candidates", ws, profile.id, 3, false],
      queryFn: () => api.listVoiceCandidates(ws, profile.id),
      enabled: !!ws,
      staleTime: 15_000,
    })),
    combine,
  });
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
 * this build does not know, and `total` already includes those.
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
    <section data-slot="dashboard-concepts" aria-labelledby="dashboard-concepts-heading">
      <div className="flex items-center justify-between gap-2">
        <div id="dashboard-concepts-heading">
          <SectionHeading>Recently changed concepts</SectionHeading>
        </div>
        {onViewConcepts && (
          <Button
            size="sm"
            variant="ghost"
            className="h-7 text-[11px] text-muted-foreground"
            onClick={onViewConcepts}
          >
            View all
            <ArrowRight />
          </Button>
        )}
      </div>
      {isLoading ? (
        <Skeleton className="mt-2 h-24 w-full" />
      ) : concepts.length === 0 ? (
        <p className="mt-2 text-sm text-muted-foreground">
          No concepts yet. A concept holds what your workspace calls one idea, in each language.
        </p>
      ) : (
        <div className="mt-1.5 divide-y divide-border/50">
          {concepts.map((concept) => (
            <ConceptRow
              key={concept.id}
              concept={concept}
              onOpen={onOpenConcept ? () => onOpenConcept(concept.id) : undefined}
            />
          ))}
        </div>
      )}
    </section>
  );
}

function ConceptRow({ concept, onOpen }: { concept: ConceptInfo; onOpen?: () => void }) {
  const Tag = onOpen ? "button" : "div";
  return (
    <Tag
      type={onOpen ? "button" : undefined}
      data-slot="dashboard-concept"
      onClick={onOpen}
      className={cn(
        "grid h-10 w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 rounded-md px-2 text-left",
        onOpen && "transition-colors hover:bg-muted/50",
      )}
    >
      <span className="flex min-w-0 items-baseline gap-1.5">
        <span className="truncate text-sm" translate="no">
          {conceptDisplayName(concept)}
        </span>
        <span className="shrink-0 text-[11px] text-muted-foreground">
          {concept.domain ? `${concept.domain} · ` : ""}
          {t("{count, plural, one {# term} other {# terms}}", { count: concept.terms.length })}
        </span>
      </span>
      <When
        iso={concept.updated_at || concept.created_at}
        relative
        className="text-[11px] whitespace-nowrap text-muted-foreground"
      />
    </Tag>
  );
}

function conceptDisplayName(concept: ConceptInfo): string {
  if (concept.terms.length === 0) return concept.domain || concept.id;
  const preferred = concept.terms.find((term) => term.status === "preferred");
  const english = concept.terms.find((term) => term.locale.startsWith("en"));
  return (preferred ?? english ?? concept.terms[0]).text;
}
