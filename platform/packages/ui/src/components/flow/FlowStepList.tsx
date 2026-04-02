import * as React from "react";
import {
  ChevronDownIcon,
  ChevronRightIcon,
  GripVerticalIcon,
} from "lucide-react";

import { cn } from "@neokapi/ui-primitives";
import { Badge } from "@neokapi/ui-primitives/components/ui/badge";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { SchemaForm, type ComponentSchema, type PropertySchema } from "@neokapi/ui-primitives";
import type { ResourceOption } from "../ResourcePicker";

export interface FlowStepInfo {
  tool: string;
  label?: string;
  config: Record<string, unknown>;
}

export interface FlowStepListProps {
  /** Steps in the flow. */
  steps: FlowStepInfo[];
  /** Schemas for each tool, keyed by tool name. */
  schemas: Map<string, ComponentSchema>;
  /** Called when a step's config changes. */
  onStepConfigChange: (index: number, config: Record<string, unknown>) => void;
  /** Available named resources for ResourcePicker dropdowns. */
  resources?: Record<string, ResourceOption[]>;
  /** Additional class name. */
  className?: string;
}

/** Extract resource references (tm:name, termbase:name) from a config map. */
function extractResourceBadges(config: Record<string, unknown>): string[] {
  const badges: string[] = [];
  for (const value of Object.values(config)) {
    if (typeof value !== "string") continue;
    if (
      value.startsWith("tm:") ||
      value.startsWith("termbase:") ||
      value.startsWith("srx:")
    ) {
      badges.push(value);
    }
  }
  return badges;
}

const categoryIcons: Record<string, string> = {
  translate: "T",
  validate: "Q",
  enrich: "E",
  convert: "C",
  transform: "X",
  pipeline: "P",
};

export function FlowStepList({
  steps,
  schemas,
  onStepConfigChange,
  resources = {},
  className,
}: FlowStepListProps) {
  const [expandedIndex, setExpandedIndex] = React.useState<number | null>(null);

  if (steps.length === 0) {
    return (
      <div
        className={cn(
          "flex flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-12 text-sm text-muted-foreground",
          className,
        )}
      >
        <p>No steps in this flow.</p>
        <p className="text-xs">Add a step to get started.</p>
      </div>
    );
  }

  return (
    <div className={cn("flex flex-col gap-1", className)}>
      {steps.map((step, index) => {
        const schema = schemas.get(step.tool);
        const isExpanded = expandedIndex === index;
        const badges = extractResourceBadges(step.config);
        const category =
          schema?.description?.toLowerCase().includes("quality") ? "validate" :
          schema?.description?.toLowerCase().includes("translat") ? "translate" :
          "pipeline";
        const icon = categoryIcons[category] || "S";

        return (
          <div
            key={index}
            className="rounded-lg border bg-card transition-colors"
          >
            <button
              type="button"
              className="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-accent/30"
              onClick={() =>
                setExpandedIndex(isExpanded ? null : index)
              }
            >
              <GripVerticalIcon className="size-4 shrink-0 cursor-grab text-muted-foreground/40" />
              <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-xs font-bold text-muted-foreground">
                {icon}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">
                    {step.label || step.tool}
                  </span>
                  {badges.map((badge) => (
                    <Badge
                      key={badge}
                      variant="secondary"
                      className="text-xs font-normal"
                    >
                      {badge}
                    </Badge>
                  ))}
                </div>
              </div>
              {isExpanded ? (
                <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground" />
              ) : (
                <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
              )}
            </button>

            {isExpanded && schema && (
              <div className="border-t px-3 py-4">
                <SchemaForm
                  schema={schema}
                  values={step.config}
                  onChange={(config) => onStepConfigChange(index, config)}
                  resources={resources}
                />
              </div>
            )}

            {isExpanded && !schema && (
              <div className="border-t px-3 py-4 text-sm text-muted-foreground">
                No schema available for this step.
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
