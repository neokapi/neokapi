// The linear flow editor: a flow is an ordered pipeline of tool steps, not a
// graph, so it is edited as a reorderable list rather than a node canvas.
//
// This is the shared, surface-agnostic editor: it takes a flow, the available
// tools, and callbacks, and renders the ordered step list with add / remove /
// reorder and inline key options (the tool's own SchemaForm). It depends on no
// host's API types and no node-canvas package, so kapi and bowrain can share
// one editor. The header carries the same outcome line and step-chip strip a
// flow card shows, so the card and the editor read as one thing.

import { useState, type ReactNode } from "react";
import { ArrowRight, Check, Pencil, Play, Star, X } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Markdown } from "../ui/markdown";
import { Switch } from "../ui/switch";
import type { ComponentSchema, SchemaFormHost } from "../schema-form";
import type { FlowSpec, FlowStep, FlowTool } from "./types";
import { StepRow } from "./StepRow";
import { AddStepPicker } from "./AddStepPicker";
import { ParallelGroupRow } from "./ParallelGroupRow";

/** A step's label for the chip strip: its own label, else the tool's name. */
function stepLabel(step: FlowStep, tools: FlowTool[]): string {
  if (step.label) return step.label;
  if (Array.isArray(step.parallel)) return t("Parallel group");
  const tool = tools.find((tl) => tl.name === step.tool);
  return tool?.display_name || step.tool;
}

export interface LinearFlowEditorProps {
  flowName: string;
  flow: FlowSpec;
  tools: FlowTool[];
  onChange: (spec: FlowSpec) => void;
  /** Resolve a tool's option schema (cached by the host). */
  onGetSchema?: (toolName: string) => ComponentSchema | null;
  host?: SchemaFormHost;
  onRun?: () => void;
  runDisabled?: boolean;
  readOnly?: boolean;
  /** True when this is the project's default flow. Absent hides the toggle. */
  isDefault?: boolean;
  onToggleDefault?: (next: boolean) => void;
  /** Absent leaves the name read-only. */
  onRename?: (next: string) => void;
  /**
   * Rendered in the empty state under the prompt, for hosts that offer a
   * template library (the shared FlowTemplateLibrary). Omitted leaves just the
   * blank-start "Add step" affordance.
   */
  templateLibrary?: ReactNode;
}

