interface MatchScoreBarProps {
  score: number; // 0.0 - 1.0
  matchType: string; // "generalized-exact", "fuzzy", etc.
  className?: string;
}

function scoreColor(score: number): string {
  if (score >= 1.0) return "oklch(0.55 0.18 252)"; // primary blue — exact
  if (score >= 0.85) return "oklch(0.55 0.17 155)"; // green — high
  if (score >= 0.7) return "oklch(0.65 0.16 85)"; // amber — medium
  return "oklch(0.55 0.2 27)"; // red — low
}

function matchTypeLabel(matchType: string): string {
  switch (matchType) {
    case "generalized-exact":
      return "gen-exact";
    case "structural-exact":
      return "struct-exact";
    case "exact":
      return "exact";
    case "generalized-fuzzy":
      return "gen-fuzzy";
    case "structural-fuzzy":
      return "struct-fuzzy";
    case "fuzzy":
      return "fuzzy";
    default:
      return matchType;
  }
}

/**
 * Horizontal bar visualizing a TM match score (0-1.0).
 * Color coded: red < 0.7, amber 0.7-0.85, green 0.85-0.99, blue 1.0.
 */
export function MatchScoreBar({ score, matchType, className }: MatchScoreBarProps) {
  const pct = Math.round(score * 100);
  const color = scoreColor(score);

  return (
    <div className={`flex items-center gap-2 ${className ?? ""}`}>
      <div className="relative h-1.5 flex-1 rounded-full bg-muted overflow-hidden">
        <div
          className="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
          style={{ width: `${pct}%`, backgroundColor: color }}
        />
      </div>
      <span
        className="text-[11px] font-semibold tabular-nums min-w-[32px] text-right"
        style={{ color }}
      >
        {pct}%
      </span>
      <span className="text-[10px] text-muted-foreground font-mono">
        {matchTypeLabel(matchType)}
      </span>
    </div>
  );
}
