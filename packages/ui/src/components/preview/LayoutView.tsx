import React, { useMemo } from "react";
import { cn } from "../../lib/utils";
import { runsText } from "./renderDoc";
import { roleStyle } from "./roleStyle";
import type { ContentNode, ContentTree, GeometryView } from "./types";

// LayoutView — the editor's spatial layout view (WS5). For layout-aware sources
// (PDF, Docling/DocLang, slide/sheet coordinates) it places each block's
// bounding box on a scaled page, colored by role and numbered by reading order,
// so the document's physical structure is legible — the spatial counterpart to
// the reflowable StructureView. It is a pure projection of the ContentTree's
// `geometry` field; blocks without geometry are listed separately as unplaced.

export interface LayoutViewProps {
  tree: ContentTree;
  /** "source" or a target variant key — selects which text the boxes show. */
  side?: string;
  className?: string;
}

function blockText(node: ContentNode, side: string): string {
  if (side && side !== "source") {
    const t = node.targets?.[side];
    if (t && t.length > 0) return runsText(t);
  }
  return runsText(node.source);
}

interface PlacedBlock {
  node: ContentNode;
  g: GeometryView;
  order: number;
}

interface Page {
  page: number;
  blocks: PlacedBlock[];
  /** Coordinate-space extent the boxes live in (for scaling to the rendered page). */
  extentW: number;
  extentH: number;
}

function flattenGeo(tree: ContentTree): { placed: PlacedBlock[]; unplaced: ContentNode[] } {
  const placed: PlacedBlock[] = [];
  const unplaced: ContentNode[] = [];
  let order = 0;
  const walk = (n: ContentNode) => {
    if (n.kind === "block") {
      order += 1;
      if (n.geometry) placed.push({ node: n, g: n.geometry, order });
      else unplaced.push(n);
    }
    n.children?.forEach(walk);
  };
  tree.root.forEach(walk);
  return { placed, unplaced };
}

function groupPages(placed: PlacedBlock[]): Page[] {
  const byPage = new Map<number, PlacedBlock[]>();
  for (const p of placed) {
    const pg = p.g.page && p.g.page > 0 ? p.g.page : 1;
    (byPage.get(pg) ?? byPage.set(pg, []).get(pg)!).push(p);
  }
  const pages: Page[] = [];
  for (const [page, blocks] of [...byPage.entries()].sort((a, b) => a[0] - b[0])) {
    // Extent: the normalized grid when a resolution is given (DocLang = 512),
    // else the max box extent (+ a small margin) so absolute-point boxes fit.
    const res = blocks.find((b) => b.g.resolution && b.g.resolution > 0)?.g.resolution ?? 0;
    let extentW = res;
    let extentH = res;
    if (!res) {
      let maxX = 0;
      let maxY = 0;
      for (const b of blocks) {
        maxX = Math.max(maxX, b.g.x + b.g.w);
        maxY = Math.max(maxY, b.g.y + b.g.h);
      }
      extentW = maxX > 0 ? maxX * 1.04 : 1;
      extentH = maxY > 0 ? maxY * 1.04 : 1;
    }
    pages.push({ page, blocks, extentW, extentH });
  }
  return pages;
}

// glyphTop flips a glyph's y to top-down when the block uses a bottom-left origin
// (glyphs share their block's coordinate space/origin).
function glyphTop(g: GeometryView, gy: number, gh: number, extentH: number): number {
  return g.origin === "bottom-left" ? extentH - gy - gh : gy;
}

function PageCanvas({
  page,
  side,
  showGlyphs,
}: {
  page: Page;
  side: string;
  showGlyphs: boolean;
}): React.ReactElement {
  const { extentW, extentH } = page;
  return (
    <figure className="m-0 flex flex-col gap-1">
      <figcaption className="text-xs text-muted-foreground">
        Page {page.page} · {page.blocks.length} block{page.blocks.length === 1 ? "" : "s"}
      </figcaption>
      <div
        className="relative w-full max-w-sm overflow-hidden rounded-md border bg-muted/20 shadow-sm"
        style={{ aspectRatio: `${extentW} / ${extentH}` }}
        data-testid="layout-page"
      >
        {page.blocks.map((b) => {
          const rs = roleStyle(b.node.structure?.role ?? b.node.type, b.node.structure?.level);
          // Bottom-left origin: flip Y so the page renders top-down.
          const topUnits = b.g.origin === "bottom-left" ? extentH - b.g.y - b.g.h : b.g.y;
          const text = blockText(b.node, side).trim();
          return (
            <div
              key={b.node.id}
              data-testid="layout-box"
              data-role={b.node.structure?.role ?? b.node.type ?? ""}
              className={cn(
                "absolute overflow-hidden rounded-[2px] border border-current/40 px-1 py-0.5 text-[9px] leading-tight",
                rs.className,
              )}
              style={{
                left: `${(b.g.x / extentW) * 100}%`,
                top: `${(topUnits / extentH) * 100}%`,
                width: `${(b.g.w / extentW) * 100}%`,
                height: `${(b.g.h / extentH) * 100}%`,
              }}
              title={`#${b.order} ${rs.label}\n${text}\n[${Math.round(b.g.x)}, ${Math.round(
                b.g.y,
              )}, ${Math.round(b.g.w)}×${Math.round(b.g.h)}]`}
            >
              <span className="font-mono opacity-60">{b.order}</span>{" "}
              <span className="align-middle">{text}</span>
            </div>
          );
        })}
        {showGlyphs &&
          page.blocks.flatMap((b) =>
            (b.g.glyphs ?? []).map((gl, i) => (
              <div
                key={`${b.node.id}-g${i}`}
                data-testid="layout-glyph"
                className="pointer-events-none absolute rounded-[1px] border border-sky-500/70 bg-sky-400/10"
                style={{
                  left: `${(gl.x / extentW) * 100}%`,
                  top: `${(glyphTop(b.g, gl.y, gl.h, extentH) / extentH) * 100}%`,
                  width: `${(gl.w / extentW) * 100}%`,
                  height: `${(gl.h / extentH) * 100}%`,
                }}
                title={gl.text}
              />
            )),
          )}
      </div>
    </figure>
  );
}

export default function LayoutView({
  tree,
  side = "source",
  className,
}: LayoutViewProps): React.ReactElement {
  const { placed, unplaced } = useMemo(() => flattenGeo(tree), [tree]);
  const pages = useMemo(() => groupPages(placed), [placed]);
  const hasGlyphs = useMemo(() => placed.some((b) => (b.g.glyphs?.length ?? 0) > 0), [placed]);
  const [showGlyphs, setShowGlyphs] = React.useState(false);

  if (placed.length === 0) {
    return (
      <p className="py-3 text-sm text-muted-foreground">
        No page geometry in this document — the layout view applies to layout-aware sources (PDF,
        Docling/DocLang, slides). Use the Structure tab for the reflowable outline.
      </p>
    );
  }

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {hasGlyphs && (
        <label className="flex w-fit items-center gap-2 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={showGlyphs}
            onChange={(e) => setShowGlyphs(e.target.checked)}
          />
          Per-glyph geometry (show character boxes)
        </label>
      )}
      <div className="flex flex-wrap gap-4">
        {pages.map((p) => (
          <PageCanvas key={p.page} page={p} side={side} showGlyphs={showGlyphs} />
        ))}
      </div>
      {unplaced.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {unplaced.length} block{unplaced.length === 1 ? "" : "s"} without geometry (not shown on a
          page).
        </p>
      )}
    </div>
  );
}