export function LinearFlowEditor({
  flowName,
  flow,
  tools,
  onChange,
  onGetSchema,
  host,
  onRun,
  runDisabled,
  readOnly,
  isDefault,
  onToggleDefault,
  onRename,
  templateLibrary,
}: LinearFlowEditorProps) {
  const steps = flow.steps ?? [];
  const [renaming, setRenaming] = useState(false);
  const [draftName, setDraftName] = useState(flowName);

  const setSteps = (next: FlowStep[]) => onChange({ ...flow, steps: next });
  const addStep = (toolName: string) => setSteps([...steps, { tool: toolName }]);
  const removeStep = (i: number) => setSteps(steps.filter((_, j) => j !== i));
  const move = (i: number, delta: number) => {
    const j = i + delta;
    if (j < 0 || j >= steps.length) return;
    const next = [...steps];
    [next[i], next[j]] = [next[j], next[i]];
    setSteps(next);
  };
  const setConfig = (i: number, config: Record<string, unknown>) => {
    const next = steps.map((s, j) => (j === i ? { ...s, config } : s));
    setSteps(next);
  };
  const setStep = (i: number, next: FlowStep) =>
    setSteps(steps.map((s, j) => (j === i ? next : s)));
  const addParallelGroup = (toolName: string) =>
    setSteps([...steps, { tool: "", parallel: [{ tool: toolName }] }]);

  /** The add-step / add-parallel-group affordances, shown in both empty and list footers. */
  const addControls = (
    <div className="flex flex-wrap gap-2">
      <AddStepPicker tools={tools} onAdd={addStep} />
      <AddStepPicker
        tools={tools}
        onAdd={addParallelGroup}
        label={t("Add parallel group")}
        triggerTestId="add-parallel-group"
      />
    </div>
  );

  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="linear-flow-editor">
      {/* Header: name, default, outcome and the step-chip strip. */}
      <div className="shrink-0 space-y-2 border-b p-4">
        <div className="flex flex-wrap items-center gap-2">
          {renaming ? (
            <>
              <Input
                value={draftName}
                onChange={(e) => setDraftName(e.target.value)}
                className="h-7 max-w-xs"
                aria-label={t("Flow name")}
                autoFocus
              />
              <Button
                size="icon-xs"
                aria-label={t("Save")}
                onClick={() => {
                  const next = draftName.trim();
                  if (next && next !== flowName) onRename?.(next);
                  setRenaming(false);
                }}
              >
                <Check className="size-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t("Cancel")}
                onClick={() => {
                  setDraftName(flowName);
                  setRenaming(false);
                }}
              >
                <X className="size-3.5" />
              </Button>
            </>
          ) : (
            <>
              <h2 className="text-base font-semibold text-foreground">{flowName}</h2>
              {onRename && !readOnly && (
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label={t("Rename flow")}
                  onClick={() => {
                    setDraftName(flowName);
                    setRenaming(true);
                  }}
                >
                  <Pencil className="size-3.5" />
                </Button>
              )}
              {isDefault && (
                <Badge variant="secondary" className="font-normal" data-testid="flow-default-badge">
                  {t("Default")}
                </Badge>
              )}
            </>
          )}
          <span className="flex-1" />
          {onToggleDefault && !readOnly && (
            <label className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
              <Star className={cn("size-3", isDefault && "fill-current text-amber-500")} />
              {t("Default flow")}
              <Switch
                checked={!!isDefault}
                onCheckedChange={(v) => onToggleDefault(v)}
                aria-label={t("Set as the project's default flow")}
              />
            </label>
          )}
          {onRun && (
            <Button
              size="sm"
              onClick={onRun}
              disabled={runDisabled}
              aria-label={t("Run flow")}
              data-testid="flow-run"
            >
              <Play className="mr-1 size-3.5" />
              {t("Run")}
            </Button>
          )}
        </div>

        {flow.description && (
          <div className="text-xs text-muted-foreground">
            <Markdown inline>{flow.description}</Markdown>
          </div>
        )}
        {steps.length > 0 && (
          <div
            className="flex flex-wrap items-center gap-1"
            data-testid="step-chips"
            aria-label={t("Steps")}
          >
            {steps.map((step, i) => (
              <span key={`${step.tool}-${i}`} className="flex items-center gap-1">
                {i > 0 && <ArrowRight className="size-3 text-muted-foreground/60" />}
                <Badge variant="secondary" className="font-normal">
                  {stepLabel(step, tools)}
                </Badge>
              </span>
            ))}
          </div>
        )}
      </div>

      {/* The step list, or the empty state for a new flow. */}
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {steps.length === 0 ? (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              {t("This flow has no steps yet. Start from a template, or add a step.")}
            </p>
            {!readOnly && templateLibrary}
            {!readOnly && addControls}
          </div>
        ) : (
          <div className="space-y-3">
            <ul className="space-y-2">
              {steps.map((step, i) =>
                Array.isArray(step.parallel) ? (
                  <ParallelGroupRow
                    key={`group-${i}`}
                    step={step}
                    index={i}
                    count={steps.length}
                    tools={tools}
                    onGetSchema={onGetSchema}
                    host={host}
                    readOnly={readOnly}
                    onChange={(next) => setStep(i, next)}
                    onRemove={() => removeStep(i)}
                    onMoveUp={() => move(i, -1)}
                    onMoveDown={() => move(i, 1)}
                  />
                ) : (
                  <StepRow
                    key={`${step.tool}-${i}`}
                    step={step}
                    tool={tools.find((tl) => tl.name === step.tool)}
                    index={i}
                    count={steps.length}
                    schema={onGetSchema?.(step.tool)}
                    host={host}
                    readOnly={readOnly}
                    onConfigChange={(config) => setConfig(i, config)}
                    onRemove={() => removeStep(i)}
                    onMoveUp={() => move(i, -1)}
                    onMoveDown={() => move(i, 1)}
                  />
                ),
              )}
            </ul>
            {!readOnly && addControls}
          </div>
        )}
      </div>
    </div>
  );
}
