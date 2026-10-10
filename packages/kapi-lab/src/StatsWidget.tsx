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

// StatsWidget runs `kapi stats <in>` on a dropped file and shows what it
// prints: translatable blocks, source words and characters, counted off the
// extracted content (not raw bytes), so boilerplate and markup are excluded.
// A thin wrapper over ToolDropWidget in "text" render mode: the command and
// its output are the whole result.
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
      // shared argv shape.
      buildArgv={(inPath) => ["stats", inPath]}
      render="text"
      sampleIds={sampleIds}
      autoSampleId={autoSampleId}
      initialInput={initialInput}
      className={className}
    />
  );
}
