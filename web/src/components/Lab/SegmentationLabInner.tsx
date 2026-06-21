import React, { useEffect, useMemo, useState } from "react";
import {
  configurePlugins,
  bootEngine,
  ensurePlugin,
  usePluginManager,
} from "@neokapi/kapi-playground/plugins";
import type { PluginId } from "@neokapi/kapi-playground/plugins";
import type { LabRuntimeAssets } from "@neokapi/kapi-lab";
import { FileSource } from "@neokapi/kapi-lab";
import type { FileSourceValue } from "@neokapi/kapi-lab";
import { loadICU4X } from "../../lib/icu4x";
import { installIntlSegmenter } from "../../lib/intlSegmenter";

// LLM model keys — must match gemmaBridge's DEFAULT_TEXT_MODEL / MULTIMODAL_MODEL.
// Hardcoded (not imported) so this page doesn't statically pull transformers.js;
// the bridge itself is dynamic-imported only when an LLM engine actually runs.
const TEXT_MODEL_KEY = "llama-3.2-1b";
const MULTIMODAL_MODEL_KEY = "gemma-4-e2b";

// SegmentationLab — one consolidated, neokapi-centric lab. Segmentation is a
// stand-off overlay on the content model: the same source, split into sentences
// by whichever engines you pick. Choose a source (sample, file, or your own
// text), select the engines to compare, press Run once, and read the engines
// SIDE BY SIDE — with a neutral "agreement" layer showing where the engines cut
// at the same place. There is no single correct segmentation; this is a
// comparison, not a verdict. Engine names match the CLI / flow editor.

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
  /** Argument passed to the wasm `segment` endpoint (engine kind only). */
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

interface EngineResult {
  sentences: string[];
  ms: number;
  /** Cut offsets (code points) of this engine's boundaries against the source,
   *  or null when the output can't be mapped back (e.g. an LLM reworded it). */
  cuts: number[] | null;
}

// Minimal shape of the booted runtime's synchronous segment endpoint.
interface SegmentRuntime {
  segment: (
    text: string,
    engine: string,
    locale: string,
  ) => { ok: boolean; segments?: { text: string }[]; error?: string };
}

