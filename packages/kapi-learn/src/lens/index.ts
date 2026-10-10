// A lens is a view the player draws above the terminal and rebuilds after
// every command: the context graph is the first. A lab names one by id.

import type React from "react";
import ContextGraphLens from "./ContextGraphLens.tsx";
import type { ContextGraphLensProps } from "./ContextGraphLens.tsx";
import { buildContextGraph } from "./contextGraph.ts";
import type { GraphModel, LineRunner } from "./types.ts";

export interface Lens {
  id: string;
  /** What the lens shows, for the frame's bar. */
  label: string;
  /** Read the project through the engine and build what the view draws. */
  build: (run: LineRunner) => Promise<GraphModel>;
  Component: React.ComponentType<ContextGraphLensProps>;
}

export const LENSES: Record<string, Lens> = {
  "context-graph": {
    id: "context-graph",
    label: "Context graph",
    build: (run) => buildContextGraph(run),
    Component: ContextGraphLens,
  },
};

export type { Focus, GraphEdge, GraphModel, GraphNode, LineRunner } from "./types.ts";
export { buildContextGraph, parseJson } from "./contextGraph.ts";
export { layoutGraph } from "./layout.ts";
export { default as ContextGraphLens } from "./ContextGraphLens.tsx";
