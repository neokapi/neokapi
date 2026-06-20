import React, { useMemo, useState } from "react";
import { cn } from "../../lib/utils";
import { extentOf, flattenGeometry, type PlacedBlock } from "./geometry";
import OCROverlay from "./OCROverlay";
import type { ContentTree } from "./types";

// MediaCanvas — an image surface with a bounding-box overlay, promoted out of the
// lab's VisionExplorer into a reusable primitive. It renders the raster from a
// `src` URL and overlays the document's geometry blocks (OCR lines / layout
// regions) via OCROverlay, role-colored and numbered by reading order. The
// coordinate mapping reuses the spatial math (geometry.ts) — boxes positioned by
// resolution grid (normalized sources) fall back to the image's natural pixel
// size for absolute-unit OCR boxes. Selection is bidirectional: pass
// `selectedBlockId` to highlight a box; `onSelectBlock` fires when one is
// clicked, so an external list (BlockInspector) and the canvas stay in sync.

export interface MediaCanvasProps {
  /** Resolvable image URL (http(s), blob:, or data: URI). */
  src: string;
  /** The engine output; geometry blocks are read from here. */
  tree: ContentTree;
  /** Currently selected block id (external selection → highlight a box). */
  selectedBlockId?: string | null;
  /** Clicking a box reports its id (or null when reselected). */
  onSelectBlock?: (id: string | null) => void;
  /** Draw per-glyph boxes when present. */
  showGlyphs?: boolean;
  /** Alt text for the image. */
  alt?: string;
  className?: string;
}

export default function MediaCanvas({
  src,
  tree,
  selectedBlockId,
  onSelectBlock,
  showGlyphs = false,
  alt = "media",
  className,
}: MediaCanvasProps): React.ReactElement {
  const placed: PlacedBlock[] = useMemo(() => flattenGeometry(tree).placed, [tree]);
  // Natural pixel size of the loaded image — the fallback extent for
  // absolute-unit (resolution-less) OCR boxes. Until load it is 0 and the
  // resolution grid (if any) governs.
  const [natural, setNatural] = useState<{ w: number; h: number }>({ w: 0, h: 0 });
  const { extentW, extentH } = useMemo(
    () => extentOf(placed, natural.w, natural.h),
    [placed, natural.w, natural.h],
  );

  return (
    <div
      className={cn(
        "relative inline-block max-w-full overflow-hidden rounded-md border bg-muted/20",
        className,
      )}
      data-testid="media-canvas"
    >
      <img
        src={src}
        alt={alt}
        className="block h-auto max-w-full"
        onLoad={(e) =>
          setNatural({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })
        }
      />
      {placed.length > 0 && (
        <OCROverlay
          blocks={placed}
          extentW={extentW}
          extentH={extentH}
          selectedBlockId={selectedBlockId}
          onSelectBlock={onSelectBlock}
          showGlyphs={showGlyphs}
        />
      )}
    </div>
  );
}
