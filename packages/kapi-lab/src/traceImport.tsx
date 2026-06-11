// Trace import for the flow workspace: replay a RECORDED `kapi run --trace`
// (native parallel workers, the Java bridge's gRPC boundary — things a live
// wasm run can't show) on the same canvas and transport as a live run. The
// imported trace's tool nodes reconstruct a read-only FlowSpec; the editor's
// run review then plays the recorded events back on those nodes.

import React, { useRef, useState } from "react";
import { ChevronDown, FileUp, PlayCircle } from "lucide-react";
import type { FlowSpec, FlowTrace } from "@neokapi/flow-editor";
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@neokapi/ui-primitives";

/** A built-in recorded trace offered by the host (URLs already base-resolved). */
export interface RecordedTraceInfo {
  name: string;
  description?: string;
  url: string;
}

/** Reconstruct the flow a recorded trace ran: its tool nodes, in order. */
export function specFromTrace(trace: FlowTrace): FlowSpec {
  const steps = (trace.nodes ?? [])
    .filter((n) => n.type === "tool" || n.type === "bridge-tool")
    // Older traces may omit `name`; fall back to the node id so the step
    // still renders rather than producing a ghost node.
    .map((n) => ({ tool: n.name || n.id, ...(n.label ? { label: n.label } : {}) }))
    .filter((s) => s.tool);
  return { steps };
}

export interface TraceImportControlProps {
  /** Built-in recorded traces (the host resolves their URLs). */
  traces?: RecordedTraceInfo[];
  onLoad: (trace: FlowTrace, label: string) => void;
  onError: (message: string) => void;
}

export function TraceImportControl({
  traces,
  onLoad,
  onError,
}: TraceImportControlProps): React.ReactElement {
  const fileRef = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);

  const loadBuiltin = async (url: string, label: string) => {
    setBusy(true);
    try {
      const resp = await fetch(url);
      if (!resp.ok) throw new Error(`failed to load trace (${resp.status})`);
      onLoad((await resp.json()) as FlowTrace, label);
    } catch (err) {
      onError(String(err));
    } finally {
      setBusy(false);
    }
  };

  const loadFile = (file: File) => {
    const reader = new FileReader();
    reader.onload = () => {
      try {
        // readAsText guarantees a string result.
        onLoad(JSON.parse(reader.result as string) as FlowTrace, file.name);
      } catch {
        onError(`${file.name} is not a kapi trace JSON`);
      }
    };
    reader.readAsText(file);
  };

  return (
    <div className="flex items-center gap-2">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" disabled={busy} className="gap-1.5 text-xs">
            <PlayCircle className="size-3.5 text-muted-foreground" />
            Replay a run
            <ChevronDown className="size-3 text-muted-foreground" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-72">
          <DropdownMenuLabel className="text-[10px] uppercase tracking-wide text-muted-foreground">
            Recorded native runs — things a live wasm run can&apos;t show
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          {(traces ?? []).map((t) => (
            <DropdownMenuItem
              key={t.url}
              className="items-start py-1.5"
              onSelect={() => void loadBuiltin(t.url, t.name)}
            >
              <span className="flex flex-col gap-0.5">
                <span className="text-xs font-medium">{t.name}</span>
                {t.description && (
                  <span className="text-[10px] leading-tight text-muted-foreground">
                    {t.description}
                  </span>
                )}
              </span>
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            className="gap-1.5 text-xs"
            onSelect={() => fileRef.current?.click()}
            title="Load your own `kapi run --trace` output"
          >
            <FileUp className="size-3.5 text-muted-foreground" />
            Upload a trace file…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <input
        ref={fileRef}
        type="file"
        accept=".json,application/json"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) loadFile(f);
          e.target.value = "";
        }}
      />
    </div>
  );
}
