// The all-surfaces voice compliance rollup (AD-021): one matrix answering "how
// compliant is every project right now?" — a row per project with its effective
// voice profile, latest score, per-dimension low point, trend, drift flag, and
// last activity. It reads the workspace rollup endpoint (a pure aggregation of
// stored scores) and complements the per-project ComplianceOverview drill-down.
import { Badge, Card, CardContent, Skeleton, cn } from "@neokapi/ui-primitives";
import { Activity, AlertTriangle, ArrowDown, ArrowRight, ArrowUp } from "../../components/icons";
import type { VoiceRollupEntry, VoiceTrend } from "../../voice/types";
import { useVoiceProfiles, useVoiceRollup } from "../../hooks/useVoiceApi";
import { DEFAULT_MIN_SCORE, barForProfile, scoreTextClass } from "../../voice/complianceBar";
import { EmptyState, formatRelative } from "../shell/atoms";

export interface RollupMatrixProps {
  /** Opens a project's own brand view when a row is clicked. Rows are static when omitted. */
  onOpenProject?: (projectId: string) => void;
}

export function RollupMatrix({ onOpenProject }: RollupMatrixProps) {
  const { data, isLoading } = useVoiceRollup();
  const { data: profiles } = useVoiceProfiles();
  const rows = data?.projects ?? [];

  return (
    <Card>
      <CardContent className="space-y-4 p-4">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-medium">
            <Activity className="size-4 text-muted-foreground" />
            Compliance across projects
          </h3>
          <p className="text-xs text-muted-foreground">
            Every project's brand health at a glance: effective profile, latest score, and drift.
          </p>
        </div>

        {isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : rows.length === 0 ? (
          <EmptyState
            icon={<Activity />}
            title="No projects yet"
            description="Voice checks score project content. Create a project and run a check to see it here."
            className="py-10"
          />
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[40rem] border-collapse text-sm">
                <thead>
                  <tr className="border-b text-left text-xs text-muted-foreground">
                    <th scope="col" className="py-2 pr-3 font-medium">
                      Project
                    </th>
                    <th scope="col" className="px-3 py-2 font-medium">
                      Profile
                    </th>
                    <th scope="col" className="px-3 py-2 text-right font-medium">
                      Score
                    </th>
                    <th scope="col" className="px-3 py-2 text-center font-medium">
                      Trend
                    </th>
                    <th scope="col" className="px-3 py-2 font-medium">
                      Drift
                    </th>
                    <th scope="col" className="py-2 pl-3 text-right font-medium">
                      Last activity
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <RollupRow
                      key={row.project_id}
                      row={row}
                      bar={barForProfile(profiles, row.profile_id)}
                      onOpenProject={onOpenProject}
                    />
                  ))}
                </tbody>
              </table>
            </div>
            {data && data.total > rows.length && (
              <p className="text-xs text-muted-foreground">
                Showing {rows.length} of {data.total} projects.
              </p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

function RollupRow({
  row,
  bar = DEFAULT_MIN_SCORE,
  onOpenProject,
}: {
  row: VoiceRollupEntry;
  /** The row's own profile bar — each project is judged against its own. */
  bar?: number;
  onOpenProject?: (projectId: string) => void;
}) {
  return (
    <tr className="border-b last:border-0">
      <td className="py-2.5 pr-3">
        {onOpenProject ? (
          <button
            type="button"
            onClick={() => onOpenProject(row.project_id)}
            className="truncate text-left font-medium text-foreground hover:text-primary hover:underline"
          >
            {row.project_name}
          </button>
        ) : (
          <span className="truncate font-medium text-foreground">{row.project_name}</span>
        )}
      </td>
      <td className="px-3 py-2.5">
        {row.profile_name ? (
          <span className="text-muted-foreground">{row.profile_name}</span>
        ) : (
          <span className="text-muted-foreground/60">Unbound</span>
        )}
      </td>
      <td className="px-3 py-2.5 text-right tabular-nums">
        {row.overall === null ? (
          <span className="text-muted-foreground/60">·</span>
        ) : (
          <span
            data-testid="rollup-score"
            data-below-bar={row.overall < bar}
            className={cn("font-semibold", scoreTextClass(row.overall, bar))}
          >
            {row.overall}
          </span>
        )}
      </td>
      <td className="px-3 py-2.5 text-center">
        <TrendCell trend={row.trend} />
      </td>
      <td className="px-3 py-2.5">
        {row.drift?.drifted ? (
          <Badge className="gap-1 border-warning/40 bg-warning/10 font-medium text-warning">
            <AlertTriangle className="size-3" />
            Drifting
          </Badge>
        ) : row.overall === null ? (
          <span className="text-muted-foreground/60">·</span>
        ) : (
          <span className="text-xs text-muted-foreground">Stable</span>
        )}
      </td>
      <td className="py-2.5 pl-3 text-right text-muted-foreground">
        {row.last_scored_at ? formatRelative(row.last_scored_at) : "Never"}
      </td>
    </tr>
  );
}

function TrendCell({ trend }: { trend: VoiceTrend }) {
  switch (trend) {
    case "up":
      return (
        <span className="inline-flex text-success" title="Improving">
          <ArrowUp className="size-4" />
          <span className="sr-only">Improving</span>
        </span>
      );
    case "down":
      return (
        <span className="inline-flex text-destructive" title="Declining">
          <ArrowDown className="size-4" />
          <span className="sr-only">Declining</span>
        </span>
      );
    case "flat":
      return (
        <span className="inline-flex text-muted-foreground" title="Steady">
          <ArrowRight className="size-4" />
          <span className="sr-only">Steady</span>
        </span>
      );
    default:
      return <span className="text-muted-foreground/60">·</span>;
  }
}
