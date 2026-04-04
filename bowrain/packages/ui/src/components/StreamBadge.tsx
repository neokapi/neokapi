import type { StreamInfo, StreamVisibility } from "../types/api";
import { Globe, Lock, User } from "./icons";

export interface StreamBadgeProps {
  stream: StreamInfo;
  /** Smaller variant for inline use. */
  compact?: boolean;
}

const visibilityColor: Record<StreamVisibility, string> = {
  public: "bg-success",
  private: "bg-destructive",
  shared: "bg-info",
};

const VisibilityIcon = ({
  visibility,
  className,
}: {
  visibility: StreamVisibility;
  className?: string;
}) => {
  switch (visibility) {
    case "public":
      return <Globe className={className} />;
    case "private":
      return <Lock className={className} />;
    case "shared":
      return <User className={className} />;
  }
};

/** Small badge showing a stream name with a colored visibility indicator. */
export function StreamBadge({ stream, compact }: StreamBadgeProps) {
  if (compact) {
    return (
      <span
        className="inline-flex items-center gap-1 text-xs text-muted-foreground"
        title={`${stream.name} (${stream.visibility})${stream.locked ? " — locked" : ""}`}
      >
        <span
          className={`inline-block h-1.5 w-1.5 rounded-full ${visibilityColor[stream.visibility]}`}
        />
        <span className="truncate max-w-[100px]">{stream.name}</span>
        {stream.locked && <Lock className="h-3 w-3 text-warning" />}
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-1.5 rounded-md border border-border/50 bg-muted/50 px-2 py-0.5 text-xs font-medium text-foreground">
      <span className={`inline-block h-2 w-2 rounded-full ${visibilityColor[stream.visibility]}`} />
      <VisibilityIcon visibility={stream.visibility} className="h-3 w-3 text-muted-foreground" />
      <span className="truncate max-w-[140px]">{stream.name}</span>
      {stream.locked && <Lock className="h-3 w-3 text-warning" />}
    </span>
  );
}
