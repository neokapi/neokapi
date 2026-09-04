// A parallel group in the linear flow editor.
//
// A flow is an ordered pipeline, and one entry in it can fan out: a parallel
// group runs several branches at once. It reads as a bordered block that holds
// its branches as rows, each a tool with its own key options, with no order
// between them. Branches are added, removed, and configured here; the group as a
// whole moves and is removed like any other step.

import { GripVertical, Split, Trash2, ChevronUp, ChevronDown } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { SimpleTooltip } from "../ui/tooltip";
import type { ComponentSchema, SchemaFormHost } from "../schema-form";
import type { FlowStep, FlowTool } from "./types";
import { StepRow } from "./StepRow";
import { AddStepPicker } from "./AddStepPicker";

export interface ParallelGroupRowProps {
  /** The parallel step (its `parallel` array holds the branches). */
  step: FlowStep;
  index: number;
  count: number;
  tools: FlowTool[];
  onGetSchema?: (toolName: string) => ComponentSchema | null;
  host?: SchemaFormHost;
  readOnly?: boolean;
  /** Replace the whole group step (a branch added, removed, or reconfigured). */
  onChange?: (step: FlowStep) => void;
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
  onRemove,
  onMoveUp,
  onMoveDown,
}: ParallelGroupRowProps) {
  const branches = step.parallel ?? [];

  const setBranches = (next: FlowStep[]) => onChange?.({ ...step, parallel: next });
  const addBranch = (toolName: string) => setBranches([...branches, { tool: toolName }]);
  const removeBranch = (bi: number) => setBranches(branches.filter((_, j) => j !== bi));
  const setBranchConfig = (bi: number, config: Record<string, unknown>) =>
    setBranches(branches.map((b, j) => (j === bi ? { ...b, config } : b)));

  return (
    <li className="rounded-lg border border-dashed" data-testid="parallel-group">
      <div className="flex items-start gap-2 p-2">
        <span className="mt-1 text-muted-foreground/50" aria-hidden="true">
          <GripVertical className="size-4" />
        </span>
        <Split className="mt-1 size-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-foreground">
              {step.label || t("Parallel")}
            </span>
            <Badge variant="secondary" className="font-normal">
              {t("{count} in parallel", { count: branches.length })}
            </Badge>
          </div>
          <div className="text-[11px] text-muted-foreground">
            {t("These branches run at the same time.")}
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

      <div className="border-t p-2 pl-4">
        {branches.length === 0 ? (
          <p className="px-1 py-2 text-[11px] text-muted-foreground">
            {t("No branches yet. Add one to fan the flow out.")}
          </p>
        ) : (
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
        )}
        {!readOnly && (
          <div className="mt-2">
            <AddStepPicker
              tools={tools}
              onAdd={addBranch}
              label={t("Add branch")}
              triggerTestId="add-branch"
            />
          </div>
        )}
      </div>
    </li>
  );
}
