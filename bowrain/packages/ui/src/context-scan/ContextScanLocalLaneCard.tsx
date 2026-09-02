import { Card, CardContent } from "@neokapi/ui-primitives";
import { Terminal, ArrowRight } from "../components/icons";

export interface ContextScanLocalLaneCardProps {
  /** Optional: opens setup guidance (e.g. the MCP/Agent Skill guide). */
  onLearnMore?: () => void;
  className?: string;
}

/**
 * The local onboarding lane: instead of the hosted scan, the user's own AI
 * assistant reads their codebase with the kapi Agent Skill and drafts a
 * profile locally, then pushes it. Deeper than the hosted scan and free.
 */
export function ContextScanLocalLaneCard({
  onLearnMore,
  className,
}: ContextScanLocalLaneCardProps) {
  return (
    <Card className={className}>
      <CardContent className="pt-4 pb-4">
        <div className="flex items-start gap-3">
          <div className="flex items-center justify-center w-9 h-9 rounded-lg bg-muted shrink-0">
            <Terminal className="w-4 h-4 text-muted-foreground" />
          </div>
          <div className="min-w-0">
            <h3 className="text-sm font-semibold">Have a codebase?</h3>
            <p className="text-xs text-muted-foreground leading-relaxed mt-1">
              The kapi Agent Skill lets your own AI assistant read the whole repository and draft a
              richer profile locally, deeper and free.
            </p>
            {onLearnMore && (
              <button
                type="button"
                onClick={onLearnMore}
                className="mt-2 text-xs font-medium text-primary flex items-center gap-1 bg-transparent border-none p-0 cursor-pointer"
              >
                See how <ArrowRight className="w-3 h-3" />
              </button>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
