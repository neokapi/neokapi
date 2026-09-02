import { GitBranch, X, Zap } from "lucide-react";
import { Button, PanelHeader } from "@neokapi/ui-primitives";
import type { ParallelSuggestion } from "./parallelChecker";

export interface ParallelSuggestionBannerProps {
  suggestion: ParallelSuggestion;
  onParallelize: (suggestion: ParallelSuggestion) => void;
  onDismiss: () => void;
}

export function ParallelSuggestionBanner({
  suggestion,
  onParallelize,
  onDismiss,
}: ParallelSuggestionBannerProps) {
  return (
    <PanelHeader
      className="py-1.5 bg-secondary text-[11px]"
      actions={
        <>
          <Button size="xs" onClick={() => onParallelize(suggestion)} className="text-[11px]">
            <GitBranch size={11} />
            Parallelize
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onDismiss}
            aria-label="Dismiss suggestion"
          >
            <X size={12} className="text-muted-foreground" />
          </Button>
        </>
      }
    >
      <Zap size={13} className="text-accent-foreground shrink-0" />
      <span className="text-muted-foreground flex-1">
        <strong className="text-foreground">{suggestion.toolNames.join(", ")}</strong> can run in
        parallel: {suggestion.reason}
      </span>
    </PanelHeader>
  );
}
