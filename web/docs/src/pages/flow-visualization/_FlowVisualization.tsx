import React, { useState, useEffect, useCallback, useMemo } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import type { FlowTrace, TraceEvent, FlowNode, PartSnapshotSet } from "./_types";
import { usePlayback } from "./_usePlayback";
import FlowGraph from "./_FlowGraph";
import TimelineControls from "./_TimelineControls";
import PartInspector from "./_PartInspector";
import TraceSelector from "./_TraceSelector";
import styles from "./_index.module.css";

const EMPTY_EVENTS: TraceEvent[] = [];
const EMPTY_NODES: FlowNode[] = [];
const EMPTY_PARTS: Record<string, PartSnapshotSet> = {};

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

export default function FlowVisualization(): React.ReactElement {
  const [trace, setTrace] = useState<FlowTrace | null>(null);
  const [tracePath, setTracePath] = useState(AVAILABLE_TRACES[0].path);
  const [selectedPartId, setSelectedPartId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadedFileName, setLoadedFileName] = useState<string | null>(null);

  const tracePathResolved = useBaseUrl(tracePath);

  useEffect(() => {
    // Skip fetch when a file was loaded directly.
    if (loadedFileName) return;

    setTrace(null);
    setError(null);
    setSelectedPartId(null);
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
    setSelectedPartId(null);
    setTrace(data as FlowTrace);
  }, []);

  const playback = usePlayback({
    events: trace?.events ?? EMPTY_EVENTS,
    nodes: trace?.nodes ?? EMPTY_NODES,
    parts: trace?.parts ?? EMPTY_PARTS,
    channelSize: trace?.channelSize ?? 64,
  });

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
          <div className={styles.inspectorHint}>Loading trace data...</div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.wrapper}>
      {selector}
      <p className={styles.traceDescription}>{trace.description}</p>
      <div className={styles.visualizationArea}>
        <FlowGraph
          nodes={trace.nodes}
          channelSize={trace.channelSize}
          particles={playback.particles}
          channelFills={playback.channelFills}
          activeNodes={playback.activeNodes}
          selectedPartId={selectedPartId}
          onPartClick={setSelectedPartId}
        />
      </div>
      <TimelineControls
        state={playback.state}
        events={trace.events}
        onPlay={playback.play}
        onPause={playback.pause}
        onStep={playback.step}
        onSeek={playback.seek}
        onSetSpeed={playback.setSpeed}
        onReset={playback.reset}
      />
      <PartInspector partId={selectedPartId} parts={trace.parts} nodes={trace.nodes} />
    </div>
  );
}
