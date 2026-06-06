import React from "react";
import ToolDropWidget from "./ToolDropWidget";
import type { LabRuntimeAssets } from "./useLabRuntime";

export interface StatsWidgetProps {
  assets: LabRuntimeAssets | null;
  /** Restrict the offered samples (default: all hero samples). */
  sampleIds?: string[];
  /** Sample selected on first render. */
  autoSampleId?: string;
  className?: string;
}

// StatsWidget runs `kapi word-count <in> --json` on a dropped file and renders a
// compact metric card — translatable blocks, source words, and an estimated
// character total parsed from the captured JSON. Word-count operates on the
// extracted content (not raw bytes), so boilerplate and markup are excluded.
// A thin wrapper over ToolDropWidget in "stat" render mode.
export default function StatsWidget({
  assets,
  sampleIds,
  autoSampleId,
  className,
}: StatsWidgetProps): React.ReactElement {
  return (
    <ToolDropWidget
      assets={assets}
      tool="word-count"
      // word-count reports on stdout; the out path is unused but kept for the
      // shared argv shape. --json makes the output machine-parseable.
      buildArgv={(inPath) => ["word-count", inPath, "--json"]}
      render="stat"
      sampleIds={sampleIds}
      autoSampleId={autoSampleId}
      className={className}
    />
  );
}
