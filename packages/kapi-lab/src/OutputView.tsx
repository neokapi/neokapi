import React, { useEffect, useMemo, useRef, useState } from "react";
import { Download, FileWarning } from "lucide-react";
import {
  Badge,
  Button,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  cn,
} from "@neokapi/ui-primitives";
import { FileIcon, fileType } from "./fileTypes";
import { downloadBytes, formatBytes } from "./download";
import CodeView from "./CodeView";
import BlockInspector from "./BlockInspector";
import ContentTreeView from "./ContentTreeView";
import type { LabRuntime } from "./useLabRuntime";
import type { ContentNode, ContentTree } from "./types";
import styles from "./OutputView.module.css";

export interface OutputViewProps {
  runtime: LabRuntime;
  /** Path in the in-memory filesystem (absolute /project/… or relative). */
  path: string;
  /** Bump to force a re-read after a run wrote new bytes to `path`. */
  version?: number;
  /** Tab shown first (default "blocks"). */
  defaultTab?: "blocks" | "structure" | "native";
  className?: string;
}

function basename(p: string): string {
  return p.replace(/\/+$/, "").split("/").pop() || p;
}

function flattenBlocks(tree: ContentTree | null): ContentNode[] {
  if (!tree) return [];
  const out: ContentNode[] = [];
  const walk = (n: ContentNode) => {
    if (n.kind === "block") out.push(n);
    n.children?.forEach(walk);
  };
  tree.root.forEach(walk);
  return out;
}

// A signature capturing a block's translatable content, used to detect which
// blocks a run changed (by id) between successive reads.
function blockSignature(n: ContentNode): string {
  return JSON.stringify([n.source, n.targets ?? null]);
}

function diffChangedIds(prev: ContentNode[] | null, next: ContentNode[]): Set<string> {
  if (!prev) return new Set();
  const before = new Map(prev.map((b) => [b.id, blockSignature(b)]));
  const changed = new Set<string>();
  for (const b of next) {
    const sig = blockSignature(b);
    if (!before.has(b.id) || before.get(b.id) !== sig) changed.add(b.id);
  }
  return changed;
}

function diffChangedLines(prev: string | null, next: string): Set<number> {
  if (prev === null) return new Set();
  const a = prev.split("\n");
  const b = next.split("\n");
  const changed = new Set<number>();
  for (let i = 0; i < b.length; i++) {
    if (a[i] !== b[i]) changed.add(i);
  }
  return changed;
}

// OutputView renders a file the engine wrote, three ways: as the content-model
// Blocks (full inspector), as the hierarchical Structure, and as the raw Native
// bytes with syntax highlighting. It offers a download, and — when a re-run
// changes the bytes — flashes the header and highlights exactly which blocks and
// lines changed, so a learner can see what the pipeline produced.
export default function OutputView({
  runtime,
  path,
  version = 0,
  defaultTab = "blocks",
  className,
}: OutputViewProps): React.ReactElement {
  const name = basename(path);
  const ft = fileType(name);

  const [tree, setTree] = useState<ContentTree | null>(null);
  const [text, setText] = useState<string>("");
  const [bytes, setBytes] = useState<Uint8Array | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pulse, setPulse] = useState(false);
  const [changedIds, setChangedIds] = useState<Set<string>>(new Set());
  const [changedLines, setChangedLines] = useState<Set<number>>(new Set());

  const prevBlocksRef = useRef<ContentNode[] | null>(null);
  const prevTextRef = useRef<string | null>(null);

  useEffect(() => {
    if (!runtime.ready) return;
    let cancelled = false;
    const raw = runtime.readBytes(path);
    if (!raw) {
      setError("no output at " + path);
      return;
    }
    const decoded = ft.binary ? "" : new TextDecoder().decode(raw);

    void (async () => {
      const res = await runtime.inspect(name, raw);
      if (cancelled) return;
      const nextTree = res.ok && res.tree ? res.tree : null;
      const nextBlocks = flattenBlocks(nextTree);

      const idDiff = diffChangedIds(prevBlocksRef.current, nextBlocks);
      const lineDiff = diffChangedLines(prevTextRef.current, decoded);
      const didChange = prevTextRef.current !== null && prevTextRef.current !== decoded;

      setTree(nextTree);
      setText(decoded);
      setBytes(raw);
      setError(res.ok ? null : (res.error ?? "could not read output"));
      setChangedIds(idDiff);
      setChangedLines(lineDiff);

      if (didChange) {
        setPulse(true);
        setTimeout(() => !cancelled && setPulse(false), 1600);
      }

      prevBlocksRef.current = nextBlocks;
      prevTextRef.current = decoded;
    })();

    return () => {
      cancelled = true;
    };
    // re-read when the path or the run version changes.
  }, [runtime.ready, runtime.inspect, runtime.readBytes, path, version, name, ft.binary]);

  const blocks = useMemo(() => flattenBlocks(tree), [tree]);

  return (
    <div
      className={cn(
        "kapi-reference flex flex-col gap-2 rounded-lg border bg-card text-foreground",
        pulse && styles.writePulse,
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-2 border-b px-3 py-2">
        <FileIcon filename={name} size={16} />
        <span className="font-mono text-sm">{name}</span>
        <Badge variant="outline" style={{ color: ft.color }}>
          {ft.label}
        </Badge>
        {bytes && (
          <span className="text-xs tabular-nums text-muted-foreground">
            {formatBytes(bytes.length)}
          </span>
        )}
        {pulse && (
          <Badge variant="outline" className="border-warning/50 text-warning">
            updated
          </Badge>
        )}
        <Button
          variant="outline"
          size="sm"
          className="ml-auto"
          disabled={!bytes}
          onClick={() => bytes && downloadBytes(name, bytes)}
        >
          <Download /> Download
        </Button>
      </div>

      {error ? (
        <div className="flex items-center gap-2 px-3 py-4 text-sm text-muted-foreground">
          <FileWarning className="size-4" /> {error}
        </div>
      ) : (
        <Tabs defaultValue={defaultTab} className="px-3 pb-3">
          <TabsList variant="line">
            <TabsTrigger value="blocks">
              Blocks{" "}
              {blocks.length > 0 && (
                <Badge variant="ghost" className="ml-1">
                  {blocks.length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value="structure">Structure</TabsTrigger>
            <TabsTrigger value="native">Native</TabsTrigger>
          </TabsList>

          <TabsContent value="blocks" className="pt-2">
            {blocks.length === 0 ? (
              <p className="py-3 text-sm text-muted-foreground">No translatable blocks.</p>
            ) : (
              <div className="flex flex-col gap-1.5">
                {blocks.map((b) => (
                  <BlockInspector
                    key={b.id}
                    node={b}
                    changed={changedIds.has(b.id)}
                    defaultOpen={changedIds.has(b.id)}
                  />
                ))}
              </div>
            )}
          </TabsContent>

          <TabsContent value="structure" className="pt-2">
            {tree ? (
              <ContentTreeView tree={tree} changedIds={changedIds} />
            ) : (
              <p className="text-sm text-muted-foreground">No structure.</p>
            )}
          </TabsContent>

          <TabsContent value="native" className="pt-2">
            {ft.binary ? (
              <div className="flex items-center gap-2 py-3 text-sm text-muted-foreground">
                <FileWarning className="size-4" /> Binary {ft.label} — download to inspect.
              </div>
            ) : (
              <CodeView text={text} filename={name} changedLines={changedLines} />
            )}
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}
