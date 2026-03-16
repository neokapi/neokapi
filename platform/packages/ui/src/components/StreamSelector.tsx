import type { StreamInfo, StreamVisibility } from "../types/api";
import {
  ChevronDown,
  Globe,
  Lock,
  Plus,
  User,
  GitBranch,
  GitMerge,
  GitPullRequest,
  Archive,
  Pencil,
} from "./icons";
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
  onEditStream?: (stream: StreamInfo) => void;
  onMergeStream?: (streamName: string) => void;
  onDiffStream?: (streamName: string) => void;
  onDeleteStream?: (streamName: string) => void;
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
  onEditStream,
  onMergeStream,
  onDiffStream,
  onDeleteStream,
}: StreamSelectorProps) {
  const groups = groupStreams(streams);
  const isNonMain = activeStream && activeStream.name !== "main";
  const hasAnyAction = onEditStream || onMergeStream || onDiffStream || onDeleteStream;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="inline-flex items-center gap-1.5 px-3 py-1 bg-muted/60 border border-border/60 cursor-pointer transition-colors outline-none text-foreground hover:bg-muted rounded-full text-sm font-medium">
        <GitBranch className="w-3.5 h-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate max-w-[160px]">{activeStream?.name || "main"}</span>
        <ChevronDown className="w-3 h-3 shrink-0 text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-[260px]">
        {/* Stream actions — edit always available, merge/diff/archive only for non-main */}
        {hasAnyAction && activeStream && (
          <>
            <div className="px-2 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
              Actions
            </div>
            {onEditStream && (
              <DropdownMenuItem
                onClick={() => onEditStream(activeStream)}
                className="flex items-center gap-2"
              >
                <Pencil className="w-4 h-4 text-muted-foreground" />
                <span className="text-sm">Edit stream</span>
              </DropdownMenuItem>
            )}
            {isNonMain && onDiffStream && (
              <DropdownMenuItem
                onClick={() => onDiffStream(activeStream.name)}
                className="flex items-center gap-2"
              >
                <GitPullRequest className="w-4 h-4 text-blue-500" />
                <span className="text-sm">Compare with {activeStream.parent || "main"}</span>
              </DropdownMenuItem>
            )}
            {isNonMain && onMergeStream && (
              <DropdownMenuItem
                onClick={() => onMergeStream(activeStream.name)}
                className="flex items-center gap-2"
              >
                <GitMerge className="w-4 h-4 text-emerald-500" />
                <span className="text-sm">Merge into {activeStream.parent || "main"}</span>
              </DropdownMenuItem>
            )}
            {isNonMain && onDeleteStream && (
              <DropdownMenuItem
                onClick={() => onDeleteStream(activeStream.name)}
                className="flex items-center gap-2 text-destructive"
              >
                <Archive className="w-4 h-4" />
                <span className="text-sm">Archive stream</span>
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
          </>
        )}

        {/* Stream list */}
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
