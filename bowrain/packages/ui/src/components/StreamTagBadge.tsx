import type { StreamTag, StreamTagKind } from "../types/api";
import { GitMerge, Flag, Tag } from "./icons";

export interface StreamTagBadgeProps {
  tag: StreamTag;
  /** Smaller variant for inline use. */
  compact?: boolean;
}

const kindConfig: Record<StreamTagKind, { label: string; color: string; bg: string }> = {
  merge: { label: "Merge", color: "text-purple-600 dark:text-purple-400", bg: "bg-purple-500/10" },
  release: { label: "Release", color: "text-blue-600 dark:text-blue-400", bg: "bg-blue-500/10" },
  milestone: {
    label: "Milestone",
    color: "text-emerald-600 dark:text-emerald-400",
    bg: "bg-emerald-500/10",
  },
  custom: { label: "Tag", color: "text-gray-600 dark:text-gray-400", bg: "bg-gray-500/10" },
};

const KindIcon = ({ kind, className }: { kind: StreamTagKind; className?: string }) => {
  switch (kind) {
    case "merge":
      return <GitMerge className={className} />;
    case "release":
      return <Flag className={className} />;
    default:
      return <Tag className={className} />;
  }
};

/** Compact badge showing a stream tag with kind-specific styling. */
export function StreamTagBadge({ tag, compact }: StreamTagBadgeProps) {
  const config = kindConfig[tag.kind] || kindConfig.custom;

  if (compact) {
    return (
      <span
        className={`inline-flex items-center gap-1 text-xs ${config.color}`}
        title={`${config.label}: ${tag.name}`}
      >
        <KindIcon kind={tag.kind} className="h-3 w-3" />
        <span className="truncate max-w-[100px]">{tag.name}</span>
      </span>
    );
  }

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-md border border-border/50 px-2 py-0.5 text-xs font-medium ${config.bg} ${config.color}`}
    >
      <KindIcon kind={tag.kind} className="h-3 w-3" />
      <span className="truncate max-w-[140px]">{tag.name}</span>
    </span>
  );
}
