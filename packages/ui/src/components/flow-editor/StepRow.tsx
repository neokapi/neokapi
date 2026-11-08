// One step in the linear flow editor.
//
// A flow is an ordered pipeline, so a step is a row, not a node: the tool it
// runs, a one-line description, a grip that drags it to a new place, and an
// expandable area that renders the step's key options through the tool's own
// schema form (the same form the tools surface uses). Rarely-used options stay
// in the recipe; the form shows the common ones.

import { useState } from "react";
import { ChevronDown, ChevronRight, Trash2 } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button } from "../ui/button";
import { Markdown } from "../ui/markdown";
import { SimpleTooltip } from "../ui/tooltip";
import { SchemaForm } from "../schema-form";
import { ToolCommandCopy } from "./ToolCommandCopy";
import { DropIndicator, ReorderHandle } from "./ReorderHandle";
import { cn } from "../../lib/utils";
import type { StepReorder } from "./reorder";
import type { ComponentSchema, SchemaFormHost } from "../schema-form";
import type { FlowStep, FlowTool } from "./types";

export interface StepRowProps {
  step: FlowStep;
  /** The tool this step runs, if the registry knows it. */
  tool?: FlowTool;
  index: number;
  /** The tool's option schema, or null while it loads or when it has none. */
  schema?: ComponentSchema | null;
  host?: SchemaFormHost;
  readOnly?: boolean;
  /** Start with the options area expanded (tests and stories). */
  defaultOpen?: boolean;
  /**
   * Drag and keyboard reordering. Absent shows no grip, as for a parallel
   * group's branches, which have no order.
   */
  reorder?: StepReorder;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onRemove?: () => void;
}

export function StepRow({
  step,
  tool,
  index,
  schema,
  host,
  readOnly,
  defaultOpen,
  reorder,
  onConfigChange,
  onRemove,
}: StepRowProps) {
  const [open, setOpen] = useState(defaultOpen ?? false);
  const name =
    step.label ||
    tool?.display_name ||
    tool?.name ||
    (step.parallel?.length ? t("Parallel group") : step.tool);
  // A read-only flow keeps its options closed; the recipe holds them.
  const hasOptions =
    !readOnly && !!schema && Object.keys(schema.properties ?? {}).length > 0;

  const toggle = () => setOpen((v) => !v);
  // Shown on hover or focus so a resting row carries only its grip and chevron.
  const onHover =
    "opacity-0 transition-opacity group-focus-within/step:opacity-100 group-hover/step:opacity-100";

  return (
    <li
      className={cn("group/step relative rounded-lg border", reorder?.dragging && "opacity-50")}
      data-testid="step-row"
      {...reorder?.row}
    >
      <DropIndicator side={reorder?.indicator} />
      <div className="flex items-start gap-2 p-2">
        {!readOnly && reorder && <ReorderHandle reorder={reorder} index={index} name={name} />}
        {hasOptions ? (
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={open ? t("Hide options") : t("Options")}
            aria-expanded={open}
            onClick={toggle}
          >
            {open ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
          </Button>
        ) : (
          <span className="size-6 shrink-0" aria-hidden="true" />
        )}
        {/* The name and description also open the options, as a larger target
            for the pointer; the chevron is the keyboard control. */}
        <div
          className={cn("min-w-0 flex-1", hasOptions && "cursor-pointer")}
          onClick={hasOptions ? toggle : undefined}
        >
          <div className="truncate text-sm font-medium text-foreground">{name}</div>
          {tool?.description && (
            <div className="line-clamp-1 text-[11px] text-muted-foreground">
              <Markdown inline>{tool.description}</Markdown>
            </div>
          )}
        </div>

        {/* The command line that runs this step's tool. */}
        {!step.parallel?.length && step.tool && (
          <ToolCommandCopy tool={step.tool} className={onHover} />
        )}
        {!readOnly && (
          <SimpleTooltip content={t("Remove step")}>
            <Button
              variant="ghost"
              size="icon-xs"
              className={onHover}
              aria-label={t("Remove {step}", { step: name })}
              onClick={onRemove}
            >
              <Trash2 className="size-3.5" />
            </Button>
          </SimpleTooltip>
        )}
      </div>

      {open && hasOptions && schema && (
        <div className="border-t p-3" data-testid="step-options">
          <SchemaForm
            schema={schema}
            values={step.config ?? {}}
            onChange={(config) => onConfigChange?.(config)}
            host={host}
            compact
            hideHeader
            readOnly={readOnly}
          />
        </div>
      )}
    </li>
  );
}
