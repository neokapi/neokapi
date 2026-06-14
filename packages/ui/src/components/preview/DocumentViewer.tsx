import React, { useMemo, useState } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { Download, FileWarning } from "lucide-react";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "../ui/toggle-group";
import FormatPreview, { type PreviewSide } from "./FormatPreview";
import BlockInspector from "./BlockInspector";
import StructureView from "./StructureView";
import LayoutView from "./LayoutView";
import CodeView from "./CodeView";
import { FileIcon, fileType } from "./fileTypes";
import { downloadBytes, formatBytes } from "./download";
import { treeToRenderDoc } from "./renderDoc";
import type { ContentNode, ContentTree } from "./types";

// DocumentViewer — the shared preview editor. View-switching tabs let the reader
// toggle between the structure Preview (FormatPreview), the Blocks list
// (BlockInspector), and the Raw source (CodeView, syntax-highlighted) — plus
// Stats and Download. When a target locale exists, a compact source↔target
// toggle sits in the header; annotations are always on and the swap crossfades.
// It is self-contained: give it a ContentTree (and optionally the original bytes
// for Raw/Download) and it shows the document every way without booting the WASM
// runtime. Hosts may pass `changedIds`/`rawChangedLines` to flag what a run
// changed.

export interface DocumentViewerProps {
  /** The engine output (from `kapi inspect` / labInspectAnnotated). */
  tree: ContentTree;
  /** File name shown in the header + used for the type icon / download. */
  filename: string;
  /** Original file bytes, enabling the Download tab/button (optional). */
  bytes?: Uint8Array | null;
  /** Tab shown first (default "preview"). */
  defaultTab?: "preview" | "structure" | "layout" | "blocks" | "raw" | "stats" | "download";
  /** Block ids changed by a recent run — flagged + auto-opened in Blocks. */
  changedIds?: ReadonlySet<string>;
  /** Raw-view line numbers changed by a recent run — highlighted in Raw. */
  rawChangedLines?: ReadonlySet<number>;
  className?: string;
}

function flattenBlocks(tree: ContentTree): ContentNode[] {
  const out: ContentNode[] = [];
  const walk = (n: ContentNode) => {
    if (n.kind === "block") out.push(n);
    n.children?.forEach(walk);
  };
  tree.root.forEach(walk);
  return out;
}

function wordCount(s: string): number {
  const t = s.trim();
  return t ? t.split(/\s+/).length : 0;
}

