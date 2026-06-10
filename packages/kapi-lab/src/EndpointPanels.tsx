// Endpoint inspector bodies for the lab flow workspace (rendered inside the
// flow editor's EndpointInspectorPanel chrome via renderEndpointPanel).
//
// Source — the anatomy lesson, in place: the reader turns the bound input's
// bytes into the content model (Layers → Groups → Blocks of Runs), and this
// panel shows that tree for the file currently feeding the flow. It is what
// the first tool receives.
//
// Sink — the round-trip lesson, in place: what the writer spliced back into
// the document skeleton, shown in the shared output viewer with the Native
// bytes diffed line-by-line against the INPUT — only block text changes,
// structure is reproduced exactly.

import React, { useEffect, useState } from "react";
import { FileInput } from "lucide-react";
import { ContentTreeView } from "@neokapi/ui-primitives/preview";
import type { ContentTree } from "@neokapi/ui-primitives/preview";
import type { LabRuntime } from "./useLabRuntime";
import type { FileSourceValue } from "./FileSource";
import OutputView from "./OutputView";

export interface SourceContentPanelProps {
  runtime: LabRuntime;
  file: FileSourceValue;
}

export function SourceContentPanel({ runtime, file }: SourceContentPanelProps): React.ReactElement {
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!runtime.ready) return;
    let cancelled = false;
    setBusy(true);
    setError(null);
    void runtime
      .inspect(file.filename, file.bytes ?? file.content)
      .then((res) => {
        if (cancelled) return;
        if (res.ok && res.tree) {
          setTree(res.tree);
        } else {
          setError(res.error ?? "could not read the file");
          setTree(null);
        }
      })
      .finally(() => !cancelled && setBusy(false));
    return () => {
      cancelled = true;
    };
  }, [runtime.ready, runtime.inspect, file]);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
        <FileInput size={12} />
        <code className="text-[10px]">{file.filename}</code>
      </div>
      <p className="text-[11px] leading-relaxed text-muted-foreground">
        The reader turns these bytes into the content model — Layers and Groups containing Blocks
        whose text is a sequence of Runs. This tree is exactly what flows into the first tool.
      </p>
      {!runtime.ready && (
        <div className="py-4 text-center text-[11px] italic text-muted-foreground">
          Waiting for the engine…
        </div>
      )}
      {busy && (
        <div className="py-4 text-center text-[11px] italic text-muted-foreground">Reading…</div>
      )}
      {error && <div className="text-[11px] text-destructive">{error}</div>}
      {tree && !busy && <ContentTreeView tree={tree} />}
    </div>
  );
}

export interface SinkOutputPanelProps {
  runtime: LabRuntime;
  /** Path the last run wrote, or null before any run. */
  outPath: string | null;
  /** Bumped per run so the viewer re-reads the bytes. */
  version: number;
  /** The input file's text, diffed against the output's Native bytes. */
  baseline: string | null;
}

export function SinkOutputPanel({
  runtime,
  outPath,
  version,
  baseline,
}: SinkOutputPanelProps): React.ReactElement {
  if (!outPath) {
    return (
      <div className="py-4 text-center text-[11px] italic text-muted-foreground">
        Nothing written yet — press Run and the output the writer produces lands here.
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-2">
      <p className="text-[11px] leading-relaxed text-muted-foreground">
        The writer splices the processed blocks back into the document&apos;s structural skeleton.
        In the <strong>Native</strong> tab, lines that differ from the input are highlighted — only
        the block text changes; the structure round-trips exactly.
      </p>
      <OutputView
        runtime={runtime}
        path={outPath}
        version={version}
        baseline={baseline}
        defaultTab="raw"
      />
    </div>
  );
}
