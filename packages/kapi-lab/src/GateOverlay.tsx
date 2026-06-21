import React from "react";
import RunGate from "./RunGate";
import type { RunGate as RunGateState } from "./useRunGate";

// GateOverlay — the shared, zero-shift Run gate.
//
// Every lab renders its body at full height ALWAYS, and drops a <GateOverlay>
// as the last child of a `position: relative` container. Until the engine is
// ready the overlay covers the body with an opaque RunGate (play → booting →
// error); once ready it renders nothing. Because the body is laid out behind
// the overlay the whole time — idle, booting, and ready — revealing it is a
// pure dissolve with NO layout shift (the page below never jumps).
//
// This replaces the old `if (!gate.armed) return <RunGate>` early-return that
// every explorer hand-rolled, which swapped a short gate card for a taller body
// and shifted everything beneath it.
//
// Contract for hosts: make the body root `relative`, and reserve the result
// area's height (a fixed/min-height container) so the body itself doesn't grow
// from ~0 to full once results arrive — otherwise the dissolve is clean but the
// body still resizes under it.

export interface GateOverlayProps {
  gate: RunGateState;
  /** Heading shown on the gate (e.g. "Content model"). */
  title?: string;
  /** One-line description of what activating will do. */
  description?: string;
  /** Play-button aria-label / tooltip (default "Run"). */
  label?: string;
}

export default function GateOverlay({
  gate,
  title,
  description,
  label,
}: GateOverlayProps): React.ReactElement | null {
  if (gate.ready) return null;
  return (
    <div className="absolute inset-0 z-40 bg-background">
      <RunGate
        gate={gate}
        title={title}
        description={description}
        label={label}
        className="h-full"
      />
    </div>
  );
}