export default function DocumentViewer({
  tree,
  filename,
  bytes,
  defaultTab = "preview",
  changedIds,
  rawChangedLines,
  className,
}: DocumentViewerProps): React.ReactElement {
  const ft = fileType(filename);
  const doc = useMemo(() => treeToRenderDoc(tree), [tree]);
  const blocks = useMemo(() => flattenBlocks(tree), [tree]);
  const locales = doc.locales ?? [];
  // The structure layer (WS1): show the Structure outline whenever any block
  // carries a role, and the spatial Layout tab only when page geometry exists.
  const hasStructure = useMemo(() => blocks.some((b) => b.structure?.role), [blocks]);
  const hasGeometry = useMemo(() => blocks.some((b) => b.geometry), [blocks]);

  // Decode the raw source for the Raw tab (text formats only; binary stays a
  // notice — its bytes are a zip/blob, not readable source).
  const rawText = useMemo(
    () => (bytes && !ft.binary ? new TextDecoder().decode(bytes) : ""),
    [bytes, ft.binary],
  );

  const [side, setSide] = useState<PreviewSide>("source");

  // Per-structure counts for the Stats tab.
  const stats = useMemo(() => {
    const rows: { label: string; value: number }[] = [];
    rows.push({ label: t("Blocks"), value: blocks.length });
    rows.push({
      label: t("Words"),
      value: blocks.reduce((sum, b) => {
        const src = (b.source ?? []).map((r) => r.text ?? "").join("");
        return sum + wordCount(src);
      }, 0),
    });
    if (doc.kind === "slides") rows.push({ label: t("Slides"), value: doc.slides?.length ?? 0 });
    if (doc.kind === "sheet") {
      rows.push({ label: t("Sheets"), value: doc.sheets?.length ?? 1 });
      const cells = (doc.sheets ?? (doc.sheet ? [doc.sheet] : [])).reduce(
        (n, sh) => n + sh.cells.length,
        0,
      );
      rows.push({ label: t("Cells"), value: cells });
    }
    if (doc.kind === "pages") rows.push({ label: t("Pages"), value: doc.pages?.length ?? 0 });
    if (doc.kind === "sections")
      rows.push({ label: t("Sections"), value: doc.sections?.length ?? 0 });
    if (locales.length > 0) rows.push({ label: t("Target locales"), value: locales.length });
    return rows;
  }, [blocks, doc, locales.length]);

  return (
    <div
      className={cn(
        "kapi-reference flex flex-col gap-2 rounded-lg border bg-card text-foreground",
        className,
      )}
    >
      {/* Header */}
      <div className="flex flex-wrap items-center gap-2 border-b px-3 py-2">
        <FileIcon filename={filename} size={16} />
        <span className="font-mono text-sm">{filename}</span>
        <Badge variant="outline" className={cn("border-current/35", ft.colorClass)}>
          {ft.label}
        </Badge>
        {bytes && (
          <span className="text-xs tabular-nums text-muted-foreground">
            {formatBytes(bytes.length)}
          </span>
        )}
        {/* Compact source↔target toggle — only when a target locale exists. */}
        {locales.length > 0 && (
          <ToggleGroup
            type="single"
            size="sm"
            value={side}
            onValueChange={(v) => v && setSide(v)}
            aria-label="Source or target"
            className="ml-auto"
          >
            <ToggleGroupItem value="source">Source</ToggleGroupItem>
            {locales.map((loc) => (
              <ToggleGroupItem key={loc} value={loc}>
                {loc}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        )}
        {/* Download only when we actually hold the bytes (the web/wasm path).
            On desktop the file is already local, so no bytes are passed and the
            control is hidden. */}
        {bytes && (
          <Button
            variant="outline"
            size="sm"
            className={locales.length > 0 ? "" : "ml-auto"}
            onClick={() => downloadBytes(filename, bytes)}
          >
            <Download /> Download
          </Button>
        )}
      </div>

      {/* Force column layout: the Tabs primitive's orientation variant keys on a
          `data-horizontal` attribute that isn't emitted, so without flex-col the
          list renders BESIDE the content (eating half the width). */}
      <Tabs defaultValue={defaultTab} className="flex flex-col px-3 pb-3">
        {/* Compact segmented control (w-fit), not a full-width tab bar. */}
        <TabsList>
          <TabsTrigger value="preview">Preview</TabsTrigger>
          {hasStructure && <TabsTrigger value="structure">Structure</TabsTrigger>}
          {hasGeometry && <TabsTrigger value="layout">Layout</TabsTrigger>}
          <TabsTrigger value="blocks">
            Blocks{" "}
            {blocks.length > 0 && (
              <Badge variant="ghost" className="ml-1">
                {blocks.length}
              </Badge>
            )}
          </TabsTrigger>
          {/* Raw + Download need the file bytes — shown on the web/wasm path,
              hidden on desktop where the local file is the source of truth. */}
          {bytes && <TabsTrigger value="raw">Raw</TabsTrigger>}
          <TabsTrigger value="stats">Stats</TabsTrigger>
          {bytes && <TabsTrigger value="download">Download</TabsTrigger>}
        </TabsList>

        <TabsContent value="preview" className="pt-3">
          <FormatPreview tree={tree} side={side} transition="crossfade" annotations />
        </TabsContent>

        {/* Structure — the role-driven outline (WS1/WS5). */}
        {hasStructure && (
          <TabsContent value="structure" className="pt-2">
            <StructureView tree={tree} side={side} />
          </TabsContent>
        )}

        {/* Layout — the spatial page view, when geometry exists (WS1/WS5). */}
        {hasGeometry && (
          <TabsContent value="layout" className="pt-3">
            <LayoutView tree={tree} side={side} />
          </TabsContent>
        )}

        {/* Blocks */}
        <TabsContent value="blocks" className="pt-2">
          {blocks.length === 0 ? (
            <p className="py-3 text-sm text-muted-foreground">No translatable blocks.</p>
          ) : (
            <div className="flex flex-col gap-1.5">
              {blocks.map((b) => (
                <BlockInspector
                  key={b.id}
                  node={b}
                  changed={changedIds?.has(b.id)}
                  defaultOpen={changedIds?.has(b.id)}
                />
              ))}
            </div>
          )}
        </TabsContent>

        {/* Raw source (only when bytes are present) */}
        {bytes && (
          <TabsContent value="raw" className="pt-2">
            {ft.binary ? (
              <div className="flex items-center gap-2 py-3 text-sm text-muted-foreground">
                <FileWarning className="size-4" /> Binary {ft.label} — use Preview, or download to
                inspect the raw bytes.
              </div>
            ) : (
              <CodeView text={rawText} filename={filename} changedLines={rawChangedLines} />
            )}
          </TabsContent>
        )}

        {/* Stats */}
        <TabsContent value="stats" className="pt-3">
          <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            {stats.map((s) => (
              <div key={s.label} className="rounded-md border bg-muted/30 px-3 py-2">
                <dt className="text-xs text-muted-foreground">{s.label}</dt>
                <dd className="text-lg font-semibold tabular-nums">{s.value}</dd>
              </div>
            ))}
          </dl>
        </TabsContent>

        {/* Download (only when bytes are present) */}
        {bytes && (
          <TabsContent value="download" className="pt-3">
            <div className="flex flex-col items-start gap-2">
              <p className="text-sm text-muted-foreground">
                {filename} · {formatBytes(bytes.length)}
              </p>
              <Button variant="default" size="sm" onClick={() => downloadBytes(filename, bytes)}>
                <Download /> Download {ft.label}
              </Button>
            </div>
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}
