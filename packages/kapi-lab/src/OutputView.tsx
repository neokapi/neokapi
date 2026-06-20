import React, { useEffect, useMemo, useRef, useState } from "react";
import { FileWarning } from "lucide-react";
import { cn } from "@neokapi/ui-primitives";
import { DocumentViewer } from "@neokapi/ui-primitives/preview";
import { fileType } from "@neokapi/ui-primitives/preview";
import type { LabRuntime } from "./useLabRuntime";
import type { ContentNode, ContentTree } from "@neokapi/ui-primitives/preview";
import styles from "./OutputView.module.css";

export interface OutputViewProps {
  runtime: LabRuntime;
  /** Path in the in-memory filesystem (absolute /project/… or relative). */
  path: string;
  /** Bump to force a re-read after a run wrote new bytes to `path`. */
  version?: number;
  /** Tab shown first (default "preview"). */
  defaultTab?: "preview" | "blocks" | "raw" | "stats";
  /**
   * Round-trip baseline: when set, the Native tab's changed-line highlights
   * compare the output against THIS text (typically the flow's input file)
   * instead of against the previous run's output — showing that only block
   * text changed while the structure round-tripped.
   */
  baseline?: string | null;
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

// OutputView shows a file the engine wrote, in the shared preview editor
// (DocumentViewer — Preview / Blocks / Raw / Stats / Download). It is the
// engine-output adapter: it reads the bytes from the in-memory filesystem,
// parses them, and — when a re-run changes the output — flashes the card and
// flags exactly which blocks and raw lines changed, so a learner sees what the
// pipeline produced. The rendering itself is the same editor used everywhere.
export default function OutputView({
  runtime,
  path,
  version = 0,
  defaultTab = "preview",
  baseline,
  className,
}: OutputViewProps): React.ReactElement {
  const name = basename(path);
  const ft = fileType(name);

  const [tree, setTree] = useState<ContentTree | null>(null);
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
      const lineDiff =
        baseline != null && !ft.binary
          ? diffChangedLines(baseline, decoded)
          : diffChangedLines(prevTextRef.current, decoded);
      const didChange = prevTextRef.current !== null && prevTextRef.current !== decoded;

      setTree(nextTree);
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
  }, [runtime.ready, runtime.inspect, runtime.readBytes, path, version, name, ft.binary, baseline]);

  const pulseClass = useMemo(() => (pulse ? styles.writePulse : undefined), [pulse]);

  if (error || !tree) {
    return (
      <div
        className={cn(
          "kapi-reference flex items-center gap-2 rounded-lg border bg-card px-3 py-4 text-sm text-muted-foreground",
          className,
        )}
      >
        <FileWarning className="size-4" /> {error ?? "Reading output…"}
      </div>
    );
  }

  return (
    <DocumentViewer
      tree={tree}
      filename={name}
      bytes={bytes}
      defaultTab={defaultTab}
      changedIds={changedIds}
      rawChangedLines={changedLines}
      className={cn(pulseClass, className)}
    />
  );
}
