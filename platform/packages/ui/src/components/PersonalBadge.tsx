import { User } from "./icons";
import { Badge } from "./ui/badge";
import { cn } from "../lib/utils";

interface PersonalBadgeProps {
  className?: string;
}

export function PersonalBadge({ className }: PersonalBadgeProps) {
  return (
    <Badge variant="outline" className={cn("gap-0.5 px-1.5 py-0 text-[10px]", className)}>
      <User className="size-2.5!" />
      Personal
    </Badge>
  );
}
