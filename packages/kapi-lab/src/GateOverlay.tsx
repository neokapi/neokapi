import React from "react";
import RunGate from "./RunGate";
import type { RunGate as RunGateState } from "./useRunGate";

// GateOverlay — the shared, zero-shift Run gate.
//
// Every lab renders its body at full height ALWAYS, and drops a <GateOverlay>
// as the last child of a `position: relative` container. Until the engine is
// ready the overlay veils the body with the shared RunGate (idle → booting →
// error) over a translucent, blurred backdrop, so the idle body reads as a
// dimmed static preview; once ready it renders nothing. Because the body is
// laid out behind the overlay the whole time — idle, booting, and ready —
// revealing it is a pure dissolve with NO layout shift (the page below never
// jumps).
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
  /** Primary action label (default "Run in your browser"). */
  label?: string;
  /** Whether Run boots the shared wasm engine (default true) — see RunGate. */
  engine?: boolean;
}

export default function GateOverlay({
  gate,
  title,
  description,
  label,
  engine,
}: GateOverlayProps): React.ReactElement | null {
  if (gate.ready) return null;
  return (
    // A translucent veil rather than an opaque sheet: the lab's idle body shows
    // through, dimmed — a static preview of what Run will bring to life. The
    // overlay intercepts all pointer events, so the preview stays inert.
    <div className="absolute inset-0 z-40 bg-background/85 backdrop-blur-[2px]">
      <RunGate
        gate={gate}
        title={title}
        description={description}
        label={label}
        engine={engine}
        className="h-full rounded-none border-none bg-transparent"
      />
    </div>
  );
}
