// The ops diff (AD-021): a change-set's ordered operations rendered as legible
// change rows — "Ban “utilize” (en-US)", "Add relation cloud replaced by cloud
// services" — grouped by the part of the graph they touch, with a governed
// badge on the ops that need a second approval to merge. Read-only by default;
// pass `editable` + `onRemove` to let a draft drop ops.
import { Badge, Button, cn } from "@neokapi/ui-primitives";
import { Lock, Trash2, GitMerge } from "../../components/icons";
import type { ChangeSetOp } from "../../types/brand-graph";
import { groupOps, governedOpCount, CATEGORY_LABEL, type OpDiffRow } from "./ops";

export interface OpsDiffProps {
  ops: ChangeSetOp[];
  editable?: boolean;
  onRemove?: (seq: number) => void;
  removingSeq?: number | null;
  className?: string;
}

export function OpsDiff({ ops, editable, onRemove, removingSeq, className }: OpsDiffProps) {
  if (ops.length === 0) {
    return (
      <p className={cn("text-sm text-muted-foreground", className)}>
        No operations yet. Add concept, term, relation, or voice-rule edits to this draft.
      </p>
    );
  }

  const groups = groupOps(ops);
  const governed = governedOpCount(ops);

  return (
    <div className={cn("space-y-4", className)}>
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span>
          {ops.length} operation{ops.length === 1 ? "" : "s"}
        </span>
        {governed > 0 && (
          <Badge variant="outline" className="gap-1 border-primary/40 text-[10px] text-primary">
            <Lock className="size-3" />
            {governed} governed
          </Badge>
        )}
      </div>

      {groups.map((group) => (
        <section key={group.category} className="space-y-1.5">
          <h4 className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
            {CATEGORY_LABEL[group.category]}
          </h4>
          <ul className="space-y-1.5">
            {group.rows.map(({ op, row }) => (
              <DiffRow
                key={op.seq}
                seq={op.seq}
                row={row}
                editable={editable}
                onRemove={onRemove}
                removing={removingSeq === op.seq}
              />
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

const TONE_BORDER: Record<OpDiffRow["tone"], string> = {
  default: "border-l-border",
  destructive: "border-l-destructive/60",
  success: "border-l-success/60",
};

const VERB_TONE: Record<OpDiffRow["tone"], string> = {
  default: "text-foreground",
  destructive: "text-destructive",
  success: "text-success",
};

function DiffRow({
  seq,
  row,
  editable,
  onRemove,
  removing,
}: {
  seq: number;
  row: OpDiffRow;
  editable?: boolean;
  onRemove?: (seq: number) => void;
  removing?: boolean;
}) {
  return (
    <li
      className={cn(
        "flex items-center gap-3 rounded-md border border-l-2 bg-card px-3 py-2 text-sm",
        TONE_BORDER[row.tone],
      )}
    >
      <span className={cn("shrink-0 text-xs font-semibold", VERB_TONE[row.tone])}>{row.verb}</span>
      <span className="min-w-0 flex-1 truncate text-foreground">{row.summary}</span>
      {row.governed && (
        <Badge
          variant="outline"
          className="hidden shrink-0 gap-1 border-primary/40 text-[10px] text-primary sm:inline-flex"
          title="Governed — needs a second approval to merge"
        >
          <GitMerge className="size-3" />
          governed
        </Badge>
      )}
      {editable && onRemove && (
        <Button
          size="icon"
          variant="ghost"
          className="size-7 shrink-0 text-muted-foreground hover:text-destructive"
          onClick={() => onRemove(seq)}
          disabled={removing}
          aria-label={`Remove operation ${seq}`}
        >
          <Trash2 />
        </Button>
      )}
    </li>
  );
}
