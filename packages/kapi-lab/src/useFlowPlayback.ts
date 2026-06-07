import { useCallback, useEffect, useMemo, useState } from "react";
import type {
  FlowNode,
  Particle,
  PartSnapshotSet,
  TraceEvent,
} from "@neokapi/ui-primitives/preview";

// useFlowPlayback drives a FlowTrace as a sequence of discrete frames so a
// learner can step through it one transition at a time ("Next"), as well as
// play it back automatically.
//
// A *frame* is one distinct event timestamp. Frame 0 is a synthetic "empty
// pipeline" state (time 0, nothing in flight). Stepping to frame k applies
// every event up to and including frames[k]; the events that land exactly at
// frames[k] are the frame's *delta* — the "what just happened" the UI narrates.
// Snapping playback to event boundaries (rather than a continuous clock) is
// what makes the pipeline legible: each Next is one observable change.

export type PlaybackMode = "step" | "play";

export interface FlowPlaybackState {
  mode: PlaybackMode;
  playing: boolean;
  frameIndex: number;
  frameCount: number;
  time: number;
  duration: number;
  /** Playback rate in frames per second (play mode). */
  speed: number;
  atStart: boolean;
  atEnd: boolean;
}

export interface FrameDelta {
  time: number;
  events: TraceEvent[];
  /** Human-readable "what just happened" for this frame. */
  summary: string;
  /** Distinct part ids touched by this frame's events. */
  affectedPartIds: string[];
}

export interface FlowPlaybackReturn {
  state: FlowPlaybackState;
  particles: Particle[];
  channelFills: Record<string, number>;
  activeNodes: Set<string>;
  delta: FrameDelta;
  stepNext: () => void;
  stepPrev: () => void;
  stepTo: (index: number) => void;
  play: () => void;
  pause: () => void;
  setSpeed: (fps: number) => void;
  reset: () => void;
}

interface Options {
  events: TraceEvent[];
  nodes: FlowNode[];
  parts: Record<string, PartSnapshotSet>;
  /** Initial frames per second for play mode. Default 3. */
  initialSpeed?: number;
}