function fmtBytes(n?: number): string {
  if (!n) return "";
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)} GB`;
  if (n >= 1e6) return `${Math.round(n / 1e6)} MB`;
  return `${Math.round(n / 1e3)} KB`;
}

const isSpace = (c: string) => /\s/.test(c);

// cutOffsets reconstructs an engine's sentence-boundary offsets (code-point
// indices into `source`) from its returned sentences, tolerant of whitespace
// differences. Returns null if the sentences don't map back to the source (an
// LLM that reworded/normalised the text), so such an engine is shown but left
// out of the agreement layer rather than misattributed.
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

// AgreementStrip renders the source once, with a marker at every boundary any
// engine produced; the marker's strength = how many of the mapped engines split
// there. Neutral: more engines agreeing = a stronger mark, never "correct".
function AgreementStrip({
  source,
  results,
  labels,
}: {
  source: string;
  results: Record<string, EngineResult>;
  labels: Record<string, string>;
}): React.ReactElement | null {
  const mapped = Object.entries(results).filter(([, r]) => r.cuts !== null);
  if (mapped.length < 2 || !source) return null;

  const cps = Array.from(source);
  // Count engines splitting at each offset.
  const counts = new Map<number, number>();
  for (const [, r] of mapped) for (const c of r.cuts!) counts.set(c, (counts.get(c) ?? 0) + 1);
  const offsets = [...counts.keys()].sort((a, b) => a - b);
  const total = mapped.length;

  // Build chunks of text separated by markers at each offset.
  const pieces: React.ReactNode[] = [];
  let prev = 0;
  offsets.forEach((off, k) => {
    pieces.push(<span key={`t${k}`}>{cps.slice(prev, off).join("")}</span>);
    const n = counts.get(off) ?? 0;
    const strength = n / total; // 0..1
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

  // Cluster engines whose boundary sets are identical.
  const groups = new Map<string, string[]>();
  for (const [id, r] of mapped) {
    const key = JSON.stringify(r.cuts);
    (groups.get(key) ?? groups.set(key, []).get(key)!).push(labels[id] ?? id);
  }
  const clusters = [...groups.values()].filter((g) => g.length > 1);

  return (
    <div className="rounded-lg border border-border bg-card/40 p-3">
      <div className="mb-2 flex items-baseline justify-between gap-2">
        <span className="text-sm font-semibold">Where the engines agree</span>
        <span className="text-xs text-muted-foreground">
          Darker mark = more of the {total} engines split there. Not a verdict — there is no single
          correct segmentation.
        </span>
      </div>
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

function EngineColumn({
  def,
  result,
  busy,
  error,
  progress,
}: {
  def: EngineDef;
  result: EngineResult | null;
  busy: boolean;
  error: string | null;
  progress?: DlProgress | null;
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
        <div className="flex flex-col gap-1.5">
          <div className="flex flex-col gap-1">
            {result.sentences.map((s, i) => (
              <div
                key={i}
                className="flex gap-2 rounded border border-border bg-card/40 px-2 py-1 text-sm"
              >
                <span className="select-none font-mono text-xs text-muted-foreground">{i + 1}</span>
                <span className="text-foreground">{s}</span>
              </div>
            ))}
          </div>
          <p className="text-xs text-muted-foreground">
            {result.sentences.length} sentence{result.sentences.length === 1 ? "" : "s"} ·{" "}
            {result.ms}&nbsp;ms
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
  const [comparedText, setComparedText] = useState("");
  const [running, setRunning] = useState(false);

  // Configure the shared plugin manager (idempotent) so bootEngine + ensurePlugin
  // reach the same engine/models the navbar widget uses. No boot here — only Run.
  useEffect(() => {
    if (assets) configurePlugins(assets);
  }, [assets]);

  // When a file is chosen, its text becomes the source.
  const onFile = (v: FileSourceValue) => {
    setFile(v);
    setText(v.content);
  };

  const toggleEngine = (id: string) =>
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  async function runEngine(def: EngineDef, src: string): Promise<string[]> {
    if (def.kind === "engine") {
      if (def.engineArg === "uax29" || def.engineArg === "hybrid") await loadICU4X();
      if (def.engineArg === "intl") installIntlSegmenter();
      const rt = (await bootEngine()) as unknown as SegmentRuntime;
      const res = rt.segment(src, def.engineArg ?? "", locale);
      if (!res.ok) throw new Error(res.error ?? "segmentation failed");
      return (res.segments ?? []).map((s) => s.text);
    }
    if (def.kind === "sat") {
      await ensurePlugin("sat");
      const { segmentSat } = await import("@neokapi/kapi-playground/satBridge");
      return (await segmentSat(src)).sentences;
    }
    // llm — load the SPECIFIC model for this column, with its own progress.
    const modelKey = def.modelKey ?? TEXT_MODEL_KEY;
    const { ensureLLM, generateLLMText } = await import("@neokapi/kapi-playground/gemmaBridge");
    await ensureLLM(modelKey, {
      onProgress: (p) => setDl((d) => ({ ...d, [def.id]: { loaded: p.loaded, total: p.total } })),
    });
    const prompt =
      "You split text into sentences. Copy the input EXACTLY — do not reword, " +
      "rephrase, translate, expand abbreviations, or change any characters — and put " +
      "each complete sentence on its own line, with nothing else.\n\n" +
      "Example input: The cat sat. It was happy! Then it left.\n" +
      "Example output:\nThe cat sat.\nIt was happy!\nThen it left.\n\n" +
      "Now do the same for this input:\n" +
      src;
    const out = await generateLLMText(prompt, modelKey, { maxTokens: 512, temperature: 0 });
    return out
      .split("\n")
      .map((l) => l.replace(/^\s*(?:\d+[.)]\s*|[-*]\s*)?/, "").trim())
      .filter(Boolean);
  }

  const runCompare = () => {
    const src = text;
    const defs = ENGINES.filter((d) => selected.has(d.id));
    if (defs.length === 0 || !src.trim()) return;
    setComparedText(src);
    setRunning(true);
    setResults({});
    setErrors({});
    setBusy(Object.fromEntries(defs.map((d) => [d.id, true])));
    setDl({});
    void Promise.allSettled(
      defs.map(async (def) => {
        const t0 = performance.now();
        try {
          const sentences = await runEngine(def, src);
          setResults((r) => ({
            ...r,
            [def.id]: {
              sentences,
              ms: Math.round(performance.now() - t0),
              cuts: cutOffsets(src, sentences),
            },
          }));
        } catch (err) {
          setErrors((e) => ({ ...e, [def.id]: err instanceof Error ? err.message : String(err) }));
        } finally {
          setBusy((b) => ({ ...b, [def.id]: false }));
          setDl((d) => ({ ...d, [def.id]: null }));
        }
      }),
    ).finally(() => setRunning(false));
  };

  const hasResults = Object.keys(results).length > 0;
  const selectedDefs = ENGINES.filter((d) => selected.has(d.id));
  const labels = useMemo(() => Object.fromEntries(ENGINES.map((d) => [d.id, d.label])), []);

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
      text: comparedText || text,
      locale,
      engines: selectedDefs
        .filter((d) => results[d.id])
        .map((d) => ({ engine: d.id, sentences: results[d.id].sentences })),
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
          Pick a source, choose the engines to compare, and run them on the same text — neokapi
          segments it into sentences as a stand-off overlay on the content model.
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
          className="w-full rounded border border-border bg-background p-2 text-sm"
          aria-label="Text to segment"
        />
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
            disabled={running || selected.size === 0 || !text.trim()}
            className="ml-auto rounded-md bg-primary px-4 py-1.5 text-sm font-medium text-primary-foreground hover:opacity-90 disabled:opacity-50"
          >
            {running
              ? "Running…"
              : `▶ Run ${selected.size} engine${selected.size === 1 ? "" : "s"}`}
          </button>
        </div>
      </div>

      {/* Agreement layer (neutral consensus) */}
      {hasResults && <AgreementStrip source={comparedText} results={results} labels={labels} />}

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
            />
          ))}
        </div>
      )}
    </div>
  );
}
