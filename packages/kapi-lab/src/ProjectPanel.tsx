// ProjectPanel — the project lens of the flow workspace, as a right-side
// overlay with two tabs:
//
//   Defaults — a FORM over the project scope (defaults.tools): the per-tool
//     presets every flow in the project inherits and a step's own config
//     overrides per key. Editing here is editing the project, and the change
//     feeds the next run.
//   Recipe — the SOURCE: the live `.kapi` YAML the canvas serializes to, with
//     the project-scope block highlighted.
//
// The same panel opens from a step's config panel ("Edit project defaults"),
// so project-wide and flow-specific configuration sit one click apart instead
// of the recipe dangling under the canvas as an afterthought.

import React, { useMemo } from "react";
import { X } from "lucide-react";
import type { ComponentSchema } from "@neokapi/flow-editor";
import {
  Button,
  SchemaForm,
  ScrollArea,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from "@neokapi/ui-primitives";
import { CodeView } from "@neokapi/ui-primitives/preview";
import { projectScopeLines } from "./RecipeView";

export type ProjectPanelTab = "defaults" | "recipe";

export interface ProjectPanelProps {
  /** The serialized recipe (buildRecipe output). */
  recipe: string;
  /** Project-level tool presets (defaults.tools). */
  presets?: Record<string, Record<string, unknown>>;
  /** Commit edited presets (absent = read-only, e.g. trace replay). */
  onPresetsChange?: (presets: Record<string, Record<string, unknown>>) => void;
  /** Resolve a tool's config schema for the defaults form. */
  getSchema: (toolName: string) => ComponentSchema | null;
  /** Resolve a tool's display name. */
  getLabel: (toolName: string) => string;
  tab: ProjectPanelTab;
  onTabChange: (tab: ProjectPanelTab) => void;
  onClose: () => void;
}

export default function ProjectPanel({
  recipe,
  presets,
  onPresetsChange,
  getSchema,
  getLabel,
  tab,
  onTabChange,
  onClose,
}: ProjectPanelProps): React.ReactElement {
  const scopeLines = useMemo(() => projectScopeLines(recipe), [recipe]);
  const presetTools = Object.keys(presets ?? {});

  return (
    <div
      className="flex h-full flex-col overflow-hidden border-l border-border bg-background"
      style={{ width: "min(380px, calc(100vw - 2rem))" }}
    >
      <div className="flex items-center gap-2 border-b border-border px-3 py-2">
        <div className="flex-1 min-w-0">
          <div className="text-sm font-semibold text-foreground">Project</div>
          <div className="text-[11px] leading-snug text-muted-foreground">
            The canvas serializes to this <code>.kapi</code> recipe — the committed file kapi works
            from.
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={onClose}
          className="self-start"
          aria-label="Close project panel"
        >
          <X size={14} className="text-muted-foreground" />
        </Button>
      </div>

      <Tabs
        value={tab}
        onValueChange={(v) => onTabChange(v as ProjectPanelTab)}
        className="flex min-h-0 flex-1 flex-col"
      >
        <TabsList className="mx-3 mt-2">
          <TabsTrigger value="defaults" className="text-xs">
            Defaults
          </TabsTrigger>
          <TabsTrigger value="recipe" className="text-xs">
            Recipe
          </TabsTrigger>
        </TabsList>

        <TabsContent value="defaults" className="min-h-0 flex-1">
          <ScrollArea className="h-full">
            <div className="flex flex-col gap-3 px-3 py-2.5">
              <p className="text-[11px] leading-relaxed text-muted-foreground">
                Project scope: <code>defaults.tools</code> pins per-tool config every flow in the
                project inherits. A step&apos;s own config overrides it per key — the step&apos;s
                panel shows what it inherited.
              </p>
              {presetTools.length === 0 && (
                <p className="py-4 text-center text-[11px] italic text-muted-foreground">
                  This project pins no tool defaults yet.
                </p>
              )}
              {presetTools.map((tool) => {
                const schema = getSchema(tool);
                const values = presets?.[tool] ?? {};
                return (
                  <section key={tool} className="flex flex-col gap-1">
                    <h4 className="m-0 text-xs font-semibold text-foreground">
                      {getLabel(tool)}{" "}
                      <code className="font-mono text-[10px] font-normal text-muted-foreground">
                        defaults.tools.{tool}
                      </code>
                    </h4>
                    {schema ? (
                      <SchemaForm
                        schema={schema}
                        values={values}
                        onChange={(next: Record<string, unknown>) =>
                          onPresetsChange?.({ ...presets, [tool]: next })
                        }
                        compact
                        hideHeader
                        readOnly={!onPresetsChange}
                      />
                    ) : (
                      <pre className="m-0 overflow-auto rounded-md bg-secondary px-2 py-1.5 font-mono text-[10px] leading-relaxed text-foreground">
                        {JSON.stringify(values, null, 2)}
                      </pre>
                    )}
                  </section>
                );
              })}
            </div>
          </ScrollArea>
        </TabsContent>

        <TabsContent value="recipe" className="min-h-0 flex-1">
          <ScrollArea className="h-full">
            <div className="flex flex-col gap-1.5 px-3 py-2.5">
              <p className="m-0 text-[11px] leading-relaxed text-muted-foreground">
                Edit the flow and watch the YAML follow. Highlighted: <code>defaults</code> — the
                project scope.
              </p>
              <CodeView text={recipe} lang="yaml" changedLines={scopeLines} />
            </div>
          </ScrollArea>
        </TabsContent>
      </Tabs>
    </div>
  );
}
