// EndpointNode — the Source / Sink binding rendered as a real graph node so the
// pipeline reads as one continuous flow: Source → tools → Sink. It is still a
// binding, not a processing step (graphToSteps ignores non-"tool" nodes); it
// carries a single handle (source: output, sink: input) that the connecting
// edge anchors to, and reuses the EndpointPicker UI for the dropdown.

import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Eye } from "lucide-react";
import { EndpointPicker } from "./EndpointPicker";
import type { FlowBinding } from "../types";

export function EndpointNode({ data }: NodeProps) {
  const role = data.role as "source" | "sink";
  const binding = data.binding as FlowBinding | undefined;
  const readOnly = data.readOnly as boolean | undefined;
  const onBindingChange = data.onBindingChange as ((b: FlowBinding) => void) | undefined;
  const onInspect = data.onInspect as (() => void) | undefined;
  const lessonFocus = !!data.lessonFocus;
  const isSource = role === "source";
  // The serpentine layout supplies the handle side that faces the flow; fall
  // back to a left→right flow (source out the right, sink in from the left).
  const handlePosition =
    (data.handlePosition as Position) ?? (isSource ? Position.Right : Position.Left);

  return (
    <div className="nodrag relative">
      {/* Lesson focus ring — a guided step is pointing at this endpoint. */}
      {lessonFocus && (
        <span
          className="pointer-events-none absolute -inset-1 rounded-full"
          style={{
            boxShadow:
              "0 0 0 3px var(--primary), 0 0 18px 2px color-mix(in oklch, var(--primary) 45%, transparent)",
          }}
        />
      )}
      <EndpointPicker
        role={role}
        binding={binding}
        onChange={onBindingChange}
        readOnly={readOnly}
      />
      {/* Inspect satellite — opens the host's endpoint inspector (the pill
          itself is the binding dropdown trigger, so this needs its own
          affordance). Hangs centered under the pill, like the side-effect
          satellites on tool nodes. */}
      {onInspect && (
        <button
          type="button"
          onClick={onInspect}
          aria-label={isSource ? "Inspect the source content" : "Inspect the written output"}
          title={
            isSource
              ? "See the content model the reader produces from this input"
              : "See what the flow wrote"
          }
          className="absolute left-1/2 top-full z-10 mt-1 flex -translate-x-1/2 items-center gap-1 rounded-full border border-border bg-card px-2 py-0.5 text-[9px] font-semibold text-muted-foreground shadow-sm transition-colors hover:bg-accent hover:text-foreground"
        >
          <Eye size={10} />
          Inspect
        </button>
      )}
      <Handle
        type={isSource ? "source" : "target"}
        position={handlePosition}
        style={{
          width: 8,
          height: 8,
          background: "var(--muted-foreground)",
          border: "2px solid var(--card)",
        }}
      />
    </div>
  );
}
