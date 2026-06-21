import React, { useEffect, useMemo, useState } from "react";
import {
  configurePlugins,
  bootEngine,
  ensurePlugin,
  usePluginManager,
} from "@neokapi/kapi-playground/plugins";
import type { PluginId } from "@neokapi/kapi-playground/plugins";
import type { KapiRuntime } from "@neokapi/kapi-playground";
import type { LabRuntimeAssets } from "@neokapi/kapi-lab";
import { FileSource } from "@neokapi/kapi-lab";
import type { FileSourceValue } from "@neokapi/kapi-lab";
import type { ContentTree, ContentNode, Run } from "@neokapi/ui-primitives/preview";
import { loadICU4X } from "../../lib/icu4x";
import { installIntlSegmenter } from "../../lib/intlSegmenter";

// LLM model keys — must match gemmaBridge's DEFAULT_TEXT_MODEL / MULTIMODAL_MODEL.
// Hardcoded (not imported) so this page doesn't statically pull transformers.js;
// the bridge itself is dynamic-imported only when an LLM engine actually runs.
const TEXT_MODEL_KEY = "llama-3.2-1b";
const MULTIMODAL_MODEL_KEY = "gemma-4-e2b";

// SegmentationLab — one consolidated, framework-native lab. Everything here runs
// INSIDE neokapi's content model, never around it:
//
//   • The source becomes a real document. A sample or your own text is written
//     as a single-block `.txt`; an uploaded file goes through its normal reader,
//     so a `.json` / `.html` / `.xliff` yields its translatable blocks (not its
//     markup). Both are parsed by the engine into Blocks.
//   • Each engine produces a real segmentation OVERLAY on those blocks. The
//     rule/Unicode engines (SRX, UAX-29, Hybrid, Intl.Segmenter) run through
//     `inspectAnnotated(segment:true)`, so the sentences are the actual
//     stand-off `segmentation` overlay spans the engine attached. SaT and the
//     local LLMs (which have no in-wasm engine) segment each block's source text
//     and the same overlay shape is built back onto the block.
//   • The columns render those blocks with their sentences, and the comparison
//     is over the overlays themselves — where engines drew a boundary at the
//     same place in the same block. There is no "correct" engine and no
//     ground-truth diff; agreement is the only signal. Engine names match the
//     CLI and the flow editor.

interface SampleText {
  id: string;
  label: string;
  text: string;
}

const SAMPLES: SampleText[] = [
  {
    id: "abbrev",
    label: "Abbreviations & decimals",
    text:
      "Dr. Smith paid $3.50 for the U.S. edition on Jan. 5, 2024. " +
      "Mr. Lee asked, “Is it ready?” It was. The next batch ships at 9 a.m.",
  },
  {
    id: "dialogue",
    label: "Dialogue & quotes",
    text: "“Are you sure?” she asked. “Quite sure,” he replied. They left at noon.",
  },
  {
    id: "cjk",
    label: "Japanese (no spaces)",
    text: "今日はいい天気ですね。明日は雨が降るでしょう。傘を持って行きましょう。",
  },
  {
    id: "paragraphs",
    label: "Multiple paragraphs",
    text: "The first sentence. The second one!\n\nA new paragraph begins here. And it ends.",
  },
];

type EngineKind = "engine" | "sat" | "llm";

interface EngineDef {
  id: string;
  label: string;
  kind: EngineKind;
  /** Segmentation engine name passed to `inspectAnnotated` (engine kind only). */
  engineArg?: string;
  /** Plugin to reflect/download in the navbar widget (sat). */
  plugin?: PluginId;
  /** LLM model key to run (llm kind). */
  modelKey?: string;
  /** Approximate download, for the selector chip (engines that fetch a model). */
  sizeBytes?: number;
  note: string;
}

