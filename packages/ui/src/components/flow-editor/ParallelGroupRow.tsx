// A parallel group in the linear flow editor, which the runtime refuses.
//
// Flow steps run in order, one after another, so a step holding a `parallel:`
// list cannot run. The editor creates none; a flow loaded with one shows the
// group as invalid, with the refusal the runtime gives, and offers to list its
// tools as ordered steps in its place. Until then the group's branches stay
// readable and can be configured or removed, and the group as a whole moves
// and is removed like any other step.

import {
  AlertCircle,
  ChevronDown,
  ChevronUp,
  GripVertical,
  ListOrdered,
  Trash2,
} from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Alert, AlertDescription } from "../ui/alert";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { SimpleTooltip } from "../ui/tooltip";
import type { ComponentSchema, SchemaFormHost } from "../schema-form";
import type { FlowStep, FlowTool } from "./types";
import { StepRow } from "./StepRow";
import { parallelStepError } from "./sequence";

export interface ParallelGroupRowProps {
  /** The parallel step (its `parallel` array holds the branches). */
  step: FlowStep;
  index: number;
  count: number;
  tools: FlowTool[];
  onGetSchema?: (toolName: string) => ComponentSchema | null;
  host?: SchemaFormHost;
  readOnly?: boolean;
  /** Replace the whole group step (a branch removed or reconfigured). */
  onChange?: (step: FlowStep) => void;
  /** Replace the group with its branches as ordered steps. */
  onListInOrder?: () => void;
  onRemove?: () => void;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
}

export function ParallelGroupRow({
  step,
  index,
  count,
  tools,
  onGetSchema,
  host,
  readOnly,
  onChange,
  onListInOrder,
  onRemove,
  onMoveUp,
  onMoveDown,
}: ParallelGroupRowProps) {
  const branches = step.parallel ?? [];
  const refusal = parallelStepError(step, index);

  const setBranches = (next: FlowStep[]) => onChange?.({ ...step, parallel: next });
  const removeBranch = (bi: number) => setBranches(branches.filter((_, j) => j !== bi));
  const setBranchConfig = (bi: number, config: Record<string, unknown>) =>
    setBranches(branches.map((b, j) => (j === bi ? { ...b, config } : b)));

  return (
    <li
      className="rounded-lg border border-dashed border-destructive/60"
      data-testid="parallel-group"
      data-invalid="true"
    >
      <div className="flex items-start gap-2 p-2">
        <span className="mt-1 text-muted-foreground/50" aria-hidden="true">
          <GripVertical className="size-4" />
        </span>
        <AlertCircle className="mt-1 size-4 shrink-0 text-destructive" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-foreground">
              {step.label || t("Parallel group")}
            </span>
            <Badge variant="secondary" className="font-normal">
              {t("{count} tools", { count: branches.length })}
            </Badge>
          </div>
        </div>

        {!readOnly && (
          <div className="flex items-center gap-0.5">
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={t("Move up")}
              disabled={index === 0}
              onClick={onMoveUp}
            >
              <ChevronUp className="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={t("Move down")}
              disabled={index === count - 1}
              onClick={onMoveDown}
            >
              <ChevronDown className="size-3.5" />
            </Button>
            <SimpleTooltip content={t("Remove parallel group")}>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t("Remove parallel group")}
                onClick={onRemove}
              >
                <Trash2 className="size-3.5" />
              </Button>
            </SimpleTooltip>
          </div>
        )}
      </div>

      {refusal && (
        <div className="px-2 pb-2">
          <Alert variant="destructive" data-testid="parallel-group-error">
            <AlertDescription className="text-[11px]">{refusal}</AlertDescription>
          </Alert>
          {!readOnly && onListInOrder && (
            <Button
              variant="outline"
              size="xs"
              className="mt-2"
              onClick={onListInOrder}
              data-testid="list-in-order"
            >
              <ListOrdered className="mr-1 size-3.5" />
              {t("List as ordered steps")}
            </Button>
          )}
        </div>
      )}

      {branches.length > 0 && (
        <div className="border-t p-2 pl-4">
          <ul className="space-y-2" data-testid="parallel-branches">
            {branches.map((branch, bi) => (
              <StepRow
                key={`${branch.tool}-${bi}`}
                step={branch}
                tool={tools.find((tl) => tl.name === branch.tool)}
                index={bi}
                count={branches.length}
                schema={onGetSchema?.(branch.tool)}
                host={host}
                readOnly={readOnly}
                hideMove
                onConfigChange={(config) => setBranchConfig(bi, config)}
                onRemove={() => removeBranch(bi)}
              />
            ))}
          </ul>
        </div>
      )}
    </li>
  );
}
