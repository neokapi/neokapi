import { useState, useRef, useCallback, useEffect } from "react";
import type { PlaybackState, TraceEvent, Particle, FlowNode, PartSnapshotSet } from "./_types";

interface UsePlaybackOptions {
  events: TraceEvent[];
  nodes: FlowNode[];
  parts: Record<string, PartSnapshotSet>;
  channelSize: number;
}

interface UsePlaybackReturn {
  state: PlaybackState;
  particles: Particle[];
  channelFills: Record<string, number>;
  activeNodes: Set<string>;
  play: () => void;
  pause: () => void;
  step: () => void;
  seek: (time: number) => void;
  setSpeed: (speed: number) => void;
  reset: () => void;
}

function computeParticles(
  events: TraceEvent[],
  nodes: FlowNode[],
  parts: Record<string, PartSnapshotSet>,
  time: number,
): { particles: Particle[]; channelFills: Record<string, number>; activeNodes: Set<string> } {
  const nodeIndex = new Map<string, number>();
  nodes.forEach((n, i) => nodeIndex.set(n.id, i));

  // Track per-part state: last event at or before current time
  const partStates = new Map<
    string,
    { lastEvent: TraceEvent; prevExit?: TraceEvent; worker?: number }
  >();

  for (const evt of events) {
    if (evt.ts > time) break;
    if (!evt.partId) continue;

    const existing = partStates.get(evt.partId);
    const worker = (evt.meta?.worker as number | undefined) ?? existing?.worker;
    if (existing && existing.lastEvent.type === "exit") {
      partStates.set(evt.partId, { lastEvent: evt, prevExit: existing.lastEvent, worker });
    } else {
      partStates.set(evt.partId, { lastEvent: evt, prevExit: existing?.prevExit, worker });
    }
  }

  const particles: Particle[] = [];
  const activeNodes = new Set<string>();
  const channelFills: Record<string, number> = {};

  // Initialize channel fills
  for (let i = 0; i < nodes.length - 1; i++) {
    channelFills[`${nodes[i].id}->${nodes[i + 1].id}`] = 0;
  }

  for (const [partId, state] of partStates) {
    const { lastEvent, prevExit, worker } = state;
    const partInfo = parts[partId];
    const snapshot = partInfo?.initial;
    if (!snapshot) continue;

    const nIdx = nodeIndex.get(lastEvent.nodeId);
    if (nIdx === undefined) continue;

    if (lastEvent.type === "enter") {
      // Part is inside a node being processed
      activeNodes.add(lastEvent.nodeId);
      particles.push({
        partId,
        partType: snapshot.type,
        position: "node",
        nodeId: lastEvent.nodeId,
        summary: snapshot.summary,
        worker,
      });
    } else if (lastEvent.type === "exit") {
      // Part has exited a node - is it traveling to next node?
      const nextNodeIdx = nIdx + 1;
      if (nextNodeIdx < nodes.length) {
        // Find when the next node's enter event happens for this part
        let nextEnterTs: number | null = null;
        for (const evt of events) {
          if (
            evt.partId === partId &&
            evt.type === "enter" &&
            evt.nodeId === nodes[nextNodeIdx].id
          ) {
            nextEnterTs = evt.ts;
            break;
          }
        }

        if (nextEnterTs !== null && nextEnterTs <= time) {
          // Already entered next node - don't show on edge (will be handled by enter event)
          continue;
        }

        // Particle is on the edge
        const edgeKey = `${nodes[nIdx].id}->${nodes[nextNodeIdx].id}`;
        channelFills[edgeKey] = (channelFills[edgeKey] || 0) + 1;

        let progress = 0.5;
        if (nextEnterTs !== null) {
          const edgeDuration = nextEnterTs - lastEvent.ts;
          if (edgeDuration > 0) {
            progress = Math.min(1, (time - lastEvent.ts) / edgeDuration);
          }
        } else {
          // No next enter yet, slowly progress
          progress = Math.min(0.9, (time - lastEvent.ts) / 200);
        }

        particles.push({
          partId,
          partType: snapshot.type,
          position: "edge",
          edgeIndex: nIdx,
          progress,
          summary: snapshot.summary,
        });
      }
      // If last node and exited, part is done - don't show
    }
  }

  return { particles, channelFills, activeNodes };
}

