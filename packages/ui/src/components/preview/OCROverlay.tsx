import React from "react";
import { cn } from "../../lib/utils";
import { boxPercent, topUnits, type PlacedBlock } from "./geometry";
import { roleStyle } from "./roleStyle";

// OCROverlay — the reusable bounding-box layer promoted out of the lab's
// VisionExplorer. Given a set of geometry-bearing blocks and the coordinate
// extent they live in, it draws each block's box (role-colored via roleStyle,
// numbered by reading order) and, optionally, its per-glyph boxes — all as
// percentages of the surface so it scales with whatever it overlays (an image,
// a video frame). It is an absolutely-positioned layer: the parent owns the
// `relative` surface. Selection is bidirectional: `selectedBlockId` highlights a
// box; clicking a box calls `onSelectBlock` (toggling it off when reselected).

export interface OCROverlayProps {
  /** The placed blocks to draw (from flattenGeometry). */
  blocks: PlacedBlock[];
  /** Coordinate-space width the boxes' x/w are expressed in. */
  extentW: number;
  /** Coordinate-space height the boxes' y/h are expressed in. */
  extentH: number;
  /** Currently selected block id, highlighted; null/undefined = none. */
  selectedBlockId?: string | null;
  /** Clicking a box reports its id (or null when the selected box is clicked). */
  onSelectBlock?: (id: string | null) => void;
  /** Draw per-character glyph boxes when a block carries them. */
  showGlyphs?: boolean;
  className?: string;
}

export default function OCROverlay({
  blocks,
  extentW,
  extentH,
  selectedBlockId,
  onSelectBlock,
  showGlyphs = false,
  className,
}: OCROverlayProps): React.ReactElement {
  return (
    <div
      className={cn("pointer-events-none absolute inset-0", className)}
      data-testid="ocr-overlay"
    >
      {blocks.map((b) => {
        const rs = roleStyle(b.node.structure?.role ?? b.node.type, b.node.structure?.level);
        const p = boxPercent(b.g, extentW, extentH);
        const selected = selectedBlockId != null && selectedBlockId === b.node.id;
        return (
          <button
            key={b.node.id}
            type="button"
            data-testid="ocr-box"
            data-block-id={b.node.id}
            data-role={b.node.structure?.role ?? b.node.type ?? ""}
            data-selected={selected ? "true" : "false"}
            onClick={() => onSelectBlock?.(selected ? null : b.node.id)}
            title={`#${b.order} ${rs.label}`}
            className={cn(
              "pointer-events-auto absolute cursor-pointer rounded-[2px] border-2 transition-shadow",
              rs.className,
              selected
                ? "border-current opacity-90 ring-2 ring-current/60"
                : "border-current/50 opacity-70 hover:opacity-90",
            )}
            style={{
              left: `${p.left}%`,
              top: `${p.top}%`,
              width: `${p.width}%`,
              height: `${p.height}%`,
            }}
          >
            <span
              className={cn(
                "absolute -top-4 left-0 rounded-[2px] px-1 font-mono text-[9px] leading-tight",
                rs.className,
              )}
            >
              {b.order}
            </span>
          </button>
        );
      })}
      {showGlyphs &&
        blocks.flatMap((b) =>
          (b.g.glyphs ?? []).map((gl, i) => (
            <div
              key={`${b.node.id}-g${i}`}
              data-testid="ocr-glyph"
              className="pointer-events-none absolute rounded-[1px] border border-sky-500/70 bg-sky-400/10"
              style={{
                left: `${(gl.x / extentW) * 100}%`,
                top: `${(topUnits(b.g.origin, gl.y, gl.h, extentH) / extentH) * 100}%`,
                width: `${(gl.w / extentW) * 100}%`,
                height: `${(gl.h / extentH) * 100}%`,
              }}
              title={gl.text}
            />
          )),
        )}
    </div>
  );
}
