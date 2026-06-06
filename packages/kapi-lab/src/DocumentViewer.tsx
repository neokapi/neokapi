import React, { useMemo, useState } from "react";
import { Download, FileWarning } from "lucide-react";
import {
  Badge,
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  cn,
} from "@neokapi/ui-primitives";
import FormatPreview, { type PreviewSide } from "./FormatPreview";
import BlockInspector from "./BlockInspector";
import { FileIcon, fileType } from "./fileTypes";
import { downloadBytes, formatBytes } from "./download";
import { treeToRenderDoc } from "./renderDoc";
import type { ContentNode, ContentTree } from "./types";
import type { TransitionEffect } from "./useTextTransition";

// DocumentViewer — wraps FormatPreview with view-switching tabs (Preview ·
// Blocks · Stats · Download), a source↔target locale toggle, a transition-effect
// selector, and an annotations toggle. It is self-contained: give it a
// ContentTree (and optionally the original bytes for Download) and it shows the
// document four ways without booting the WASM runtime.

export interface DocumentViewerProps {
  /** The engine output (from `kapi inspect` / labInspectAnnotated). */
  tree: ContentTree;
  /** File name shown in the header + used for the type icon / download. */
  filename: string;
  /** Original file bytes, enabling the Download tab/button (optional). */
  bytes?: Uint8Array | null;
  /** Tab shown first (default "preview"). */
  defaultTab?: "preview" | "blocks" | "stats" | "download";
  /** Initial transition effect (default "none"). */
  defaultTransition?: TransitionEffect;
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

const TRANSITIONS: { value: TransitionEffect; label: string }[] = [
  { value: "none", label: "Instant" },
  { value: "crossfade", label: "Crossfade" },
  { value: "typewriter", label: "Typewriter" },
];

export default function DocumentViewer({
  tree,
  filename,
  bytes,
  defaultTab = "preview",
  defaultTransition = "none",
  className,
}: DocumentViewerProps): React.ReactElement {
  const ft = fileType(filename);
  const doc = useMemo(() => treeToRenderDoc(tree), [tree]);
  const blocks = useMemo(() => flattenBlocks(tree), [tree]);
  const locales = doc.locales ?? [];

  const [side, setSide] = useState<PreviewSide>("source");
  const [transition, setTransition] = useState<TransitionEffect>(defaultTransition);
  const [annotations, setAnnotations] = useState(true);

  // Per-structure counts for the Stats tab.
  const stats = useMemo(() => {
    const rows: { label: string; value: number }[] = [];
    rows.push({ label: "Blocks", value: blocks.length });
    rows.push({
      label: "Words",
      value: blocks.reduce((sum, b) => {
        const src = (b.source ?? []).map((r) => r.text ?? "").join("");
        return sum + wordCount(src);
      }, 0),
    });
    if (doc.kind === "slides") rows.push({ label: "Slides", value: doc.slides?.length ?? 0 });
    if (doc.kind === "sheet") {
      rows.push({ label: "Sheets", value: doc.sheets?.length ?? 1 });
      const cells = (doc.sheets ?? (doc.sheet ? [doc.sheet] : [])).reduce(
        (n, sh) => n + sh.cells.length,
        0,
      );
      rows.push({ label: "Cells", value: cells });
    }
    if (doc.kind === "pages") rows.push({ label: "Pages", value: doc.pages?.length ?? 0 });
    if (doc.kind === "sections") rows.push({ label: "Sections", value: doc.sections?.length ?? 0 });
    if (locales.length > 0) rows.push({ label: "Target locales", value: locales.length });
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
        <Button
          variant="outline"
          size="sm"
          className="ml-auto"
          disabled={!bytes}
          onClick={() => bytes && downloadBytes(filename, bytes)}
        >
          <Download /> Download
        </Button>
      </div>

      <Tabs defaultValue={defaultTab} className="px-3 pb-3">
        <div className="flex flex-wrap items-center gap-2">
          <TabsList variant="line">
            <TabsTrigger value="preview">Preview</TabsTrigger>
            <TabsTrigger value="blocks">
              Blocks{" "}
              {blocks.length > 0 && (
                <Badge variant="ghost" className="ml-1">
                  {blocks.length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="stats">Stats</TabsTrigger>
            <TabsTrigger value="download">Download</TabsTrigger>
          </TabsList>
        </div>

        {/* Preview controls */}
        <TabsContent value="preview" className="flex flex-col gap-3 pt-3">
          <div className="flex flex-wrap items-center gap-3">
            {/* source ↔ target toggle */}
            <label className="flex items-center gap-1.5 text-xs">
              <span className="text-muted-foreground">Side</span>
              <Select value={side} onValueChange={(v) => setSide(v)}>
                <SelectTrigger size="sm" className="h-7 w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="source">Source</SelectItem>
                  {locales.map((loc) => (
                    <SelectItem key={loc} value={loc}>
                      {loc}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>

            {/* transition effect */}
            <label className="flex items-center gap-1.5 text-xs">
              <span className="text-muted-foreground">Transition</span>
              <Select
                value={transition}
                onValueChange={(v) => setTransition(v as TransitionEffect)}
              >
                <SelectTrigger size="sm" className="h-7 w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TRANSITIONS.map((t) => (
                    <SelectItem key={t.value} value={t.value}>
                      {t.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>

            {/* annotations toggle */}
            <label className="flex items-center gap-1.5 text-xs">
              <Switch checked={annotations} onCheckedChange={setAnnotations} />
              <span className="text-muted-foreground">Annotations</span>
            </label>
          </div>

          <FormatPreview
            tree={tree}
            side={side}
            transition={transition}
            annotations={annotations}
          />
        </TabsContent>

        {/* Blocks */}
        <TabsContent value="blocks" className="pt-2">
          {blocks.length === 0 ? (
            <p className="py-3 text-sm text-muted-foreground">No translatable blocks.</p>
          ) : (
            <div className="flex flex-col gap-1.5">
              {blocks.map((b) => (
                <BlockInspector key={b.id} node={b} />
              ))}
            </div>
          )}
        </TabsContent>

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

        {/* Download */}
        <TabsContent value="download" className="pt-3">
          {bytes ? (
            <div className="flex flex-col items-start gap-2">
              <p className="text-sm text-muted-foreground">
                {filename} · {formatBytes(bytes.length)}
              </p>
              <Button variant="default" size="sm" onClick={() => downloadBytes(filename, bytes)}>
                <Download /> Download {ft.label}
              </Button>
            </div>
          ) : (
            <div className="flex items-center gap-2 py-3 text-sm text-muted-foreground">
              <FileWarning className="size-4" /> No file bytes available to download.
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
