// RecipeView — the project lens of the flow workspace. The canvas is not a
// drawing: it is a view over a `.kapi` recipe, the same committed file kapi
// uses on disk. This panel shows that recipe live (edit the flow and the YAML
// follows), with the project-scope block (defaults.tools) highlighted — the
// presets every flow in the project inherits and a step's own config
// overrides per key.

import React, { useMemo } from "react";
import { CodeView } from "@neokapi/ui-primitives/preview";

export interface RecipeViewProps {
  /** The serialized recipe (buildRecipe output). */
  recipe: string;
}

/** Zero-based lines of the `defaults:` block (the project scope) — from the
 *  `defaults:` line up to (not including) `flows:`. Exported for tests. */
export function projectScopeLines(recipe: string): Set<number> {
  const lines = recipe.split("\n");
  const out = new Set<number>();
  const start = lines.findIndex((l) => l === "defaults:");
  if (start < 0) return out;
  for (let i = start; i < lines.length; i++) {
    if (i > start && !lines[i].startsWith(" ")) break;
    out.add(i);
  }
  return out;
}

export default function RecipeView({ recipe }: RecipeViewProps): React.ReactElement {
  const scopeLines = useMemo(() => projectScopeLines(recipe), [recipe]);
  return (
    <div className="flex flex-col gap-1.5 rounded-lg border border-border bg-card p-3">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-[10px] font-bold uppercase tracking-[0.12em] text-muted-foreground">
          The recipe this canvas is
        </span>
        <span className="text-[10px] text-muted-foreground">
          highlighted: project scope (<code>defaults</code>)
        </span>
      </div>
      <p className="text-[11px] leading-relaxed text-muted-foreground">
        Everything you design here serializes to a <code>.kapi</code> recipe — the committed file
        kapi works from. <code>defaults.tools</code> is project scope (every flow inherits it; a
        step&apos;s own config wins per key); the flow&apos;s steps are the nodes on the canvas.
        Edit the flow and watch the YAML follow.
      </p>
      <CodeView text={recipe} lang="yaml" changedLines={scopeLines} maxHeight="320px" />
    </div>
  );
}