// computeParticles resolves the visual state at a given trace time: which parts
// sit inside a node, which travel on an edge, channel fill levels, and which
// nodes are active. Requires `events` sorted ascending by ts.
function computeParticles(
  events: TraceEvent[],
  nodes: FlowNode[],
  parts: Record<string, PartSnapshotSet>,
  time: number,
): { particles: Particle[]; channelFills: Record<string, number>; activeNodes: Set<string> } {
  const nodeIndex = new Map<string, number>();
  nodes.forEach((n, i) => nodeIndex.set(n.id, i));

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
  for (let i = 0; i < nodes.length - 1; i++) {
    channelFills[`${nodes[i].id}->${nodes[i + 1].id}`] = 0;
  }

  for (const [partId, state] of partStates) {
    const { lastEvent, worker } = state;
    const snapshot = parts[partId]?.initial;
    if (!snapshot) continue;
    const nIdx = nodeIndex.get(lastEvent.nodeId);
    if (nIdx === undefined) continue;

    if (lastEvent.type === "enter") {
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
      const nextNodeIdx = nIdx + 1;
      if (nextNodeIdx < nodes.length) {
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
        if (nextEnterTs !== null && nextEnterTs <= time) continue;

        const edgeKey = `${nodes[nIdx].id}->${nodes[nextNodeIdx].id}`;
        channelFills[edgeKey] = (channelFills[edgeKey] || 0) + 1;

        let progress = 0.5;
        if (nextEnterTs !== null) {
          const edgeDuration = nextEnterTs - lastEvent.ts;
          if (edgeDuration > 0) progress = Math.min(1, (time - lastEvent.ts) / edgeDuration);
        } else {
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
    }
  }

  return { particles, channelFills, activeNodes };
}

// describeDelta turns the events at a frame into a single learner-facing line.
function describeDelta(
  events: TraceEvent[],
  nodes: FlowNode[],
  parts: Record<string, PartSnapshotSet>,
): string {
  if (events.length === 0)
    return "Ready — nothing in the pipeline yet. Press Next to read the first part.";
  const labelOf = (id: string) => nodes.find((n) => n.id === id)?.label ?? id;

  // Group by (type, nodeId), collecting distinct parts.
  const groups = new Map<string, { type: string; node: string; parts: Set<string> }>();
  for (const e of events) {
    const key = `${e.type}:${e.nodeId}`;
    let g = groups.get(key);
    if (!g) {
      g = { type: e.type, node: e.nodeId, parts: new Set() };
      groups.set(key, g);
    }
    if (e.partId) g.parts.add(e.partId);
  }

  const verb = (t: string) => (t === "enter" ? "entered" : t === "exit" ? "left" : t);
  const phrases: string[] = [];
  for (const g of groups.values()) {
    const n = g.parts.size || 1;
    // When a single part leaves a node and gained a target, call it out.
    if (n === 1 && g.type === "exit") {
      const pid = [...g.parts][0];
      const after = parts[pid]?.afterNode?.[g.node];
      if (after?.targetText) {
        phrases.push(
          `${after.id || "a part"} left ${labelOf(g.node)} → "${truncate(after.targetText, 32)}"`,
        );
        continue;
      }
    }
    phrases.push(`${n} ${n === 1 ? "part" : "parts"} ${verb(g.type)} ${labelOf(g.node)}`);
  }
  return phrases.join("; ");
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n) + "…" : s;
}

export function useFlowPlayback(options: Options): FlowPlaybackReturn {
  const { events, nodes, parts, initialSpeed = 3 } = options;

  // frames[0] = 0 (empty pipeline); then each distinct event timestamp.
  const frames = useMemo(() => {
    const uniq = Array.from(new Set(events.map((e) => e.ts))).sort((a, b) => a - b);
    return uniq[0] === 0 ? uniq : [0, ...uniq];
  }, [events]);

  const duration = frames.length > 0 ? frames[frames.length - 1] : 0;

  const [mode, setMode] = useState<PlaybackMode>("step");
  const [playing, setPlaying] = useState(false);
  const [frameIndex, setFrameIndex] = useState(0);
  const [speed, setSpeedState] = useState(initialSpeed);

  // Reset whenever the trace (its frames) changes.
  useEffect(() => {
    setFrameIndex(0);
    setPlaying(false);
    setMode("step");
  }, [frames]);

  const clampIndex = useCallback(
    (i: number) => Math.max(0, Math.min(i, frames.length - 1)),
    [frames.length],
  );

  const stepTo = useCallback(
    (i: number) => {
      setPlaying(false);
      setMode("step");
      setFrameIndex(clampIndex(i));
    },
    [clampIndex],
  );
  const stepNext = useCallback(() => {
    setMode("step");
    setFrameIndex((i) => clampIndex(i + 1));
  }, [clampIndex]);
  const stepPrev = useCallback(() => {
    setPlaying(false);
    setMode("step");
    setFrameIndex((i) => clampIndex(i - 1));
  }, [clampIndex]);
  const reset = useCallback(() => {
    setPlaying(false);
    setMode("step");
    setFrameIndex(0);
  }, []);
  const setSpeed = useCallback((fps: number) => setSpeedState(fps), []);

  const play = useCallback(() => {
    setMode("play");
    setPlaying(true);
    // Restart from the top if we're already at the end.
    setFrameIndex((i) => (i >= frames.length - 1 ? 0 : i));
  }, [frames.length]);
  const pause = useCallback(() => setPlaying(false), []);

  // Auto-advance in play mode.
  useEffect(() => {
    if (!playing) return;
    if (frameIndex >= frames.length - 1) {
      setPlaying(false);
      return;
    }
    const id = setTimeout(
      () => setFrameIndex((i) => clampIndex(i + 1)),
      Math.max(80, 1000 / speed),
    );
    return () => clearTimeout(id);
  }, [playing, frameIndex, frames.length, speed, clampIndex]);

  const time = frames[frameIndex] ?? 0;

  const { particles, channelFills, activeNodes } = useMemo(
    () => computeParticles(events, nodes, parts, time),
    [events, nodes, parts, time],
  );

  const delta = useMemo<FrameDelta>(() => {
    const frameEvents = frameIndex === 0 ? [] : events.filter((e) => e.ts === time);
    return {
      time,
      events: frameEvents,
      summary: describeDelta(frameEvents, nodes, parts),
      affectedPartIds: Array.from(
        new Set(frameEvents.map((e) => e.partId).filter(Boolean) as string[]),
      ),
    };
  }, [events, nodes, parts, time, frameIndex]);

  return {
    state: {
      mode,
      playing,
      frameIndex,
      frameCount: frames.length,
      time,
      duration,
      speed,
      atStart: frameIndex <= 0,
      atEnd: frameIndex >= frames.length - 1,
    },
    particles,
    channelFills,
    activeNodes,
    delta,
    stepNext,
    stepPrev,
    stepTo,
    play,
    pause,
    setSpeed,
    reset,
  };
}
