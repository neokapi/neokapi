import React, { useEffect, useState } from "react";
import type { FlowTrace } from "@neokapi/ui-primitives/preview";
import { useFlowPlayback } from "./useFlowPlayback";
import FlowGraph from "./FlowGraph";
import StepControls from "./StepControls";
import PartInspector from "./PartInspector";
import PartDetailsModal from "./PartDetailsModal";
import styles from "./styles.module.css";

export interface FlowTracePlayerProps {
  trace: FlowTrace;
  /** Show the trace's prose description above the graph. Default true. */
  showDescription?: boolean;
}

// FlowTracePlayer renders a FlowTrace as an interactive, step-driven
// visualization: a flow graph with Parts in flight, step/play controls, a
// "what just happened" narration, and a Part inspector that follows the action.
// Pure presentation — the host supplies the trace (canned or live from WASM).
export default function FlowTracePlayer({
  trace,
  showDescription = true,
}: FlowTracePlayerProps): React.ReactElement {
  const [selectedPartId, setSelectedPartId] = useState<string | null>(null);
  // Tracks whether the user picked a part by hand; if so, stop auto-following.
  const [manualSelect, setManualSelect] = useState(false);
  const [detailsOpen, setDetailsOpen] = useState(false);

  const playback = useFlowPlayback({
    events: trace.events,
    nodes: trace.nodes,
    parts: trace.parts,
  });

  const { delta } = playback;

  // Follow the action: when a step touches a part and the user hasn't pinned a
  // selection, point the inspector at the first part that just moved.
  useEffect(() => {
    if (manualSelect) return;
    if (delta.affectedPartIds.length > 0) setSelectedPartId(delta.affectedPartIds[0]);
  }, [delta, manualSelect]);

  // Reset selection bookkeeping when the trace changes.
  useEffect(() => {
    setSelectedPartId(null);
    setManualSelect(false);
    setDetailsOpen(false);
  }, [trace]);

  const handlePartClick = (id: string) => {
    setManualSelect(true);
    setSelectedPartId(id);
  };

  // The nodes touched at the current step, for inspector highlighting.
  const activeNodeIds = Array.from(new Set(delta.events.map((e) => e.nodeId)));

  return (
    <div className={styles.wrapper}>
      {showDescription && trace.description && (
        <p className={styles.traceDescription}>{trace.description}</p>
      )}

      <div className={styles.visualizationArea}>
        <FlowGraph
          nodes={trace.nodes}
          channelSize={trace.channelSize}
          particles={playback.particles}
          channelFills={playback.channelFills}
          activeNodes={playback.activeNodes}
          selectedPartId={selectedPartId}
          onPartClick={handlePartClick}
        />
      </div>

      <StepControls
        state={playback.state}
        onStepPrev={playback.stepPrev}
        onStepNext={playback.stepNext}
        onPlay={playback.play}
        onPause={playback.pause}
        onReset={playback.reset}
        onSeek={playback.stepTo}
        onSetSpeed={playback.setSpeed}
      />

      <div className={styles.narration}>
        <span className={styles.narrationStep}>
          step {playback.state.frameIndex}/{playback.state.frameCount - 1}
        </span>
        <span>{delta.summary}</span>
      </div>

      <PartInspector
        partId={selectedPartId}
        parts={trace.parts}
        nodes={trace.nodes}
        activeNodeIds={activeNodeIds}
        onOpenDetails={() => setDetailsOpen(true)}
      />

      <PartDetailsModal
        set={detailsOpen && selectedPartId ? trace.parts[selectedPartId] : null}
        nodes={trace.nodes}
        onClose={() => setDetailsOpen(false)}
      />
    </div>
  );
}
