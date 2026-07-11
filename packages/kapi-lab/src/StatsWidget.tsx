import React from "react";
import ToolDropWidget from "./ToolDropWidget";
import type { DropInput } from "./ToolDropWidget";
import type { LabRuntimeAssets } from "./useLabRuntime";

export interface StatsWidgetProps {
  assets: LabRuntimeAssets | null;
  /** Restrict the offered samples (default: all hero samples). */
  sampleIds?: string[];
  /** Sample selected on first render. */
  autoSampleId?: string;
  /** A file to load on first render instead of a sample. */
  initialInput?: DropInput | null;
  className?: string;
}

// StatsWidget runs `kapi stats <in> --json` on a dropped file and renders a
// compact metric card — translatable blocks, source words, and characters
// parsed from the captured JSON. Stats operates on the extracted content
// (not raw bytes), so boilerplate and markup are excluded.
// A thin wrapper over ToolDropWidget in "stat" render mode.
export default function StatsWidget({
  assets,
  sampleIds,
  autoSampleId,
  initialInput,
  className,
}: StatsWidgetProps): React.ReactElement {
  return (
    <ToolDropWidget
      assets={assets}
      tool="stats"
      // stats reports on stdout; the out path is unused but kept for the
      // shared argv shape. --json makes the output machine-parseable.
      buildArgv={(inPath) => ["stats", inPath, "--json"]}
      render="stat"
      sampleIds={sampleIds}
      autoSampleId={autoSampleId}
      initialInput={initialInput}
      className={className}
    />
  );
}
