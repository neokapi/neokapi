// The workspace's projects on the context dashboard, as aligned rows in the
// shape Kapi Desktop's home lists them: the project, the voice profile bound
// to it, its latest compliance score against that profile's own bar, the
// trend, whether it is drifting, when it was last checked, and Open. It reads
// the workspace rollup endpoint (a pure aggregation of stored scores) (AD-021).
import { t } from "@neokapi/i18n-react/runtime";
import {
  Badge,
  Button,
  EmptyState,
  SectionHeading,
  SimpleTooltip,
  Skeleton,
  When,
  cn,
} from "@neokapi/ui-primitives";
import { AlertTriangle, ArrowDown, ArrowRight, ArrowUp, Folder } from "../../components/icons";
import type { VoiceRollupEntry, VoiceTrend } from "../../voice/types";
import { useVoiceProfiles, useVoiceRollup } from "../../hooks/useVoiceApi";
import { DEFAULT_MIN_SCORE, barForProfile, scoreTextClass } from "../../voice/complianceBar";

export interface RollupMatrixProps {
  /** Opens a project's own page. Rows carry no button when omitted. */
  onOpenProject?: (projectId: string) => void;
}

/**
 * The columns every row shares, so the list reads as one table: icon, name,
 * profile, score, trend, drift, last checked, Open. A narrow pane keeps the
 * name and the button.
 */
const COLUMNS =
  "grid-cols-[16px_minmax(0,1fr)_auto] @2xl:grid-cols-[16px_minmax(0,1fr)_9rem_2.5rem_1.25rem_5.5rem_7rem_4.5rem]";

export function RollupMatrix({ onOpenProject }: RollupMatrixProps) {
  const { data, isLoading } = useVoiceRollup();
  const { data: profiles } = useVoiceProfiles();
  const rows = data?.projects ?? [];

  return (
    <section data-slot="dashboard-projects" aria-labelledby="dashboard-projects-heading">
      <div id="dashboard-projects-heading">
        <SectionHeading>Projects</SectionHeading>
      </div>
      {isLoading ? (
        <Skeleton className="mt-2 h-32 w-full" />
      ) : rows.length === 0 ? (
        <EmptyState
          className="mt-2"
          title={t("No projects yet")}
          description={t(
            "Voice checks score project content. Create a project and run a check to see it here.",
          )}
        />
      ) : (
        <div className="@container mt-1.5">
          <div className="divide-y divide-border/50">
            {rows.map((row) => (
              <ProjectRow
                key={row.project_id}
                row={row}
                bar={barForProfile(profiles, row.profile_id)}
                onOpenProject={onOpenProject}
              />
            ))}
          </div>
          {data && data.total > rows.length && (
            <p className="mt-2 px-2 text-[11px] text-muted-foreground">
              {t("Showing {shown} of {total} projects.", {
                shown: rows.length,
                total: data.total,
              })}
            </p>
          )}
        </div>
      )}
    </section>
  );
}

function ProjectRow({
  row,
  bar = DEFAULT_MIN_SCORE,
  onOpenProject,
}: {
  row: VoiceRollupEntry;
  /** The row's own profile bar: each project is judged against its own. */
  bar?: number;
  onOpenProject?: (projectId: string) => void;
}) {
  return (
    <div
      data-testid="dashboard-project"
      data-project={row.project_id}
      className={cn("grid h-11 items-center gap-x-3 px-2", COLUMNS)}
    >
      <Folder aria-hidden className="size-4 text-muted-foreground" />
      <span className="truncate text-sm" translate="no">
        {row.project_name}
      </span>
      <div className="hidden text-[11px] text-muted-foreground @2xl:contents">
        <span data-slot="dashboard-project-profile" className="truncate" translate="no">
          {row.profile_name ?? <span className="text-muted-foreground/60">no voice</span>}
        </span>
        <span className="text-right text-sm tabular-nums">
          {row.overall === null ? (
            <span className="text-[11px] text-muted-foreground/60">·</span>
          ) : (
            <SimpleTooltip
              content={t("Compliance {score} against a bar of {bar}", {
                score: row.overall,
                bar,
              })}
            >
              <span
                data-testid="rollup-score"
                data-below-bar={row.overall < bar}
                tabIndex={0}
                className={cn("font-semibold", scoreTextClass(row.overall, bar))}
              >
                {row.overall}
              </span>
            </SimpleTooltip>
          )}
        </span>
        <TrendCell trend={row.trend} />
        <span className="flex min-w-0 items-center">
          {row.drift?.drifted ? (
            <Badge
              variant="warning"
              data-slot="dashboard-project-drift"
              className="gap-1 text-[11px] font-normal"
            >
              <AlertTriangle aria-hidden className="size-3" />
              Drifting
            </Badge>
          ) : row.overall === null ? null : (
            <span>Stable</span>
          )}
        </span>
        <span className="truncate">
          {row.last_scored_at ? <When iso={row.last_scored_at} relative /> : <>never checked</>}
        </span>
      </div>
      <div className="flex items-center justify-end">
        {onOpenProject && (
          <Button size="sm" variant="outline" onClick={() => onOpenProject(row.project_id)}>
            Open
          </Button>
        )}
      </div>
    </div>
  );
}

function TrendCell({ trend }: { trend: VoiceTrend }) {
  const meta =
    trend === "up"
      ? { icon: ArrowUp, label: t("Improving"), tone: "text-success" }
      : trend === "down"
        ? { icon: ArrowDown, label: t("Declining"), tone: "text-destructive" }
        : trend === "flat"
          ? { icon: ArrowRight, label: t("Steady"), tone: "text-muted-foreground" }
          : null;
  if (!meta) return <span />;
  const Icon = meta.icon;
  return (
    <SimpleTooltip content={meta.label}>
      <span
        role="img"
        aria-label={meta.label}
        tabIndex={0}
        className={cn("inline-flex", meta.tone)}
      >
        <Icon aria-hidden className="size-3.5" />
      </span>
    </SimpleTooltip>
  );
}
