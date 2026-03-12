import type { StreamDiffResult, BlockChangeInfo } from "../types/api";
import { Plus, ArrowRight, Trash2 } from "./icons";

export interface StreamDiffViewProps {
  diff: StreamDiffResult;
}

type ChangeType = BlockChangeInfo["change_type"];

const changeConfig: Record<ChangeType, { label: string; color: string; bgColor: string; icon: typeof Plus }> = {
  added:    { label: "Added",    color: "text-emerald-600 dark:text-emerald-400", bgColor: "bg-emerald-500/10", icon: Plus },
  modified: { label: "Modified", color: "text-amber-600 dark:text-amber-400",    bgColor: "bg-amber-500/10",   icon: ArrowRight },
  removed:  { label: "Removed",  color: "text-red-600 dark:text-red-400",        bgColor: "bg-red-500/10",     icon: Trash2 },
};

const changeOrder: ChangeType[] = ["added", "modified", "removed"];

/** Categorize changes by type. */
function groupChanges(changes: BlockChangeInfo[]): Record<ChangeType, BlockChangeInfo[]> {
  const result: Record<ChangeType, BlockChangeInfo[]> = { added: [], modified: [], removed: [] };
  for (const c of changes) {
    result[c.change_type].push(c);
  }
  return result;
}

/** Displays the diff between a stream and its parent, grouped by change type. */
export function StreamDiffView({ diff }: StreamDiffViewProps) {
  const grouped = groupChanges(diff.changes);

  if (diff.changes.length === 0) {
    return (
      <div className="rounded-lg border border-border/50 p-6 text-center text-sm text-muted-foreground">
        No differences between <span className="font-medium text-foreground">{diff.stream_name}</span> and{" "}
        <span className="font-medium text-foreground">{diff.parent_name}</span>.
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Summary bar */}
      <div className="flex items-center gap-4 text-sm">
        <span className="text-muted-foreground">
          Comparing <span className="font-medium text-foreground">{diff.stream_name}</span> to{" "}
          <span className="font-medium text-foreground">{diff.parent_name}</span>
        </span>
        <span className="ml-auto text-muted-foreground">
          {diff.changes.length} {diff.changes.length === 1 ? "change" : "changes"}
        </span>
      </div>

      {/* Count badges */}
      <div className="flex items-center gap-3">
        {changeOrder.map((type) => {
          const count = grouped[type].length;
          if (count === 0) return null;
          const cfg = changeConfig[type];
          return (
            <span
              key={type}
              className={`inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium ${cfg.color} ${cfg.bgColor}`}
            >
              <cfg.icon className="h-3 w-3" />
              {count} {cfg.label.toLowerCase()}
            </span>
          );
        })}
      </div>

      {/* Change lists */}
      {changeOrder.map((type) => {
        const items = grouped[type];
        if (items.length === 0) return null;
        const cfg = changeConfig[type];

        return (
          <div key={type}>
            <h4 className={`mb-2 text-xs font-semibold uppercase tracking-wider ${cfg.color}`}>
              {cfg.label} ({items.length})
            </h4>
            <div className="rounded-lg border border-border/50 divide-y divide-border/50">
              {items.map((change) => (
                <div
                  key={change.block_id}
                  className="flex items-center gap-3 px-3 py-2 text-sm"
                >
                  <cfg.icon className={`h-3.5 w-3.5 shrink-0 ${cfg.color}`} />
                  <span className="font-mono text-xs truncate flex-1">{change.block_id}</span>
                  {change.old_hash && change.new_hash && (
                    <span className="text-[10px] text-muted-foreground font-mono">
                      {change.old_hash.slice(0, 7)} → {change.new_hash.slice(0, 7)}
                    </span>
                  )}
                </div>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}
