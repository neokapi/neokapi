import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { AlertTriangle } from "../icons";

interface BravoStepUpCardProps {
  currentMode: string;
  requiredMode: string;
  action: string;
  onSwitchMode: (mode: string) => void;
  onDismiss: () => void;
}

export function BravoStepUpCard({
  currentMode,
  requiredMode,
  action,
  onSwitchMode,
  onDismiss,
}: BravoStepUpCardProps) {
  const modeLabels: Record<string, string> = {
    ask: "Ask",
    coworker: "Co-worker",
    voice: "Voice",
    bravo: "Voice",
  };

  return (
    <div className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 my-2">
      <div className="flex items-start gap-2">
        <AlertTriangle className="h-4 w-4 text-amber-500 mt-0.5 shrink-0" />
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium">Mode restriction</p>
          <p className="text-sm text-muted-foreground mt-0.5">
            {action} requires {modeLabels[requiredMode] ?? requiredMode} mode. You're currently in{" "}
            {modeLabels[currentMode] ?? currentMode} mode.
          </p>
          <div className="flex gap-2 mt-3">
            <Button size="sm" onClick={() => onSwitchMode(requiredMode)}>
              Switch to {modeLabels[requiredMode] ?? requiredMode}
            </Button>
            <Button size="sm" variant="outline" onClick={onDismiss}>
              Stay in {modeLabels[currentMode] ?? currentMode}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