const ENGINES: EngineDef[] = [
  {
    id: "srx",
    label: "SRX rules",
    kind: "engine",
    engineArg: "",
    note: "Fast rule-based splitting.",
  },
  {
    id: "uax29",
    label: "UAX-29",
    kind: "engine",
    engineArg: "uax29",
    note: "The Unicode standard's sentence rules.",
  },
  {
    id: "hybrid",
    label: "Hybrid",
    kind: "engine",
    engineArg: "hybrid",
    note: "The Unicode rules, refined for real-world abbreviations.",
  },
  {
    id: "intl",
    label: "Intl.Segmenter",
    kind: "engine",
    engineArg: "intl",
    note: "The browser's built-in segmenter (zero download).",
  },
  {
    id: "sat",
    label: "SaT (ML)",
    kind: "sat",
    plugin: "sat",
    sizeBytes: 428_000_000,
    note: "A small machine-learning model trained to find sentence breaks.",
  },
  {
    id: "llm-llama",
    label: "LLM · Llama 3.2 1B",
    kind: "llm",
    modelKey: TEXT_MODEL_KEY,
    sizeBytes: 900_000_000,
    note: "A small local language model asked to split the text.",
  },
  {
    id: "llm-gemma",
    label: "LLM · Gemma 4 E2B",
    kind: "llm",
    modelKey: MULTIMODAL_MODEL_KEY,
    sizeBytes: 6_800_000_000,
    note: "A larger multimodal local model asked to split the text.",
  },
];

// Engines on by default: the instant ones (no download). Heavy models are opt-in.
const DEFAULT_SELECTED = new Set(["srx", "uax29", "hybrid", "intl"]);

interface DlProgress {
  loaded?: number;
  total?: number;
  frac?: number;
}

/** A translatable block of the parsed input, shared across every engine. */
interface BaseBlock {
  id: string;
  name?: string;
  /** The block's source text (run sequence flattened to plain text). */
  text: string;
}

/** One engine's segmentation of a single block — the overlay, viewed per block. */
interface BlockSeg {
  id: string;
  name?: string;
  text: string;
  /** Sentence texts from the engine's `segmentation` overlay spans. */
  sentences: string[];
  /** Boundary offsets (code points) within `text`, or null when the engine's
   *  output can't be mapped back (e.g. an LLM reworded it) — shown, but kept out
   *  of the agreement layer rather than misattributed. */
  cuts: number[] | null;
}

interface EngineResult {
  blocks: BlockSeg[];
  ms: number;
  /** Total sentences across all blocks. */
  total: number;
}

