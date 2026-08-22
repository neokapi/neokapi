import { Card, CardContent, CardHeader, CardTitle, cn } from "@neokapi/ui-primitives";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import type { VoiceComplianceScore, ScoreTrend, StoredScore } from "./types";
import { VoiceScoreGauge } from "./VoiceScoreGauge";
import { VoiceDimensionBreakdown } from "./VoiceDimensionBreakdown";
import { VoiceFindingsList } from "./VoiceFindingsList";
import { DEFAULT_MIN_SCORE, scoreFillClass, scoreTextClass } from "./complianceBar";

interface VoiceDashboardProps {
  score: VoiceComplianceScore | null;
  trends: ScoreTrend[];
  recentScores: StoredScore[];
  /** The profile's compliance bar; DEFAULT_MIN_SCORE when no profile is loaded. */
  bar?: number;
  className?: string;
}

export function VoiceDashboard({
  score,
  trends,
  recentScores,
  bar = DEFAULT_MIN_SCORE,
  className,
}: VoiceDashboardProps) {
  if (!score) {
    return (
      <div className={cn("space-y-6", className)}>
        <h1 className="text-lg font-semibold mb-6">Voice Compliance Dashboard</h1>
        <Card className="p-8 text-center">
          <p className="text-sm text-muted-foreground">
            No compliance data yet. Run a voice check on your project content to see results here.
          </p>
        </Card>
      </div>
    );
  }

  return (
    <div className={cn("space-y-6", className)}>
      <h1 className="text-lg font-semibold">Voice Compliance Dashboard</h1>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Overall Score */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Overall Score</CardTitle>
          </CardHeader>
          <CardContent className="flex justify-center pb-4">
            <div className="relative">
              <VoiceScoreGauge score={score.overall} bar={bar} size={140} />
            </div>
          </CardContent>
        </Card>

        {/* Dimension Breakdown */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Dimensions</CardTitle>
          </CardHeader>
          <CardContent>
            <VoiceDimensionBreakdown dimensions={score.dimensions} bar={bar} />
          </CardContent>
        </Card>

        {/* Trend */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Score Trend</CardTitle>
          </CardHeader>
          <CardContent>
            {trends.length === 0 ? (
              <p className="text-xs text-muted-foreground text-center py-6">
                Not enough data for trends yet.
              </p>
            ) : (
              <div className="space-y-1">
                {trends.slice(-7).map((t) => (
                  <div key={t.date} className="flex items-center gap-2 text-xs">
                    <span className="text-muted-foreground w-20 shrink-0">{t.date}</span>
                    <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden">
                      <div
                        className={cn(
                          "h-full rounded-full transition-all",
                          scoreFillClass(t.avg_score, bar),
                        )}
                        style={{ width: `${t.avg_score}%` }}
                      />
                    </div>
                    <span className="tabular-nums w-8 text-right">{Math.round(t.avg_score)}</span>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Issue Density */}
      {recentScores.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Recent Checks</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2 max-h-60 overflow-auto">
              {recentScores.slice(0, 20).map((s) => (
                <div
                  key={s.id}
                  className="flex items-center justify-between text-xs border rounded px-3 py-2"
                >
                  <span className="text-muted-foreground truncate max-w-[200px]">{s.block_id}</span>
                  <span className="text-muted-foreground">{s.locale}</span>
                  <span
                    data-testid="recent-score"
                    data-below-bar={s.score < bar}
                    className={cn("font-medium tabular-nums", scoreTextClass(s.score, bar))}
                  >
                    {s.score}
                  </span>
                  <span className="text-muted-foreground">
                    <Plural count={s.findings.length}>
                      <One>{s.findings.length} finding</One>
                      <Other>{s.findings.length} findings</Other>
                    </Plural>
                  </span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Findings */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Findings ({score.findings.length})</CardTitle>
        </CardHeader>
        <CardContent>
          <VoiceFindingsList findings={score.findings} />
        </CardContent>
      </Card>
    </div>
  );
}
