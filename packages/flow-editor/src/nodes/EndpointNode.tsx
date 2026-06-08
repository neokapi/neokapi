// EndpointNode — the Source / Sink binding rendered as a real graph node so the
// pipeline reads as one continuous flow: Source → tools → Sink. It is still a
// binding, not a processing step (graphToSteps ignores non-"tool" nodes); it
// carries a single handle (source: output, sink: input) that the connecting
// edge anchors to, and reuses the EndpointPicker UI for the dropdown.

import { Handle, Position, type NodeProps } from "@xyflow/react";
import { EndpointPicker } from "./EndpointPicker";
import type { FlowBinding } from "../types";

export function EndpointNode({ data }: NodeProps) {
  const role = data.role as "source" | "sink";
  const vertical = data.layoutDirection !== "horizontal";
  const binding = data.binding as FlowBinding | undefined;
  const readOnly = data.readOnly as boolean | undefined;
  const onBindingChange = data.onBindingChange as ((b: FlowBinding) => void) | undefined;
  const isSource = role === "source";
  const handlePosition =
    (data.handlePosition as Position) ??
    (isSource
      ? vertical
        ? Position.Bottom
        : Position.Right
      : vertical
        ? Position.Top
        : Position.Left);

  return (
    <div className="nodrag relative">
      <EndpointPicker
        role={role}
        binding={binding}
        onChange={onBindingChange}
        readOnly={readOnly}
      />
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
