import React, { useState, useEffect, useCallback } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import { FlowTracePlayer } from "@neokapi/kapi-lab";
import type { FlowTrace } from "@neokapi/kapi-lab";
import TraceSelector from "./ConcurrencyTraceSelector";
import styles from "./ConcurrencyExplorer.module.css";

const AVAILABLE_TRACES = [
  {
    name: "Pseudo-translate JSON",
    description: "Basic native pipeline with 6 Parts",
    path: "/data/traces/pseudo-translate-json.json",
  },
  {
    name: "Multi-tool Pipeline",
    description: "Multiple tools, concurrency, buffering",
    path: "/data/traces/multi-tool-pipeline.json",
  },
  {
    name: "Bridge HTML",
    description: "Java/Okapi bridge with gRPC boundary",
    path: "/data/traces/bridge-html-pseudo.json",
  },
  {
    name: "AI Translate (Parallel)",
    description: "Parallel block processing with 3 concurrent workers",
    path: "/data/traces/ai-translate-parallel.json",
  },
  {
    name: "Translate + QA (Parallel)",
    description: "Two parallel stages: AI translate then QA check",
    path: "/data/traces/translate-qa-parallel.json",
  },
];

// ConcurrencyExplorer is the Lab's trace-replay surface: a trace picker
// (built-in traces + upload-your-own `kapi run --trace` output) feeding the
// step-driven <FlowTracePlayer> from @neokapi/kapi-lab. Unlike the Lab's live
// explorers, it replays recorded traces so it can show what live single-flow
// runs don't: parallel workers, channel buffering, and the Java bridge's gRPC
// boundary.
export function ConcurrencyExplorer(): React.ReactElement {
  const [trace, setTrace] = useState<FlowTrace | null>(null);
  const [tracePath, setTracePath] = useState(AVAILABLE_TRACES[0].path);
  const [error, setError] = useState<string | null>(null);
  const [loadedFileName, setLoadedFileName] = useState<string | null>(null);

  const tracePathResolved = useBaseUrl(tracePath);

  useEffect(() => {
    if (loadedFileName) return; // a file was loaded directly; skip fetch
    setTrace(null);
    setError(null);
    fetch(tracePathResolved)
      .then((r) => {
        if (!r.ok) throw new Error(`Failed to load trace: ${r.status}`);
        return r.json();
      })
      .then(setTrace)
      .catch((e) => setError(e.message));
  }, [tracePathResolved, loadedFileName]);

  const handleSelectBuiltin = useCallback((path: string) => {
    setLoadedFileName(null);
    setTracePath(path);
  }, []);

  const handleLoadFile = useCallback((data: unknown, fileName: string) => {
    setLoadedFileName(fileName);
    setError(null);
    setTrace(data as FlowTrace);
  }, []);

  const selector = (
    <TraceSelector
      traces={AVAILABLE_TRACES}
      selectedTrace={tracePath}
      onSelect={handleSelectBuiltin}
      onLoadFile={handleLoadFile}
      loadedFileName={loadedFileName}
    />
  );

  if (error) {
    return (
      <div className={styles.wrapper}>
        {selector}
        <div className={styles.inspector}>
          <div className={styles.inspectorHint}>Error: {error}</div>
        </div>
      </div>
    );
  }

  if (!trace) {
    return (
      <div className={styles.wrapper}>
        {selector}
        <div className={styles.inspector}>
          <div className={styles.inspectorHint}>Loading trace data…</div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.wrapper}>
      {selector}
      <FlowTracePlayer trace={trace} />
    </div>
  );
}
