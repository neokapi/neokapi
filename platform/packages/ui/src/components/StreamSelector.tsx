import type { StreamInfo, StreamVisibility } from "../types/api";
import { ChevronsUpDown, Globe, Lock, Plus, User } from "./icons";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "./ui/dropdown-menu";

export interface StreamSelectorProps {
  streams: StreamInfo[];
  activeStream: StreamInfo | null;
  onStreamChange: (stream: StreamInfo) => void;
  onCreateStream?: () => void;
}

const visibilityColor: Record<StreamVisibility, string> = {
  public: "bg-emerald-500",
  private: "bg-red-500",
  shared: "bg-blue-500",
};

const visibilityLabel: Record<StreamVisibility, string> = {
  public: "Public",
  private: "Private",
  shared: "Shared",
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

/** Group streams by visibility for display. */
function groupStreams(
  streams: StreamInfo[],
): { visibility: StreamVisibility; streams: StreamInfo[] }[] {
  const order: StreamVisibility[] = ["public", "shared", "private"];
  const groups: { visibility: StreamVisibility; streams: StreamInfo[] }[] = [];
  for (const vis of order) {
    const items = streams.filter((s) => s.visibility === vis && !s.archived);
    if (items.length > 0) {
      groups.push({ visibility: vis, streams: items });
    }
  }
  return groups;
}

/** Dropdown selector for switching between streams. */
export function StreamSelector({
  streams,
  activeStream,
  onStreamChange,
  onCreateStream,
}: StreamSelectorProps) {
  const groups = groupStreams(streams);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center gap-2 px-3 py-1.5 bg-transparent border border-border/50 cursor-pointer transition-colors outline-none text-foreground hover:bg-accent rounded-md text-sm">
        {activeStream && (
          <span
            className={`inline-block h-2 w-2 rounded-full ${visibilityColor[activeStream.visibility]}`}
          />
        )}
        <span className="truncate max-w-[160px]">{activeStream?.name || "Select stream"}</span>
        <ChevronsUpDown className="w-3.5 h-3.5 shrink-0 opacity-50" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-[240px]">
        {groups.map((group, gi) => (
          <div key={group.visibility}>
            {gi > 0 && <DropdownMenuSeparator />}
            <div className="px-2 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
              <VisibilityIcon
                visibility={group.visibility}
                className="inline h-3 w-3 mr-1 align-text-bottom"
              />
              {visibilityLabel[group.visibility]}
            </div>
            {group.streams.map((stream) => (
              <DropdownMenuItem
                key={stream.name}
                onClick={() => onStreamChange(stream)}
                className="flex items-center gap-2"
              >
                <span
                  className={`inline-block h-2 w-2 rounded-full shrink-0 ${visibilityColor[stream.visibility]}`}
                />
                <span className="flex-1 truncate text-sm">{stream.name}</span>
                {stream.name === activeStream?.name && (
                  <span className="text-[10px] text-muted-foreground">current</span>
                )}
              </DropdownMenuItem>
            ))}
          </div>
        ))}
        {onCreateStream && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onCreateStream} className="flex items-center gap-2">
              <Plus className="w-4 h-4" />
              <span className="text-sm">Create stream</span>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