export function usePlayback(options: UsePlaybackOptions): UsePlaybackReturn {
  const { events, nodes, parts, channelSize } = options;

  const duration = events.length > 0 ? events[events.length - 1].ts : 0;

  const [state, setState] = useState<PlaybackState>({
    playing: false,
    time: 0,
    speed: 0.1,
    duration,
    eventIndex: 0,
  });

  const stateRef = useRef(state);
  stateRef.current = state;

  const rafRef = useRef<number | null>(null);
  const lastFrameRef = useRef<number | null>(null);

  // Update duration when events change
  useEffect(() => {
    const newDuration = events.length > 0 ? events[events.length - 1].ts : 0;
    setState((s) => ({ ...s, duration: newDuration, time: 0, eventIndex: 0, playing: false }));
    lastFrameRef.current = null;
    if (rafRef.current) {
      cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
    }
  }, [events]);

  const tick = useCallback(
    (timestamp: number) => {
      if (lastFrameRef.current === null) {
        lastFrameRef.current = timestamp;
        rafRef.current = requestAnimationFrame(tick);
        return;
      }

      const deltaMs = timestamp - lastFrameRef.current;
      lastFrameRef.current = timestamp;

      // Convert real-time ms to trace-time µs, scaled by speed
      // Use a time scale: 1ms real = 2µs trace at 1x speed
      const deltaUs = deltaMs * 2 * stateRef.current.speed;

      setState((prev) => {
        const newTime = Math.min(prev.time + deltaUs, prev.duration);
        let newIndex = prev.eventIndex;
        while (newIndex < events.length && events[newIndex].ts <= newTime) {
          newIndex++;
        }

        if (newTime >= prev.duration) {
          lastFrameRef.current = null;
          return { ...prev, time: prev.duration, eventIndex: newIndex, playing: false };
        }

        return { ...prev, time: newTime, eventIndex: newIndex };
      });

      if (stateRef.current.playing) {
        rafRef.current = requestAnimationFrame(tick);
      }
    },
    [events],
  );

  const play = useCallback(() => {
    setState((prev) => {
      // If at the end, reset to beginning
      if (prev.time >= prev.duration) {
        lastFrameRef.current = null;
        return { ...prev, playing: true, time: 0, eventIndex: 0 };
      }
      return { ...prev, playing: true };
    });
    lastFrameRef.current = null;
    rafRef.current = requestAnimationFrame(tick);
  }, [tick]);

  const pause = useCallback(() => {
    setState((prev) => ({ ...prev, playing: false }));
    if (rafRef.current) {
      cancelAnimationFrame(rafRef.current);
      rafRef.current = null;
    }
    lastFrameRef.current = null;
  }, []);

  const step = useCallback(() => {
    pause();
    setState((prev) => {
      if (prev.eventIndex < events.length) {
        const nextTime = events[prev.eventIndex].ts;
        return { ...prev, time: nextTime, eventIndex: prev.eventIndex + 1 };
      }
      return prev;
    });
  }, [events, pause]);

  const seek = useCallback(
    (time: number) => {
      const clampedTime = Math.max(0, Math.min(time, duration));
      let newIndex = 0;
      while (newIndex < events.length && events[newIndex].ts <= clampedTime) {
        newIndex++;
      }
      setState((prev) => ({ ...prev, time: clampedTime, eventIndex: newIndex }));
    },
    [events, duration],
  );

  const setSpeed = useCallback((speed: number) => {
    setState((prev) => ({ ...prev, speed }));
  }, []);

  const reset = useCallback(() => {
    pause();
    setState((prev) => ({ ...prev, time: 0, eventIndex: 0 }));
  }, [pause]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current);
      }
    };
  }, []);

  const { particles, channelFills, activeNodes } = computeParticles(
    events,
    nodes,
    parts,
    state.time,
  );

  return {
    state: { ...state, duration },
    particles,
    channelFills,
    activeNodes,
    play,
    pause,
    step,
    seek,
    setSpeed,
    reset,
  };
}
