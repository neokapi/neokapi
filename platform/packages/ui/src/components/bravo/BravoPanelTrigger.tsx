import { cn } from "../../lib/utils";
import { Button } from "../ui/button";

export interface BravoPanelTriggerProps {
  onClick: () => void;
  active?: boolean;
  hasUnread?: boolean;
}

export function BravoPanelTrigger({ onClick, active, hasUnread }: BravoPanelTriggerProps) {
  return (
    <Button
      variant={active ? "default" : "ghost"}
      size="sm"
      onClick={onClick}
      className={cn("relative gap-1.5")}
    >
      <span className="text-sm">@bravo</span>
      {hasUnread && (
        <span className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-primary" />
      )}
    </Button>
  );
}
