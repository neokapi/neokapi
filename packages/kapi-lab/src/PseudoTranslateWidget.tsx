import React from "react";
import ToolDropWidget from "./ToolDropWidget";
import type { LabRuntimeAssets } from "./useLabRuntime";

export interface PseudoTranslateWidgetProps {
  assets: LabRuntimeAssets | null;
  /** Restrict the offered samples (default: all hero samples). */
  sampleIds?: string[];
  /** Sample selected on first render. */
  autoSampleId?: string;
  className?: string;
}

// PseudoTranslateWidget runs `kapi pseudo-translate <in> -o <out>` on a dropped
// file and shows the accented preview (Blocks/Structure/Native) with a download.
// Pseudo-translation is offline and needs no API key — the most legible "drop a
// file → see what kapi does" demo. A thin wrapper over ToolDropWidget.
export default function PseudoTranslateWidget({
  assets,
  sampleIds,
  autoSampleId,
  className,
}: PseudoTranslateWidgetProps): React.ReactElement {
  return (
    <ToolDropWidget
      assets={assets}
      tool="pseudo-translate"
      buildArgv={(inPath, outPath) => ["pseudo-translate", inPath, "-o", outPath]}
      sampleIds={sampleIds}
      autoSampleId={autoSampleId}
      render="output"
      className={className}
    />
  );
}