function fmtBytes(n?: number): string {
  if (!n) return "";
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)} GB`;
  if (n >= 1e6) return `${Math.round(n / 1e6)} MB`;
  return `${Math.round(n / 1e3)} KB`;
}

// runsText flattens a Block's run sequence to plain text — concatenating the
// text runs and skipping inline codes — matching what the segmentation overlay
// covers. (Plain-text samples have no codes; richer files lose only the inline
// placeholders, which segmentation does not split on.)
function runsText(runs?: Run[]): string {
  if (!runs) return "";
  let s = "";
  for (const r of runs) if (typeof r.text === "string") s += r.text;
  return s;
}

// flattenBlocks walks a ContentTree depth-first and returns its Block nodes in
// document order — the same blocks every engine segments.
function flattenBlocks(tree: ContentTree): ContentNode[] {
  const out: ContentNode[] = [];
  const walk = (nodes?: ContentNode[]) => {
    for (const n of nodes ?? []) {
      if (n.kind === "block") out.push(n);
      walk(n.children);
    }
  };
  walk(tree.root);
  return out;
}

const isSpace = (c: string) => /\s/.test(c);

// cutOffsets maps an engine's sentence texts back to boundary offsets (code-point
// indices) within `source`, tolerant of whitespace differences, so the agreement
// layer can place markers on the shared block text. Returns null when the
// sentences don't map back (an LLM that reworded/normalised the text).
function cutOffsets(source: string, sentences: string[]): number[] | null {
  const cps = Array.from(source);
  let pos = 0;
  const cuts: number[] = [];
  const trimmed = sentences.map((s) => s.trim()).filter(Boolean);
  for (let i = 0; i < trimmed.length; i++) {
    const sc = Array.from(trimmed[i]);
    while (pos < cps.length && isSpace(cps[pos])) pos++;
    let j = 0;
    while (pos < cps.length && j < sc.length) {
      if (isSpace(sc[j])) {
        j++;
        continue;
      }
      if (isSpace(cps[pos])) {
        pos++;
        continue;
      }
      if (cps[pos] === sc[j]) {
        pos++;
        j++;
      } else {
        return null;
      }
    }
    if (j < sc.length) return null;
    if (i < trimmed.length - 1) {
      let cut = pos;
      while (cut < cps.length && isSpace(cps[cut])) cut++;
      cuts.push(cut);
    }
  }
  return cuts;
}

// llmSentences runs a local LLM on one block's text and parses one-sentence-per
// line output back to a sentence list.
function parseLLMLines(out: string): string[] {
  return out
    .split("\n")
    .map((l) => l.replace(/^\s*(?:\d+[.)]\s*|[-*]\s*)?/, "").trim())
    .filter(Boolean);
}

function DownloadBar({ p }: { p?: DlProgress | null }): React.ReactElement | null {
  if (!p) return null;
  const frac = p.frac ?? (p.total ? (p.loaded ?? 0) / p.total : 0);
  const pct = Math.round(Math.min(1, Math.max(0, frac)) * 100);
  return (
    <div className="mb-2">
      <div className="h-2 w-full overflow-hidden rounded bg-muted">
        <div className="h-full bg-primary transition-all" style={{ width: `${pct}%` }} />
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        Downloading model · {pct}%
        {p.total ? ` · ${fmtBytes(p.loaded)} of ${fmtBytes(p.total)}` : ""}
      </p>
    </div>
  );
}

// BlockAgreement renders one block's source text once, with a marker at every
// boundary any engine's overlay drew; the marker's strength = how many of the
// mapped engines split there. Neutral: more engines agreeing = a stronger mark,
// never "correct".
function BlockAgreement({
  text,
  cutsByEngine,
  labels,
}: {
  text: string;
  cutsByEngine: Record<string, number[] | null>;
  labels: Record<string, string>;
}): React.ReactElement | null {
  const mapped = Object.entries(cutsByEngine).filter(([, c]) => c !== null) as [string, number[]][];
  if (mapped.length < 2 || !text) return null;

  const cps = Array.from(text);
  const counts = new Map<number, number>();
  for (const [, c] of mapped) for (const off of c) counts.set(off, (counts.get(off) ?? 0) + 1);
  const offsets = [...counts.keys()].sort((a, b) => a - b);
  const total = mapped.length;

  const pieces: React.ReactNode[] = [];
  let prev = 0;
  offsets.forEach((off, k) => {
    pieces.push(<span key={`t${k}`}>{cps.slice(prev, off).join("")}</span>);
    const n = counts.get(off) ?? 0;
    const strength = n / total;
    pieces.push(
      <span
        key={`m${k}`}
        title={`${n} of ${total} engines split here`}
        className="mx-0.5 inline-block align-middle rounded-sm"
        style={{
          width: 3,
          height: "1em",
          backgroundColor: `color-mix(in srgb, var(--ifm-color-primary) ${Math.round(
            30 + strength * 70,
          )}%, transparent)`,
        }}
      />,
    );
    prev = off;
  });
  pieces.push(<span key="tlast">{cps.slice(prev).join("")}</span>);

  // Cluster engines whose boundary sets are identical for this block.
  const groups = new Map<string, string[]>();
  for (const [id, c] of mapped) {
    const key = JSON.stringify(c);
    (groups.get(key) ?? groups.set(key, []).get(key)!).push(labels[id] ?? id);
  }
  const clusters = [...groups.values()].filter((g) => g.length > 1);

  return (
    <div>
      <p className="whitespace-pre-wrap text-sm leading-7 text-foreground">{pieces}</p>
      {clusters.length > 0 && (
        <ul className="mt-2 flex flex-col gap-0.5 text-xs text-muted-foreground">
          {clusters.map((g, i) => (
            <li key={i}>
              Same split: <span className="text-foreground">{g.join(", ")}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// AgreementPanel frames the per-block agreement views. Driven entirely from the
// overlays each engine attached — agreement is the only signal, not a verdict.
function AgreementPanel({
  blocks,
  results,
  selectedIds,
  labels,
}: {
  blocks: BaseBlock[];
  results: Record<string, EngineResult>;
  selectedIds: string[];
  labels: Record<string, string>;
}): React.ReactElement | null {
  const cutsFor = (blockId: string): Record<string, number[] | null> => {
    const m: Record<string, number[] | null> = {};
    for (const id of selectedIds) {
      const er = results[id];
      if (!er) continue;
      const bs = er.blocks.find((b) => b.id === blockId);
      if (bs) m[id] = bs.cuts;
    }
    return m;
  };

  const panels = blocks
    .map((b) => ({ block: b, cuts: cutsFor(b.id) }))
    .filter(({ cuts }) => Object.values(cuts).filter((c) => c !== null).length >= 2);
  if (panels.length === 0) return null;

  const multi = blocks.length > 1;
  return (
    <div className="rounded-lg border border-border bg-card/40 p-3">
      <div className="mb-2 flex items-baseline justify-between gap-2">
        <span className="text-sm font-semibold">Where the engines agree</span>
        <span className="text-xs text-muted-foreground">
          Darker mark = more engines drew that boundary. Not a verdict — there is no single correct
          segmentation.
        </span>
      </div>
      <div className="flex flex-col gap-3">
        {panels.map(({ block, cuts }, i) => (
          <div key={block.id}>
            {multi && (
              <div className="mb-1 font-mono text-xs text-muted-foreground">
                {block.name || `block ${i + 1}`}
              </div>
            )}
            <BlockAgreement text={block.text} cutsByEngine={cuts} labels={labels} />
          </div>
        ))}
      </div>
    </div>
  );
}

function EngineColumn({
  def,
  result,
  busy,
  error,
  progress,
  showBlockLabels,
}: {
  def: EngineDef;
  result: EngineResult | null;
  busy: boolean;
  error: string | null;
  progress?: DlProgress | null;
  showBlockLabels: boolean;
}): React.ReactElement {
  return (
    <div className="flex w-64 shrink-0 flex-col rounded-lg border border-border p-3">
      <div className="mb-0.5 text-sm font-semibold">{def.label}</div>
      <p className="mb-2 text-xs text-muted-foreground">{def.note}</p>
      {busy && <DownloadBar p={progress} />}
      {error ? (
        <p className="text-sm text-destructive">{error}</p>
      ) : busy && !result ? (
        <p className="text-sm text-muted-foreground">Running…</p>
      ) : !result ? (
        <p className="text-sm text-muted-foreground">Not run.</p>
      ) : (
        <div className="flex flex-col gap-2">
          {result.blocks.map((b) => (
            <div key={b.id} className="flex flex-col gap-1">
              {showBlockLabels && (
                <div className="font-mono text-[0.7rem] text-muted-foreground">
                  {b.name || b.id}
                </div>
              )}
              {b.sentences.map((s, i) => (
                <div
                  key={i}
                  className="flex gap-2 rounded border border-border bg-card/40 px-2 py-1 text-sm"
                >
                  <span className="select-none font-mono text-xs text-muted-foreground">
                    {i + 1}
                  </span>
                  <span className="text-foreground">{s}</span>
                </div>
              ))}
            </div>
          ))}
          <p className="text-xs text-muted-foreground">
            {result.total} sentence{result.total === 1 ? "" : "s"}
            {result.blocks.length > 1 ? ` · ${result.blocks.length} blocks` : ""} · {result.ms}
            &nbsp;ms
          </p>
        </div>
      )}
    </div>
  );
}

export interface SegmentationLabInnerProps {
  assets: LabRuntimeAssets | null;
}

export default function SegmentationLabInner({
  assets,
}: SegmentationLabInnerProps): React.ReactElement {
  const mgr = usePluginManager();
  const [text, setText] = useState(SAMPLES[0].text);
  const [locale, setLocale] = useState("en");
  const [file, setFile] = useState<FileSourceValue | null>(null);

  const [selected, setSelected] = useState<Set<string>>(new Set(DEFAULT_SELECTED));
  const [results, setResults] = useState<Record<string, EngineResult>>({});
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [errors, setErrors] = useState<Record<string, string | null>>({});
  const [dl, setDl] = useState<Record<string, DlProgress | null>>({});
  const [comparedBlocks, setComparedBlocks] = useState<BaseBlock[]>([]);
  const [running, setRunning] = useState(false);

  // Configure the shared plugin manager (idempotent) so bootEngine + ensurePlugin
  // reach the same engine/models the navbar widget uses. No boot here — only Run.
  useEffect(() => {
    if (assets) configurePlugins(assets);
  }, [assets]);

  // When a file is chosen it becomes the source; the textarea is cleared so the
  // file's blocks (not its raw markup) are what gets parsed and segmented.
  const onFile = (v: FileSourceValue) => {
    setFile(v);
    setText("");
  };

  const toggleEngine = (id: string) =>
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  // Write the source into the engine's in-memory filesystem as a real document
  // and parse it into Blocks. A sample / typed text becomes a single-block
  // `.txt`; a file keeps its extension so its own reader runs. The parsed blocks
  // are the shared unit every engine segments.
  async function buildInput(rt: KapiRuntime): Promise<{ path: string; blocks: BaseBlock[] }> {
    rt.vol.mkdirp("/seg");
    let path: string;
    if (file?.bytes && file.bytes.length > 0) {
      const ext = file.filename.match(/\.[A-Za-z0-9]+$/)?.[0] ?? "";
      path = `/seg/input${ext}`;
      rt.vol.writeFile(path, file.bytes);
    } else {
      path = "/seg/input.txt";
      rt.vol.writeFile(path, new TextEncoder().encode(text));
    }
    const res = await rt.inspect(path);
    if (!res.ok || !res.tree) throw new Error(res.error ?? "could not read the input");
    const blocks = flattenBlocks(res.tree as ContentTree)
      .map((b) => ({ id: b.id, name: b.name, text: runsText(b.source) }))
      .filter((b) => b.text.trim().length > 0);
    if (blocks.length === 0) throw new Error("the input has no translatable text");
    return { path, blocks };
  }

  // Run one engine over the parsed blocks, returning its per-block segmentation.
  // Rule/Unicode engines read the real `segmentation` overlay produced by
  // inspectAnnotated; SaT / LLM segment each block's text and the same shape is
  // reconstructed onto the block.
  async function runEngine(
    def: EngineDef,
    rt: KapiRuntime,
    path: string,
    base: BaseBlock[],
  ): Promise<BlockSeg[]> {
    if (def.kind === "engine") {
      if (def.engineArg === "uax29" || def.engineArg === "hybrid") await loadICU4X();
      if (def.engineArg === "intl") installIntlSegmenter();
      const res = await rt.inspectAnnotated(path, {
        segment: true,
        segmentEngine: def.engineArg ?? "",
        term: false,
        brand: false,
        qa: false,
      });
      if (!res.ok || !res.tree) throw new Error(res.error ?? "segmentation failed");
      const byId = new Map(flattenBlocks(res.tree as ContentTree).map((n) => [n.id, n]));
      return base.map((b) => {
        const node = byId.get(b.id);
        const overlay = node?.overlays?.find(
          (o) => o.type === "segmentation" && o.side === "source",
        );
        // No overlay (or a single span) means the engine found no internal
        // boundary: the whole block is one sentence.
        const sentences =
          overlay && overlay.spans.length > 0 ? overlay.spans.map((s) => s.text ?? "") : [b.text];
        return { ...b, sentences, cuts: cutOffsets(b.text, sentences) };
      });
    }

    if (def.kind === "sat") {
      await ensurePlugin("sat");
      const { segmentSat } = await import("@neokapi/kapi-playground/satBridge");
      const out: BlockSeg[] = [];
      for (const b of base) {
        const sentences = (await segmentSat(b.text)).sentences;
        out.push({ ...b, sentences, cuts: cutOffsets(b.text, sentences) });
      }
      return out;
    }

    // llm — load the SPECIFIC model for this column, with its own progress, then
    // segment each block's text.
    const modelKey = def.modelKey ?? TEXT_MODEL_KEY;
    const { ensureLLM, generateLLMText } = await import("@neokapi/kapi-playground/gemmaBridge");
    await ensureLLM(modelKey, {
      onProgress: (p) => setDl((d) => ({ ...d, [def.id]: { loaded: p.loaded, total: p.total } })),
    });
    const out: BlockSeg[] = [];
    for (const b of base) {
      const prompt =
        "You split text into sentences. Copy the input EXACTLY — do not reword, " +
        "rephrase, translate, expand abbreviations, or change any characters — and put " +
        "each complete sentence on its own line, with nothing else.\n\n" +
        "Example input: The cat sat. It was happy! Then it left.\n" +
        "Example output:\nThe cat sat.\nIt was happy!\nThen it left.\n\n" +
        "Now do the same for this input:\n" +
        b.text;
      const raw = await generateLLMText(prompt, modelKey, { maxTokens: 512, temperature: 0 });
      const sentences = parseLLMLines(raw);
      out.push({ ...b, sentences, cuts: cutOffsets(b.text, sentences) });
    }
    return out;
  }

  const runCompare = () => {
    const defs = ENGINES.filter((d) => selected.has(d.id));
    const hasSource = file?.bytes?.length || text.trim();
    if (defs.length === 0 || !hasSource) return;
    setRunning(true);
    setResults({});
    setErrors({});
    setComparedBlocks([]);
    setBusy(Object.fromEntries(defs.map((d) => [d.id, true])));
    setDl({});

    void (async () => {
      let rt: KapiRuntime;
      let path: string;
      let base: BaseBlock[];
      try {
        rt = (await bootEngine()) as KapiRuntime;
        ({ path, blocks: base } = await buildInput(rt));
      } catch (err) {
        // Couldn't even read the input — surface it on every selected engine.
        const msg = err instanceof Error ? err.message : String(err);
        setErrors(Object.fromEntries(defs.map((d) => [d.id, msg])));
        setBusy({});
        setRunning(false);
        return;
      }
      setComparedBlocks(base);
      await Promise.allSettled(
        defs.map(async (def) => {
          const t0 = performance.now();
          try {
            const blocks = await runEngine(def, rt, path, base);
            const total = blocks.reduce((n, b) => n + b.sentences.length, 0);
            setResults((r) => ({
              ...r,
              [def.id]: { blocks, total, ms: Math.round(performance.now() - t0) },
            }));
          } catch (err) {
            setErrors((e) => ({
              ...e,
              [def.id]: err instanceof Error ? err.message : String(err),
            }));
          } finally {
            setBusy((b) => ({ ...b, [def.id]: false }));
            setDl((d) => ({ ...d, [def.id]: null }));
          }
        }),
      );
      setRunning(false);
    })();
  };

  const hasResults = Object.keys(results).length > 0;
  const selectedDefs = ENGINES.filter((d) => selected.has(d.id));
  const selectedIds = useMemo(() => selectedDefs.map((d) => d.id), [selectedDefs]);
  const labels = useMemo(() => Object.fromEntries(ENGINES.map((d) => [d.id, d.label])), []);
  const multiBlock = comparedBlocks.length > 1;

  // Download progress per engine: sat reflects the shared manager state (also
  // shown in the navbar widget); the LLM models report locally via onProgress.
  const progressFor = (d: EngineDef): DlProgress | null => {
    if (d.kind === "sat") {
      const st = mgr.state.plugins.sat;
      if (st?.phase !== "downloading") return null;
      return st.progress
        ? { loaded: st.progress.loaded, total: st.progress.total, frac: st.progress.frac }
        : { frac: 0 };
    }
    return dl[d.id] ?? null;
  };

  const downloadResults = () => {
    const payload = {
      locale,
      blocks: comparedBlocks.map((b) => ({ id: b.id, name: b.name, text: b.text })),
      engines: selectedDefs
        .filter((d) => results[d.id])
        .map((d) => ({
          engine: d.id,
          blocks: results[d.id].blocks.map((b) => ({ id: b.id, sentences: b.sentences })),
        })),
    };
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "segmentation.json";
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="kapi-reference flex flex-col gap-4 text-foreground">
      {/* Header + single download */}
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          Pick a source, choose the engines to compare, and run them on the same document — neokapi
          parses it into blocks and each engine attaches its sentences as a stand-off segmentation
          overlay.
        </p>
        <button
          type="button"
          onClick={downloadResults}
          disabled={!hasResults}
          className="shrink-0 rounded border border-border px-3 py-1.5 text-sm hover:bg-muted/60 disabled:opacity-40"
        >
          Download results
        </button>
      </div>

      {/* Source: quick samples · file · editable text */}
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-muted-foreground">Quick samples:</span>
          {SAMPLES.map((s) => (
            <button
              key={s.id}
              type="button"
              onClick={() => {
                setText(s.text);
                setFile(null);
              }}
              className="rounded-full border border-border px-2.5 py-1 text-xs hover:bg-muted/60"
            >
              {s.label}
            </button>
          ))}
          <span className="ml-auto flex items-center gap-1">
            <label className="text-sm text-muted-foreground" htmlFor="seg-locale">
              Locale
            </label>
            <input
              id="seg-locale"
              value={locale}
              onChange={(e) => setLocale(e.target.value)}
              className="w-16 rounded border border-border bg-background px-2 py-1 text-sm"
            />
          </span>
        </div>
        <FileSource value={file} onChange={onFile} label="Or a file" />
        <textarea
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            setFile(null);
          }}
          rows={4}
          placeholder={file ? "Using the file above — type here to switch back to text." : ""}
          className="w-full rounded border border-border bg-background p-2 text-sm"
          aria-label="Text to segment"
        />
        {file && (
          <p className="text-xs text-muted-foreground">
            Source: <span className="text-foreground">{file.label}</span> — parsed by its own reader
            into translatable blocks.
          </p>
        )}
      </div>

      {/* Engine selection + Run */}
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-muted-foreground">Compare:</span>
          {ENGINES.map((d) => {
            const on = selected.has(d.id);
            return (
              <button
                key={d.id}
                type="button"
                onClick={() => toggleEngine(d.id)}
                aria-pressed={on}
                className={`rounded-full border px-2.5 py-1 text-xs ${
                  on
                    ? "border-primary bg-primary/10 text-foreground"
                    : "border-border text-muted-foreground hover:bg-muted/60"
                }`}
              >
                {d.label}
                {d.sizeBytes ? (
                  <span className="ml-1 text-[0.65rem] text-muted-foreground">
                    {fmtBytes(d.sizeBytes)}
                  </span>
                ) : null}
              </button>
            );
          })}
          <button
            type="button"
            onClick={runCompare}
            disabled={running || selected.size === 0 || !(file?.bytes?.length || text.trim())}
            className="ml-auto rounded-md bg-primary px-4 py-1.5 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
          >
            {running
              ? "Running…"
              : `▶ Run ${selected.size} engine${selected.size === 1 ? "" : "s"}`}
          </button>
        </div>
      </div>

      {/* Agreement layer (neutral consensus over the overlays, per block) */}
      {hasResults && (
        <AgreementPanel
          blocks={comparedBlocks}
          results={results}
          selectedIds={selectedIds}
          labels={labels}
        />
      )}

      {/* Side-by-side columns, one per selected engine */}
      {(hasResults || running) && (
        <div className="flex gap-3 overflow-x-auto pb-2">
          {selectedDefs.map((d) => (
            <EngineColumn
              key={d.id}
              def={d}
              result={results[d.id] ?? null}
              busy={!!busy[d.id]}
              error={errors[d.id] ?? null}
              progress={progressFor(d)}
              showBlockLabels={multiBlock}
            />
          ))}
        </div>
      )}
    </div>
  );
}
