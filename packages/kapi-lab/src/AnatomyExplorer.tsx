import React, { useEffect, useState } from "react";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import RunSequence from "./RunSequence";
import { SAMPLES } from "./samples";
import type { ContentNode, ContentStats, ContentTree } from "./types";
import styles from "./styles.module.css";

export interface AnatomyExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Sample selected on first render (default: first sample). */
  defaultSampleId?: string;
  /** Restrict the offered samples. */
  sampleIds?: string[];
}

// AnatomyExplorer decomposes a file (a bundled sample or the learner's own) into
// the neokapi content model — Layers → Groups → Blocks (with their run
// sequences) → Data / Media — by running the real reader in WASM via labInspect.
export default function AnatomyExplorer({
  assets,
  defaultSampleId,
  sampleIds,
}: AnatomyExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);

  const initial = SAMPLES.find((s) => s.id === defaultSampleId) ?? SAMPLES[0];
  const [file, setFile] = useState<FileSourceValue>({
    filename: initial.filename,
    content: initial.content,
    label: initial.label,
  });
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Re-inspect whenever the runtime becomes ready or the file changes.
  useEffect(() => {
    if (!runtime.ready) return;
    let cancelled = false;
    setBusy(true);
    setError(null);
    runtime
      .inspect(file.filename, file.bytes ?? file.content)
      .then((res) => {
        if (cancelled) return;
        if (res.ok && res.tree) {
          setTree(res.tree);
        } else {
          setError(res.error ?? "could not inspect file");
          setTree(null);
        }
      })
      .finally(() => !cancelled && setBusy(false));
    return () => {
      cancelled = true;
    };
    // runtime.inspect is a stable useCallback; depending on the whole runtime
    // object (new each render) would loop.
  }, [runtime.ready, runtime.inspect, file]);

  return (
    <div className={styles.explorer}>
      <FileSource value={file} onChange={setFile} sampleIds={sampleIds} />

      <div className={`${styles.statusBar} ${error ? styles.statusError : ""}`}>
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Reading…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
        {runtime.ready && !busy && !error && tree && (
          <Stats stats={tree.stats} format={tree.format} />
        )}
      </div>

      {tree && (
        <div className={styles.tree}>
          {tree.root.map((node) => (
            <NodeView key={`${node.kind}:${node.id}`} node={node} />
          ))}
        </div>
      )}
    </div>
  );
}

function Stats({ stats, format }: { stats: ContentStats; format: string }): React.ReactElement {
  const items: [string, number][] = [
    ["layers", stats.layers],
    ["groups", stats.groups],
    ["blocks", stats.blocks],
    ["data", stats.data],
    ["media", stats.media],
    ["runs", stats.runs],
  ];
  return (
    <div className={styles.stats}>
      <span className={styles.statBadge}>
        <span className={styles.statCount}>{format}</span> reader
      </span>
      {items
        .filter(([, n]) => n > 0)
        .map(([label, n]) => (
          <span key={label} className={styles.statBadge}>
            <span className={styles.statCount}>{n}</span> {label}
          </span>
        ))}
    </div>
  );
}

const KIND_CLASS: Record<string, string> = {
  layer: styles.kindLayer,
  group: styles.kindGroup,
  block: styles.kindBlock,
  data: styles.kindData,
  media: styles.kindMedia,
};

function NodeView({ node }: { node: ContentNode }): React.ReactElement {
  const isContainer = node.kind === "layer" || node.kind === "group";
  const [open, setOpen] = useState(true);

  const badge = <span className={`${styles.kindBadge} ${KIND_CLASS[node.kind]}`}>{node.kind}</span>;

  if (isContainer) {
    const meta = [node.format, node.locale].filter(Boolean).join(" · ");
    return (
      <div className={styles.treeNode}>
        <div
          className={`${styles.nodeRow} ${styles.nodeRowClickable}`}
          onClick={() => setOpen((o) => !o)}
        >
          <span className={styles.disclosure}>{open ? "▾" : "▸"}</span>
          {badge}
          <span className={styles.nodeId}>{node.name || node.id}</span>
          {meta && <span className={styles.nodeMeta}>{meta}</span>}
        </div>
        {open && node.children && node.children.length > 0 && (
          <div className={styles.treeChildren}>
            {node.children.map((child) => (
              <NodeView key={`${child.kind}:${child.id}`} node={child} />
            ))}
          </div>
        )}
      </div>
    );
  }

  if (node.kind === "block") {
    return (
      <div className={styles.treeNode}>
        <div className={styles.nodeRow}>
          {badge}
          <span className={styles.nodeId}>{node.id}</span>
          <RunSequence runs={node.source ?? []} segments={node.segments} />
        </div>
        {node.targets &&
          Object.entries(node.targets).map(([loc, runs]) => (
            <div key={loc} className={styles.targetRow}>
              <span className={styles.targetLocale}>{loc}</span>
              <RunSequence runs={runs} />
            </div>
          ))}
      </div>
    );
  }

  // data / media leaf
  return (
    <div className={styles.treeNode}>
      <div className={styles.nodeRow}>
        {badge}
        <span className={styles.nodeId}>{node.id}</span>
        {node.summary && <span className={styles.nodeMeta}>{node.summary}</span>}
        {node.hasSkeleton && <span className={styles.nodeMeta}>· skeleton</span>}
      </div>
    </div>
  );
}
