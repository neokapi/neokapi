import { cn } from "@neokapi/ui-primitives";
import { DEFAULT_MIN_SCORE, scoreStrokeClass, scoreTextClass } from "./complianceBar";

interface BrandScoreGaugeProps {
  score: number;
  /** The profile's compliant bar; DEFAULT_MIN_SCORE when no profile is loaded. */
  bar?: number;
  size?: number;
  className?: string;
  label?: string;
}

export function BrandScoreGauge({
  score,
  bar = DEFAULT_MIN_SCORE,
  size = 120,
  className,
  label,
}: BrandScoreGaugeProps) {
  const clamped = Math.max(0, Math.min(100, score));
  const radius = 45;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference - (clamped / 100) * circumference;

  return (
    <div className={cn("flex flex-col items-center gap-1", className)}>
      <svg width={size} height={size} viewBox="0 0 100 100" className="-rotate-90">
        <circle cx="50" cy="50" r={radius} fill="none" className="stroke-muted" strokeWidth="8" />
        <circle
          cx="50"
          cy="50"
          r={radius}
          fill="none"
          className={cn(
            scoreStrokeClass(clamped, bar),
            "transition-[stroke-dashoffset] duration-[600ms] ease-in-out",
          )}
          strokeWidth="8"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
        />
      </svg>
      <div
        className="absolute flex flex-col items-center justify-center"
        style={{ width: size, height: size }}
      >
        <span
          data-testid="brand-score"
          data-below-bar={clamped < bar}
          className={cn("text-2xl font-bold tabular-nums", scoreTextClass(clamped, bar))}
        >
          {clamped}
        </span>
      </div>
      {label && <span className="text-xs text-muted-foreground">{label}</span>}
    </div>
  );
}
