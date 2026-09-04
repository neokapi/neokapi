// One step in the linear flow editor.
//
// A flow is an ordered pipeline, so a step is a row, not a node: the tool it
// runs, a one-line description, controls to move or remove it, and an
// expandable area that renders the step's key options through the tool's own
// schema form (the same form the tools surface uses). Rarely-used options stay
// in the recipe; the form shows the common ones.

import { useState } from "react";
import { ChevronDown, ChevronRight, ChevronUp, GripVertical, Trash2, Wrench } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button } from "../ui/button";
import { Markdown } from "../ui/markdown";
import { SimpleTooltip } from "../ui/tooltip";
import { SchemaForm } from "../schema-form";
import type { ComponentSchema, SchemaFormHost } from "../schema-form";
import type { FlowStep, FlowTool } from "./types";

export interface StepRowProps {
  step: FlowStep;
  /** The tool this step runs, if the registry knows it. */
  tool?: FlowTool;
  index: number;
  count: number;
  /** The tool's option schema, or null while it loads or when it has none. */
  schema?: ComponentSchema | null;
  host?: SchemaFormHost;
  readOnly?: boolean;
  /** Start with the options area expanded (tests and stories). */
  defaultOpen?: boolean;
  /** Hide the up/down controls — a parallel group's branches have no order. */
  hideMove?: boolean;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onRemove?: () => void;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
}

export function StepRow({
  step,
  tool,
  index,
  count,
  schema,
  host,
  readOnly,
  defaultOpen,
  hideMove,
  onConfigChange,
  onRemove,
  onMoveUp,
  onMoveDown,
}: StepRowProps) {
  const [open, setOpen] = useState(defaultOpen ?? false);
  const name =
    step.label ||
    tool?.display_name ||
    tool?.name ||
    (step.parallel?.length ? t("Parallel group") : step.tool);
  const hasOptions = !!schema && Object.keys(schema.properties ?? {}).length > 0;

  return (
    <li className="rounded-lg border" data-testid="step-row">
      <div className="flex items-start gap-2 p-2">
        <span className="mt-1 text-muted-foreground/50" aria-hidden="true">
          <GripVertical className="size-4" />
        </span>
        <Wrench className="mt-1 size-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium text-foreground">{name}</span>
          </div>
          {tool?.description && (
            <div className="line-clamp-1 text-[11px] text-muted-foreground">
              <Markdown inline>{tool.description}</Markdown>
            </div>
          )}
        </div>

        {!readOnly && (
          <div className="flex items-center gap-0.5">
            {hasOptions && (
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={open ? t("Hide options") : t("Options")}
                aria-expanded={open}
                onClick={() => setOpen((v) => !v)}
              >
                {open ? (
                  <ChevronDown className="size-3.5" />
                ) : (
                  <ChevronRight className="size-3.5" />
                )}
              </Button>
            )}
            {!hideMove && (
              <>
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
              </>
            )}
            <SimpleTooltip content={t("Remove step")}>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t("Remove {step}", { step: name })}
                onClick={onRemove}
              >
                <Trash2 className="size-3.5" />
              </Button>
            </SimpleTooltip>
          </div>
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
