import { useEffect, useRef, useState } from "react";
import type { GraphModel, Lens } from "../lens/index.ts";
import type { LabSession } from "./useLabSession.ts";

// A lab's lens, rebuilt from the engine's answers after every command
// settles. The previous model travels with the new one, so the view can mark
// what the last command changed. Both players draw a lens through this.

export interface LensModel {
  graph: GraphModel | null;
  previous: GraphModel | null;
  /** A rebuild is under way. */
  loading: boolean;
}

export function useLens(lens: Lens | undefined, session: LabSession, ready: boolean): LensModel {
  const [graph, setGraph] = useState<GraphModel | null>(null);
  const [loading, setLoading] = useState(false);
  const graphRef = useRef<GraphModel | null>(null);
  const previousRef = useRef<GraphModel | null>(null);
  const { runCapture, version, busy } = session;
  useEffect(() => {
    if (!lens || !ready || busy) return;
    let cancelled = false;
    const h = setTimeout(() => {
      setLoading(true);
      void lens
        .build(runCapture)
        .then((m) => {
          if (cancelled) return;
          previousRef.current = graphRef.current;
          graphRef.current = m;
          setGraph(m);
        })
        .finally(() => !cancelled && setLoading(false));
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(h);
    };
  }, [lens, ready, busy, version, runCapture]);
  return { graph, previous: previousRef.current, loading };
}
