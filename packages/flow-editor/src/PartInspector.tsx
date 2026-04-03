import { cn, ScrollArea, PanelHeader } from "@neokapi/ui-primitives";
import type { PartSnapshotSet, PartSnapshot } from "./traceTypes";

interface PartInspectorProps {
  nodeId: string;
  nodeName: string;
  /** All part snapshot sets from the trace. */
  parts: Record<string, PartSnapshotSet>;
}

/** Single row showing before/after state for a block part. */
function PartRow({
  before,
  after,
}: {
  before: PartSnapshot;
  after: PartSnapshot;
}) {
  const sourceChanged = before.sourceText !== after.sourceText;
  const targetChanged = before.targetText !== after.targetText;

  return (
    <div className="mb-2.5 rounded-md border border-border p-2">
      <div className="mb-1.5 text-[10px] text-muted-foreground">
        {before.summary}
      </div>

      {/* Source text */}
      {before.sourceText && (
        <div className="mb-1">
          <div className="mb-0.5 text-[9px] font-semibold text-muted-foreground">
            SOURCE
          </div>
          <div className="text-[11px] leading-snug text-foreground">
            {after.sourceText || before.sourceText}
            {sourceChanged && (
              <span className="ml-1 text-[9px] text-accent-foreground">
                changed
              </span>
            )}
          </div>
        </div>
      )}

      {/* Target text */}
      <div>
        <div className="mb-0.5 text-[9px] font-semibold text-muted-foreground">
          TARGET
        </div>
        {before.targetText || after.targetText ? (
          <div className="flex flex-col gap-0.5">
            {before.targetText && before.targetText !== after.targetText && (
              <div className="text-[11px] text-muted-foreground line-through opacity-60">
                {before.targetText}
              </div>
            )}
            <div
              className={cn(
                "text-[11px] leading-snug",
                targetChanged
                  ? "text-accent-foreground"
                  : "text-foreground",
              )}
            >
              {after.targetText || "(empty)"}
            </div>
          </div>
        ) : (
          <div className="text-[11px] italic text-muted-foreground">
            (no target)
          </div>
        )}
      </div>
    </div>
  );
}

/**
 * Part inspector sidebar — shows blocks that passed through a node,
 * with before/after source and target text.
 */
export function PartInspector({ nodeId, nodeName, parts }: PartInspectorProps) {
  // Filter to Block-type parts that have snapshots for this node.
  const relevantParts = Object.entries(parts).filter(
    ([, ss]) => ss.initial.type === "Block" && ss.afterNode?.[nodeId],
  );

  return (
    <div className="flex w-[300px] flex-col overflow-hidden border-l border-border bg-background">
      <PanelHeader className="py-2.5 flex-col items-start gap-0.5">
        <div className="text-[11px] font-semibold text-foreground">Part Inspector</div>
        <div className="text-[10px] text-muted-foreground">
          {nodeName} &mdash; {relevantParts.length} block{relevantParts.length !== 1 ? "s" : ""}
        </div>
      </PanelHeader>

      <ScrollArea className="flex-1">
        <div className="px-3 py-2">
          {relevantParts.length === 0 && (
            <div className="py-5 text-center text-[11px] italic text-muted-foreground">
              No block data for this node.
            </div>
          )}

          {relevantParts.map(([partId, ss]) => (
            <PartRow
              key={partId}
              before={ss.initial}
              after={ss.afterNode![nodeId]}
            />
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}
