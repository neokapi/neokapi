import React, { useEffect, useMemo, useState } from "react";
import type { Block, File, Run } from "@neokapi/kapi-format";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import { KLF_SAMPLES, klfSampleById, klfText, ANNOTATIONS_KLFL } from "./klfFixtures";
import styles from "./Klf.module.css";

export interface KlfExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Sample selected on first render (default: the complete document). */
  defaultSampleId?: string;
  /** Hide the annotation overlay panel (e.g. for a compact inline embed). */
  hideAnnotations?: boolean;
}

interface KlfValidationError {
  kind: string;
  blockId: string;
  placeholder: string;
  runId: string;
  message: string;
}

interface BlockAnalysis {
  html: string;
  errors: KlfValidationError[];
}

interface ParsedAnnotation {
  id: string;
  anchor: AnyAnchor;
  resolution: { ok: boolean; kind?: string; reason?: string; detail?: string };
}

interface AnyAnchor {
  kind: "block" | "run" | "range" | "form";
  block: string;
  path?: Array<number | Record<string, string>>;
  runId?: string;
  offset?: number;
  length?: number;
  key?: string;
}

// KlfExplorer is the KLF Lab: edit a `.klf` document and watch the canonical Go
// engine (core/klf, compiled to WASM) parse it, canonicalize the bytes, render
// each block to its Level-1 HTML preview, validate the run structure, and
// resolve a companion `.klfl` annotation overlay anchor-by-anchor. Everything
// here is the real engine the CLI and server run — no mocks.
export default function KlfExplorer({
  assets,
  defaultSampleId,
  hideAnnotations,
}: KlfExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);

  const initial = klfSampleById(defaultSampleId ?? "full");
  const [sampleId, setSampleId] = useState(initial.id);
  const [klfValue, setKlfValue] = useState(() => klfText(initial.file));
  const [klflValue, setKlflValue] = useState(ANNOTATIONS_KLFL);
  const [selectedAnn, setSelectedAnn] = useState<string | null>(null);

  // Parse the editor text into a File (client-side JSON.parse). The Go engine
  // re-validates the envelope; this parse only drives the per-block UI.
  const parsed = useMemo<{ file?: File; error?: string }>(() => {
    try {
      return { file: JSON.parse(klfValue) as File };
    } catch (e) {
      return { error: (e as Error).message };
    }
  }, [klfValue]);

  const blocks: Block[] = useMemo(
    () => (parsed.file?.documents ?? []).flatMap((d) => d.blocks ?? []),
    [parsed.file],
  );

  // Canonical bytes + per-block render/validation from the Go engine.
  const [canonical, setCanonical] = useState<{ output: string; sha256: string } | null>(null);
  const [analysis, setAnalysis] = useState<Record<string, BlockAnalysis>>({});
  const [engineError, setEngineError] = useState<string | null>(null);

  useEffect(() => {
    if (!runtime.ready) return;
    if (parsed.error || !parsed.file) {
      setCanonical(null);
      setAnalysis({});
      setEngineError(null);
      return;
    }
    const round = runtime.klf({ op: "roundtrip", klf: klfValue });
    if (!round.ok) {
      setEngineError((round.error as string) ?? "the engine rejected this document");
      setCanonical(null);
      setAnalysis({});
      return;
    }
    setEngineError(null);
    setCanonical({ output: round.output as string, sha256: round.sha256 as string });

    const next: Record<string, BlockAnalysis> = {};
    for (const b of blocks) {
      const render = runtime.klf({ op: "renderHtml", block: b });
      const validate = runtime.klf({ op: "validateBlock", block: b });
      next[b.id] = {
        html: render.ok ? (render.html as string) : "",
        errors: validate.ok ? ((validate.errors as KlfValidationError[]) ?? []) : [],
      };
    }
    setAnalysis(next);
  }, [runtime.ready, runtime, klfValue, parsed.file, parsed.error, blocks]);

  // Resolve each annotation against its target block via the Go engine.
  const annotations = useMemo<ParsedAnnotation[]>(() => {
    if (!runtime.ready || hideAnnotations) return [];
    const byId = new Map(blocks.map((b) => [b.id, b]));
    const out: ParsedAnnotation[] = [];
    for (const line of klflValue.split("\n")) {
      const trimmed = line.trim();
      if (!trimmed) continue;
      let rec: { type?: string; id?: string; anchor?: AnyAnchor };
      try {
        rec = JSON.parse(trimmed);
      } catch {
        continue;
      }
      if (rec.type !== "annotation" || !rec.anchor) continue;
      const block = byId.get(rec.anchor.block);
      if (!block) {
        out.push({
          id: rec.id ?? "?",
          anchor: rec.anchor,
          resolution: { ok: false, reason: "block-not-found" },
        });
        continue;
      }
      const res = runtime.klf({ op: "resolveAnchor", block, anchor: rec.anchor });
      const r = (res.resolution as Record<string, unknown>) ?? {};
      out.push({
        id: rec.id ?? "?",
        anchor: rec.anchor,
        resolution: {
          ok: Boolean(r.ok),
          kind: r.kind as string | undefined,
          reason: r.reason as string | undefined,
          detail: describeResolution(r),
        },
      });
    }
    return out;
    // klflValue + blocks identity drive recomputation.
  }, [runtime.ready, runtime, klflValue, blocks, hideAnnotations]);

  const selected = annotations.find((a) => a.id === selectedAnn) ?? null;

  const selectSample = (id: string) => {
    setSampleId(id);
    setKlfValue(klfText(klfSampleById(id).file));
    setSelectedAnn(null);
  };

  return (
    <div className={styles.lab}>
      <div className={styles.row}>
        {KLF_SAMPLES.map((s) => (
          <button
            key={s.id}
            className={`${styles.chip} ${sampleId === s.id ? styles.chipActive : ""}`}
            onClick={() => selectSample(s.id)}
            type="button"
          >
            {s.label}
          </button>
        ))}
      </div>
      <p className={styles.status}>{klfSampleById(sampleId).blurb}</p>

      <div className={styles.section}>
        <span className={styles.label}>.klf document — edit it</span>
        <textarea
          className={`${styles.editor} ${parsed.error ? styles.editorError : ""}`}
          spellCheck={false}
          value={klfValue}
          onChange={(e) => setKlfValue(e.target.value)}
        />
      </div>

      <div className={`${styles.status} ${parsed.error || engineError ? styles.statusError : ""}`}>
        {runtime.status === "booting" && "Booting the kapi engine (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && parsed.error && `Invalid JSON: ${parsed.error}`}
        {runtime.ready && !parsed.error && engineError && `Engine: ${engineError}`}
        {runtime.ready && !parsed.error && !engineError && canonical && (
          <CanonicalBadge canonical={canonical} input={klfValue} />
        )}
      </div>

      {runtime.ready && !parsed.error && !engineError && (
        <div className={styles.blocks}>
          {blocks.map((b) => (
            <BlockCard
              key={b.id}
              block={b}
              analysis={analysis[b.id]}
              highlight={selected && selected.anchor.block === b.id ? selected.anchor : null}
            />
          ))}
        </div>
      )}

      {!hideAnnotations && (
        <div className={styles.section}>
          <span className={styles.label}>.klfl annotation overlay — stand-off anchors</span>
          <textarea
            className={`${styles.editor} ${styles.editorSmall}`}
            spellCheck={false}
            value={klflValue}
            onChange={(e) => setKlflValue(e.target.value)}
          />
          <p className={styles.status}>
            Each record anchors to a location in a block. Click one to resolve it against the live
            document and highlight what it points at — or see the machine-readable reason it failed.
          </p>
          <div className={styles.annList}>
            {annotations.map((a) => (
              <AnnotationRow
                key={a.id}
                ann={a}
                active={selectedAnn === a.id}
                onClick={() => setSelectedAnn(selectedAnn === a.id ? null : a.id)}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function CanonicalBadge({
  canonical,
  input,
}: {
  canonical: { output: string; sha256: string };
  input: string;
}): React.ReactElement {
  const isCanonical = canonical.output === input;
  return (
    <span className={styles.row}>
      <span className={`${styles.badge} ${isCanonical ? styles.badgeOk : styles.badgeMuted}`}>
        {isCanonical ? "already canonical" : "canonicalized"}
      </span>
      <span className={styles.detail}>sha256:{canonical.sha256.slice(0, 16)}…</span>
    </span>
  );
}

function BlockCard({
  block,
  analysis,
  highlight,
}: {
  block: Block;
  analysis?: BlockAnalysis;
  highlight: AnyAnchor | null;
}): React.ReactElement {
  const errors = analysis?.errors ?? [];
  return (
    <div className={styles.card}>
      <div className={styles.cardHeader}>
        <span className={styles.cardId}>{block.id}</span>
        <span className={`${styles.badge} ${styles.badgeType}`}>{block.type}</span>
        {!block.translatable && (
          <span className={`${styles.badge} ${styles.badgeMuted}`}>not translatable</span>
        )}
        <span className={styles.spacer} />
        {errors.length === 0 ? (
          <span className={`${styles.badge} ${styles.badgeOk}`}>valid</span>
        ) : (
          <span className={`${styles.badge} ${styles.badgeErr}`}>
            {errors.length} {errors.length === 1 ? "issue" : "issues"}
          </span>
        )}
      </div>
      <div className={styles.cardBody}>
        <div className={styles.section}>
          <span className={styles.label}>Preview (rendered by the engine)</span>
          {analysis?.html ? (
            // The HTML is produced by the Go engine's RenderBlockHTML (which
            // HTML-escapes text/data/equiv) from a document the user authored in
            // their own browser — a browser-local, self-authored preview with no
            // second party, the same trust model as the @neokapi/ui editor
            // InlinePreview.
            <div
              className={styles.preview}
              dangerouslySetInnerHTML={{ __html: stripBlockWrapper(analysis.html) }}
            />
          ) : (
            <div className={styles.preview}>—</div>
          )}
        </div>

        <div className={styles.section}>
          <span className={styles.label}>Source runs</span>
          <RunsStrip runs={block.source} highlight={highlight} />
        </div>

        {block.placeholders.length > 0 && (
          <div className={styles.section}>
            <span className={styles.label}>Placeholders</span>
            <div className={styles.phList}>
              {block.placeholders.map((p) => (
                <span
                  key={p.name}
                  className={`${styles.phChip} ${p.optional ? styles.phOptional : ""}`}
                >
                  {p.name}
                  <span className={styles.runTextKind}>{p.kind}</span>
                  {p.optional ? " · optional" : ""}
                </span>
              ))}
            </div>
          </div>
        )}

        {errors.length > 0 && (
          <div className={styles.section}>
            <span className={styles.label}>Validation</span>
            <div className={styles.findings}>
              {errors.map((e, i) => (
                <div key={i} className={styles.finding}>
                  <span className={styles.findingKind}>{e.kind}</span>
                  <span>{e.message}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function RunsStrip({
  runs,
  highlight,
}: {
  runs: Run[];
  highlight: AnyAnchor | null;
}): React.ReactElement {
  // Only single-step numeric paths drive highlighting (sufficient for the
  // fixtures); the resolution text always shows the full result regardless.
  const hiIndex =
    highlight &&
    highlight.kind !== "block" &&
    Array.isArray(highlight.path) &&
    typeof highlight.path[0] === "number"
      ? (highlight.path[0] as number)
      : null;
  return (
    <div className={styles.runs}>
      {runs.map((r, i) => (
        <RunChip
          key={i}
          run={r}
          index={i}
          highlighted={hiIndex === i}
          range={
            hiIndex === i && highlight?.kind === "range"
              ? { offset: highlight.offset ?? 0, length: highlight.length ?? 0 }
              : null
          }
          form={hiIndex === i && highlight?.kind === "form" ? (highlight.key ?? null) : null}
        />
      ))}
    </div>
  );
}

function RunChip({
  run,
  index,
  highlighted,
  range,
  form,
}: {
  run: Run;
  index: number;
  highlighted: boolean;
  range: { offset: number; length: number } | null;
  form: string | null;
}): React.ReactElement {
  const hi = highlighted ? styles.runHighlight : "";
  if ("text" in run) {
    return (
      <span className={`${styles.run} ${hi}`}>
        <span className={styles.runIndex}>{index}</span>
        <span className={`${styles.runKind} ${styles.runTextKind}`}>text</span>
        <span className={styles.runText}>
          {range ? markRange(run.text, range) : JSON.stringify(run.text)}
        </span>
      </span>
    );
  }
  if ("ph" in run) {
    return (
      <span className={`${styles.run} ${styles.runPh} ${hi}`}>
        <span className={styles.runIndex}>{index}</span>
        <span className={`${styles.runKind} ${styles.runPhKind}`}>ph · {run.ph.equiv}</span>
        <span className={styles.runText}>{run.ph.data}</span>
      </span>
    );
  }
  if ("pcOpen" in run) {
    return (
      <span className={`${styles.run} ${styles.runPc} ${hi}`}>
        <span className={styles.runIndex}>{index}</span>
        <span className={`${styles.runKind} ${styles.runPcKind}`}>pcOpen · {run.pcOpen.equiv}</span>
        <span className={styles.runText}>{run.pcOpen.data}</span>
      </span>
    );
  }
  if ("pcClose" in run) {
    return (
      <span className={`${styles.run} ${styles.runPc} ${hi}`}>
        <span className={styles.runIndex}>{index}</span>
        <span className={`${styles.runKind} ${styles.runPcKind}`}>pcClose</span>
        <span className={styles.runText}>{run.pcClose.data}</span>
      </span>
    );
  }
  if ("sub" in run) {
    return (
      <span className={`${styles.run} ${styles.runSub} ${hi}`}>
        <span className={styles.runIndex}>{index}</span>
        <span className={`${styles.runKind} ${styles.runSubKind}`}>sub → {run.sub.ref}</span>
        <span className={styles.runText}>{run.sub.equiv}</span>
      </span>
    );
  }
  if ("plural" in run) {
    return (
      <span className={`${styles.run} ${styles.runPlural} ${hi}`}>
        <span className={styles.runIndex}>{index}</span>
        <span className={`${styles.runKind} ${styles.runPluralKind}`}>
          plural · {run.plural.pivot}
        </span>
        <span className={styles.runText}>
          {Object.keys(run.plural.forms)
            .map((k) => (k === form ? `[${k}]` : k))
            .join(" · ")}
        </span>
      </span>
    );
  }
  if ("select" in run) {
    return (
      <span className={`${styles.run} ${styles.runPlural} ${hi}`}>
        <span className={styles.runIndex}>{index}</span>
        <span className={`${styles.runKind} ${styles.runPluralKind}`}>
          select · {run.select.pivot}
        </span>
        <span className={styles.runText}>
          {Object.keys(run.select.cases)
            .map((k) => (k === form ? `[${k}]` : k))
            .join(" · ")}
        </span>
      </span>
    );
  }
  return <span className={styles.run}>?</span>;
}

function AnnotationRow({
  ann,
  active,
  onClick,
}: {
  ann: ParsedAnnotation;
  active: boolean;
  onClick: () => void;
}): React.ReactElement {
  return (
    <div className={`${styles.annItem} ${active ? styles.annItemActive : ""}`} onClick={onClick}>
      <div className={styles.annTop}>
        <span className={styles.annId}>{ann.id}</span>
        <span className={`${styles.badge} ${styles.badgeType}`}>{ann.anchor.kind}</span>
        <span className={styles.annAnchor}>{anchorSummary(ann.anchor)}</span>
        <span className={styles.spacer} />
        {ann.resolution.ok ? (
          <span className={`${styles.badge} ${styles.badgeOk}`}>resolved</span>
        ) : (
          <span className={`${styles.badge} ${styles.badgeErr}`}>{ann.resolution.reason}</span>
        )}
      </div>
      <div className={styles.annResolution}>
        {ann.resolution.ok ? ann.resolution.detail : `did not resolve — ${ann.resolution.reason}`}
      </div>
    </div>
  );
}

// ─── helpers ─────────────────────────────────────────────────────────────

function describeResolution(r: Record<string, unknown>): string {
  switch (r.kind) {
    case "block":
      return "the whole block (block-level metadata)";
    case "run":
      return `run id ${String(r.runId)}`;
    case "range":
      return `“${String(r.rangeText).slice(r.rangeOffset as number, (r.rangeOffset as number) + (r.rangeLength as number))}” at offset ${String(r.rangeOffset)}`;
    case "form":
      return `${String(r.formRunCount)} run(s) in the selected form`;
    default:
      return "";
  }
}

function anchorSummary(a: AnyAnchor): string {
  const path = a.path
    ? `[${a.path.map((p) => (typeof p === "number" ? p : JSON.stringify(p))).join(", ")}]`
    : "";
  switch (a.kind) {
    case "block":
      return a.block;
    case "run":
      return `${a.block} ${path} runId=${a.runId}`;
    case "range":
      return `${a.block} ${path} ${a.offset}+${a.length}`;
    case "form":
      return `${a.block} ${path} key=${a.key}`;
    default:
      return a.block;
  }
}

function markRange(text: string, range: { offset: number; length: number }): React.ReactNode {
  const before = text.slice(0, range.offset);
  const mid = text.slice(range.offset, range.offset + range.length);
  const after = text.slice(range.offset + range.length);
  return (
    <>
      {JSON.stringify(before).slice(1, -1)}
      <mark className={styles.rangeMark}>{mid}</mark>
      {JSON.stringify(after).slice(1, -1)}
    </>
  );
}

// The engine wraps preview HTML in <kat-block …>…</kat-block>; strip the
// wrapper so the inner runs render inline in the preview box.
function stripBlockWrapper(html: string): string {
  return html.replace(/^<kat-block[^>]*>/, "").replace(/<\/kat-block>$/, "");
}
