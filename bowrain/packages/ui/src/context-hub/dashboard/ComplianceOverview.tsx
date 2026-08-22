// Compliance overview for the Voice dashboard (AD-021): per-project brand-check
// health. Brand scores are scoped to a project's content, so this offers a
// project picker and reads the existing brand score / trend / drift hooks. It
// reuses the brand score gauge, dimension breakdown, and drift banner.
import { useEffect, useState } from "react";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import {
  Card,
  CardContent,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Skeleton,
} from "@neokapi/ui-primitives";
import { BarChart3 } from "../../components/icons";
import { useProjects } from "../../hooks/useProjectApi";
import {
  useVoiceScores,
  useVoiceTrends,
  useVoiceDrift,
  useVoiceProfiles,
} from "../../hooks/useVoiceApi";
import { barForProfile } from "../../voice/complianceBar";
import { VoiceScoreGauge } from "../../voice/VoiceScoreGauge";
import { VoiceDimensionBreakdown } from "../../voice/VoiceDimensionBreakdown";
import { DriftAlert } from "../../voice/DriftAlert";
import { EmptyState } from "../shell/atoms";
import { averageScore, aggregateDimensions } from "./metrics";
import { ScoreTrendChart } from "./ScoreTrendChart";

export function ComplianceOverview() {
  const { data: projects, isLoading: projectsLoading } = useProjects();
  const [projectId, setProjectId] = useState("");

  useEffect(() => {
    if (!projectId && projects && projects.length > 0) setProjectId(projects[0].id);
  }, [projects, projectId]);

  const { data: scores, isLoading: scoresLoading } = useVoiceScores(projectId);
  const { data: trends } = useVoiceTrends(projectId);
  const { data: drift } = useVoiceDrift(projectId);
  const { data: profiles } = useVoiceProfiles();

  const recentScores = scores ?? [];
  const trendSeries = trends ?? [];
  const avg = averageScore(recentScores);
  const dimensions = aggregateDimensions(recentScores);
  const hasData = recentScores.length > 0 || trendSeries.length > 0;
  // Scores name the profile that produced them, so the bar comes from that
  // profile — the same pairing the server makes when aggregating compliance.
  const bar = barForProfile(profiles, recentScores[0]?.profile_id);

  return (
    <Card>
      <CardContent className="space-y-4 p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="flex items-center gap-2 text-sm font-medium">
              <BarChart3 className="size-4 text-muted-foreground" />
              Compliance
            </h3>
            <p className="text-xs text-muted-foreground">
              Brand-check health for a project's content.
            </p>
          </div>
          {projects && projects.length > 0 && (
            <Select value={projectId} onValueChange={setProjectId}>
              <SelectTrigger size="sm" className="w-48 text-xs">
                <SelectValue placeholder="Choose a project" />
              </SelectTrigger>
              <SelectContent>
                {projects.map((p) => (
                  <SelectItem key={p.id} value={p.id} className="text-xs">
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </div>

        {projectsLoading ? (
          <Skeleton className="h-44 w-full" />
        ) : !projects || projects.length === 0 ? (
          <EmptyState
            icon={<BarChart3 />}
            title="No projects yet"
            description="Voice checks score project content. Create a project and run a check to see compliance here."
            className="py-10"
          />
        ) : scoresLoading && !hasData ? (
          <Skeleton className="h-44 w-full" />
        ) : !hasData ? (
          <EmptyState
            title="No brand checks recorded"
            description="Run a voice check on this project's content to populate its compliance trend."
            className="py-10"
          />
        ) : (
          <div className="grid gap-6 lg:grid-cols-12">
            <div className="flex flex-col items-center justify-center gap-3 lg:col-span-3">
              {avg === null ? (
                <p className="text-sm text-muted-foreground">No score yet</p>
              ) : (
                <div className="relative">
                  <VoiceScoreGauge score={avg} bar={bar} size={132} />
                </div>
              )}
              <p className="text-center text-xs text-muted-foreground">
                <Plural count={recentScores.length}>
                  <One>{recentScores.length} block checked</One>
                  <Other>{recentScores.length} blocks checked</Other>
                </Plural>
              </p>
            </div>

            <div className="lg:col-span-4">
              {dimensions.length > 0 ? (
                <VoiceDimensionBreakdown dimensions={dimensions} bar={bar} />
              ) : (
                <p className="text-xs text-muted-foreground">No dimension data yet.</p>
              )}
            </div>

            <div className="space-y-3 lg:col-span-5">
              {trendSeries.length > 0 ? (
                <ScoreTrendChart trends={trendSeries} />
              ) : (
                <p className="text-xs text-muted-foreground">Not enough history for a trend yet.</p>
              )}
              {drift && <DriftAlert drift={drift} showStable />}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
