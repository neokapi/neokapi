import React, { useMemo, useState } from "react";
import { Input, Switch, cn } from "@neokapi/ui-primitives";
import ToolDropWidget from "./ToolDropWidget";
import type { LabRuntimeAssets } from "./useLabRuntime";

export interface SearchReplaceWidgetProps {
  assets: LabRuntimeAssets | null;
  /** Restrict the offered samples (default: all hero samples). */
  sampleIds?: string[];
  /** Sample selected on first render. */
  autoSampleId?: string;
  /** Initial find string. */
  defaultFind?: string;
  /** Initial replace string. */
  defaultReplace?: string;
  className?: string;
}

// Build a one-step `.kapi` recipe for the search-replace tool. The find/replace
// pair is carried in the tool config's `pairs` array; `regEx` toggles regular-
// expression mode. The pairs decode into SearchReplaceConfig.Pairs via the
// config's JSON round-trip (the field's `schema:"-"` only hides it from the
// generated schema form, not from JSON), so an inline recipe is the canonical
// way to drive search-replace with a pair from the browser. Exported for tests.
export function buildSearchReplaceRecipe(find: string, replace: string, regex: boolean): string {
  const pair = { search: find, replace, isRegex: regex };
  return [
    "version: v1",
    "name: Lab",
    "defaults:",
    "  source_language: en",
    "flows:",
    "  lab:",
    "    steps:",
    "      - tool: search-replace",
    "        config:",
    `          pairs: ${JSON.stringify([pair])}`,
    "          source: true",
    "          target: false",
    `          regEx: ${regex}`,
    "",
  ].join("\n");
}

// SearchReplaceWidget lets a learner type a find + replace (with an optional
// regex toggle) and runs the search-replace tool on a dropped file via an inline
// recipe, showing a before/after of the source text with a download. A thin
// wrapper over ToolDropWidget: the find/replace inputs feed the recipe builder.
export default function SearchReplaceWidget({
  assets,
  sampleIds,
  autoSampleId,
  defaultFind = "color",
  defaultReplace = "colour",
  className,
}: SearchReplaceWidgetProps): React.ReactElement {
  const [find, setFind] = useState(defaultFind);
  const [replace, setReplace] = useState(defaultReplace);
  const [regex, setRegex] = useState(false);

  // The recipe closes over the current find/replace/regex; ToolDropWidget's
  // debounced auto-run re-runs whenever this identity changes.
  const recipe = useMemo(
    () => () => buildSearchReplaceRecipe(find, replace, regex),
    [find, replace, regex],
  );

  return (
    <div className={cn("kapi-reference flex flex-col gap-3", className)}>
      <div className="flex flex-wrap items-end gap-3 rounded-lg border bg-card p-3 text-foreground">
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          Find
          <Input
            value={find}
            onChange={(e) => setFind(e.target.value)}
            placeholder="text or pattern"
            className="w-40"
          />
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          Replace
          <Input
            value={replace}
            onChange={(e) => setReplace(e.target.value)}
            placeholder="replacement"
            className="w-40"
          />
        </label>
        <label className="flex items-center gap-2 pb-2 text-sm text-foreground">
          <Switch checked={regex} onCheckedChange={setRegex} />
          Regex
        </label>
      </div>

      <ToolDropWidget
        assets={assets}
        tool="search-replace"
        recipe={recipe}
        // `run lab` requires a target language even though this is a source-only
        // transform; fr is a formality (no target is written).
        extraArgs={["--target-lang", "fr"]}
        render="diff"
        sampleIds={sampleIds}
        autoSampleId={autoSampleId}
      />
    </div>
  );
}
