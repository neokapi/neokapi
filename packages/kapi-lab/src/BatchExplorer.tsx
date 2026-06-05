import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Play } from "lucide-react";
import { Button, ToggleGroup, ToggleGroupItem } from "@neokapi/ui-primitives";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import { useFileLibrary, resolveSelection } from "./fileLibrary";
import type { FileSelection } from "./fileLibrary";
import FileSelectorField from "./FileSelectorField";
import OutputView from "./OutputView";

// The batch tools a learner can run across a selection. Each writes one output
// file per input; the explorer shows them side by side with change highlighting.
interface BatchTool {
  id: string;
  label: string;
  build: (inPath: string, outPath: string) => string[];
}

const TOOLS: BatchTool[] = [
  {
    id: "pseudo-translate",
    label: "Pseudo-translate",
    build: (i, o) => ["pseudo-translate", i, "-o", o],
  },
  {
    id: "ai-translate-qa",
    label: "AI translate + QA (demo)",
    build: (i, o) => ["run", "ai-translate-qa", "-i", i, "-o", o, "--target-lang", "fr"],
  },
];

export interface BatchExplorerProps {
  assets: LabRuntimeAssets | null;
  sampleIds?: string[];
  /** Initial glob pattern (default "*.json"). */
  defaultPattern?: string;
}

// BatchExplorer shows the file selector at full strength: pick many files or a
// glob, run a tool across the whole selection in the browser, and inspect each
// output (Blocks / Structure / Native) with the bytes the engine wrote — every
// changed file and line highlighted. It is the canonical demo of selecting one,
// many, or a glob of files and seeing the outputs that result.
export default function BatchExplorer({
  assets,
  sampleIds,
  defaultPattern = "*.json",
}: BatchExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);
  const library = useFileLibrary({ sampleIds });
  const [selection, setSelection] = useState<FileSelection>({
    mode: "glob",
    paths: [],
    pattern: defaultPattern,
  });
  const [toolId, setToolId] = useState(TOOLS[0].id);
  const [outputs, setOutputs] = useState<string[]>([]);
  const [version, setVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const selected = useMemo(() => resolveSelection(selection, library), [selection, library]);

  const run = useCallback(async () => {
    if (!runtime.ready || selected.length === 0) return;
    const tool = TOOLS.find((t) => t.id === toolId) ?? TOOLS[0];
    setBusy(true);
    setError(null);
    const produced: string[] = [];
    for (const f of selected) {
      const inPath = runtime.writeFile(f.path, f.bytes);
      const outPath = `out/${f.name}`;
      const absOut = runtime.writeFile(outPath, new Uint8Array(0));
      const code = await runtime.run(tool.build(inPath, absOut));
      const bytes = runtime.readBytes(absOut);
      if (code === 0 && bytes && bytes.length > 0) {
        library.setOutput(outPath, bytes);
        produced.push(absOut);
      }
    }
    if (produced.length === 0) setError("no outputs were produced");
    setOutputs(produced);
    setVersion((v) => v + 1);
    setBusy(false);
  }, [runtime.ready, runtime.writeFile, runtime.run, runtime.readBytes, library, selected, toolId]);

  // Auto-run once ready so the explorer shows results immediately.
  useEffect(() => {
    if (runtime.ready && outputs.length === 0) void run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runtime.ready]);

  return (
    <div className="kapi-reference flex flex-col gap-3 text-foreground">
      <div className="flex flex-wrap items-end gap-3">
        <FileSelectorField
          label="Inputs"
          library={library}
          selection={selection}
          onSelectionChange={setSelection}
          multiple
          sampleIds={sampleIds}
        />
        <ToggleGroup
          type="single"
          variant="outline"
          value={toolId}
          onValueChange={(v) => v && setToolId(v)}
        >
          {TOOLS.map((t) => (
            <ToggleGroupItem key={t.id} value={t.id} className="px-3 text-xs">
              {t.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Button
          onClick={() => void run()}
          disabled={!runtime.ready || busy || selected.length === 0}
        >
          <Play /> Run on {selected.length} file{selected.length === 1 ? "" : "s"}
        </Button>
      </div>

      <div className="min-h-[1.4rem] text-sm text-muted-foreground">
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Running across the selection…"}
        {runtime.ready && !busy && error && <span className="text-destructive">{error}</span>}
      </div>

      {outputs.length > 0 && (
        <div className="flex flex-col gap-3">
          {outputs.map((p) => (
            <OutputView key={p} runtime={runtime} path={p} version={version} />
          ))}
        </div>
      )}
    </div>
  );
}
