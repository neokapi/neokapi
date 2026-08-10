import { Badge, cn } from "@neokapi/ui-primitives";
import type { BrandVoiceFinding, BrandSeverity } from "./types";
import { ListCapRow } from "../components/ListCapRow";

interface BrandFindingsListProps {
  findings: BrandVoiceFinding[];
  className?: string;
}

/** Hard render cap — a large scan can produce hundreds of findings. */
const MAX_FINDINGS = 200;

const severityStyles: Record<BrandSeverity, string> = {
  neutral: "bg-muted text-muted-foreground",
  minor: "bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning",
  major: "bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning",
  critical: "bg-destructive/10 text-destructive dark:bg-destructive/20 dark:text-destructive",
};

export function BrandFindingsList({ findings, className }: BrandFindingsListProps) {
  if (findings.length === 0) {
    return (
      <div className={cn("text-sm text-muted-foreground text-center py-6", className)}>
        No findings. Content is fully compliant.
      </div>
    );
  }

  return (
    <ul className={cn("space-y-2", className)}>
      {findings.slice(0, MAX_FINDINGS).map((finding, i) => (
        <li key={i} className="flex items-start gap-3 rounded-md border p-3 text-sm bg-card/50">
          <Badge className={cn("shrink-0 text-[10px]", severityStyles[finding.severity])}>
            {finding.severity}
          </Badge>
          <div className="flex-1 min-w-0 space-y-1">
            <p>{finding.message}</p>
            {finding.suggestion && (
              <p className="text-xs text-muted-foreground">Suggestion: {finding.suggestion}</p>
            )}
            <div className="flex gap-2 text-[10px] text-muted-foreground">
              <span className="capitalize">{(finding.category ?? "").replace("_", " ")}</span>
              {finding.original_text && (
                <span className="truncate max-w-[200px]">
                  &ldquo;{finding.original_text}&rdquo;
                </span>
              )}
            </div>
          </div>
        </li>
      ))}
      {findings.length > MAX_FINDINGS && (
        <li>
          <ListCapRow
            shown={MAX_FINDINGS}
            total={findings.length}
            noun="findings"
            hint="Fix the findings above to surface the rest."
            className="border border-border rounded-md"
          />
        </li>
      )}
    </ul>
  );
}
