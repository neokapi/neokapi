import type { BrandComplianceScore, ScoreTrend, StoredScore } from "./types";
import { BrandScoreGauge } from "./BrandScoreGauge";
import { BrandDimensionBreakdown } from "./BrandDimensionBreakdown";
import { BrandFindingsList } from "./BrandFindingsList";
import { GlassCard, CardHeader, CardTitle, CardContent } from "../components/ui/card";
import { cn } from "../lib/utils";

interface BrandDashboardProps {
  score: BrandComplianceScore | null;
  trends: ScoreTrend[];
  recentScores: StoredScore[];
  className?: string;
}

export function BrandDashboard({ score, trends, recentScores, className }: BrandDashboardProps) {
  if (!score) {
    return (
      <div className={cn("space-y-6", className)}>
        <h1 className="text-lg font-semibold mb-6">Brand Compliance Dashboard</h1>
        <GlassCard intensity="subtle" className="p-8 text-center">
          <p className="text-sm text-muted-foreground">
            No compliance data yet. Run a brand voice check on your project content to see results
            here.
          </p>
        </GlassCard>
      </div>
    );
  }

  return (
    <div className={cn("space-y-6", className)}>
      <h1 className="text-lg font-semibold">Brand Compliance Dashboard</h1>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Overall Score */}
        <GlassCard intensity="subtle">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Overall Score</CardTitle>
          </CardHeader>
          <CardContent className="flex justify-center pb-4">
            <div className="relative">
              <BrandScoreGauge score={score.overall} size={140} />
            </div>
          </CardContent>
        </GlassCard>

        {/* Dimension Breakdown */}
        <GlassCard intensity="subtle">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Dimensions</CardTitle>
          </CardHeader>
          <CardContent>
            <BrandDimensionBreakdown dimensions={score.dimensions} />
          </CardContent>
        </GlassCard>

        {/* Trend */}
        <GlassCard intensity="subtle">
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
                          t.avg_score >= 80
                            ? "bg-green-500"
                            : t.avg_score >= 60
                              ? "bg-yellow-500"
                              : "bg-red-500",
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
        </GlassCard>
      </div>

      {/* Issue Density */}
      {recentScores.length > 0 && (
        <GlassCard intensity="subtle">
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
                  <span className="text-muted-foreground truncate max-w-[200px]">
                    {s.block_id}
                  </span>
                  <span className="text-muted-foreground">{s.locale}</span>
                  <span
                    className={cn(
                      "font-medium tabular-nums",
                      s.score >= 80
                        ? "text-green-500"
                        : s.score >= 60
                          ? "text-yellow-500"
                          : "text-red-500",
                    )}
                  >
                    {s.score}
                  </span>
                  <span className="text-muted-foreground">
                    {s.findings.length} finding{s.findings.length !== 1 ? "s" : ""}
                  </span>
                </div>
              ))}
            </div>
          </CardContent>
        </GlassCard>
      )}

      {/* Findings */}
      <GlassCard intensity="subtle">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">
            Findings ({score.findings.length})
          </CardTitle>
        </CardHeader>
        <CardContent>
          <BrandFindingsList findings={score.findings} />
        </CardContent>
      </GlassCard>
    </div>
  );
}
