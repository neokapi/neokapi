import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  configurePlugins,
  bootEngine,
  ensurePlugin,
  usePluginManager,
} from "@neokapi/kapi-playground/plugins";
import type { PluginState, PluginId } from "@neokapi/kapi-playground/plugins";
import type { LabRuntimeAssets } from "@neokapi/kapi-lab";
import { loadICU4X } from "../../lib/icu4x";
import { installIntlSegmenter } from "../../lib/intlSegmenter";

// SegmentationLab — one consolidated lab demonstrating how neokapi segments a
// source into sentences (the stand-off segmentation overlay on the content
// model), comparing every engine on the SAME input you provide. The rule-based
// (SRX), Unicode (UAX-29 / Intl.Segmenter) and learned (SaT / LLM) segmenters all
// run in the browser on your own text; each shows its sentences inline so the
// boundaries are visible at a glance, no drill-down. The engine names match the
// ones the CLI and flow editor expose (srx / uax29 / hybrid / intl / sat / llm).

interface SampleText {
  id: string;
  label: string;
  text: string;
}

const SAMPLES: SampleText[] = [
  {
    id: "abbrev",
    label: "English — abbreviations, decimals, dates",
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
    label: "Japanese (no spaces between sentences)",
    text: "今日はいい天気ですね。明日は雨が降るでしょう。僘を持って行きましょう。",
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
  /** Plugin to ensure before running (sat/llm). */
  plugin?: PluginId;
  note: string;
}

const ENGINES: EngineDef[] = [
  {
    id: "srx",
    label: "SRX rules",
    kind: "engine",
    engineArg: "",
    note: "Fast rule-based splitting — neokapi's default.",
  },
  {
    id: "uax29",
    label: "UAX-29 (ICU4X)",
    kind: "engine",
    engineArg: "uax29",
    note: "The Unicode standard's sentence rules — a plain baseline.",
  },
  {
    id: "hybrid",
    label: "Hybrid",
    kind: "engine",
    engineArg: "hybrid",
    note: "The Unicode rules, refined for real-world abbreviations and the like.",
  },
  {
    id: "intl",
    label: "Intl.Segmenter",
    kind: "engine",
    engineArg: "intl",
    note: "The browser's built-in Unicode segmenter (zero download).",
  },
  {
    id: "sat",
    label: "SaT (ML)",
    kind: "sat",
    plugin: "sat",
    note: "A small machine-learning model trained to find sentence breaks.",
  },
  {
    id: "llm",
    label: "LLM (Gemma)",
    kind: "llm",
    plugin: "llm",
    note: "A local language model asked to split the text into sentences.",
  },
];

interface EngineResult {
  sentences: string[];
  ms: number;
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

function DownloadBar({ st }: { st?: PluginState }): React.ReactElement | null {
  if (!st || st.phase !== "downloading") return null;
  const p = st.progress;
  const frac = p?.frac ?? (p?.total ? (p.loaded ?? 0) / p.total : 0);
  const pct = Math.round(Math.min(1, Math.max(0, frac)) * 100);
  return (
    <div className="mb-2">
      <div className="h-2 w-full overflow-hidden rounded bg-muted">
        <div className="h-full bg-primary transition-all" style={{ width: `${pct}%` }} />
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        Downloading model · {pct}%
        {p?.total ? ` · ${fmtBytes(p.loaded)} of ${fmtBytes(p.total)}` : ""}
      </p>
    </div>
  );
}

// SegmentedSentences renders the engine's output inline: each sentence is a
// numbered block so the boundaries are obvious without opening an overlay view.
function SegmentedSentences({
  result,
  busy,
  error,
}: {
  result: EngineResult | null;
  busy: boolean;
  error: string | null;
}): React.ReactElement {
  if (error) return <p className="text-sm text-destructive">{error}</p>;
  if (busy && !result) return <p className="text-sm text-muted-foreground">Segmenting…</p>;
  if (!result) return <p className="text-sm text-muted-foreground">Not run yet.</p>;
  return (
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
        {result.sentences.length} sentence{result.sentences.length === 1 ? "" : "s"} · {result.ms}
        &nbsp;ms
      </p>
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
  const [sampleId, setSampleId] = useState(SAMPLES[0].id);
  const [text, setText] = useState(SAMPLES[0].text);
  const [locale, setLocale] = useState("en");
  const fileRef = useRef<HTMLInputElement>(null);

  const [results, setResults] = useState<Record<string, EngineResult | null>>({});
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [errors, setErrors] = useState<Record<string, string | null>>({});

  // Configure the shared plugin manager with the wasm asset URLs (idempotent) so
  // bootEngine + ensurePlugin reach the same engine/models the rest of the lab and
  // the navbar widget use. No boot happens here — only on an explicit Run.
  useEffect(() => {
    if (assets) configurePlugins(assets);
  }, [assets]);

  const pickSample = (id: string) => {
    const s = SAMPLES.find((x) => x.id === id);
    if (s) {
      setSampleId(id);
      setText(s.text);
    }
  };

  const onUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    if (!f) return;
    void f.text().then((t) => {
      setText(t);
      setSampleId("");
    });
  };

  async function runEngine(def: EngineDef): Promise<string[]> {
    if (def.kind === "engine") {
      // uax29/hybrid need the ICU4X companion wasm; intl needs the (instant)
      // Intl.Segmenter host bridge installed before the Go engine calls out.
      if (def.engineArg === "uax29" || def.engineArg === "hybrid") await loadICU4X();
      if (def.engineArg === "intl") installIntlSegmenter();
      const rt = (await bootEngine()) as unknown as SegmentRuntime;
      const res = rt.segment(text, def.engineArg ?? "", locale);
      if (!res.ok) throw new Error(res.error ?? "segmentation failed");
      return (res.segments ?? []).map((s) => s.text);
    }
    if (def.kind === "sat") {
      await ensurePlugin("sat");
      const { segmentSat } = await import("@neokapi/kapi-playground/satBridge");
      const r = await segmentSat(text);
      return r.sentences;
    }
    // llm
    await ensurePlugin("llm");
    const { generateGemmaText } = await import("@neokapi/kapi-playground/gemmaBridge");
    const prompt =
      "Split the following text into individual sentences. Output one sentence per line, " +
      "preserving the exact original wording and punctuation. Output only the sentences, " +
      "nothing else.\n\n" +
      text;
    const out = await generateGemmaText(prompt, { maxTokens: 512, temperature: 0 });
    return out
      .split("\n")
      .map((l) => l.replace(/^\s*(?:\d+[.)]\s*|[-*]\s*)?/, "").trim())
      .filter(Boolean);
  }

  const run = (def: EngineDef) => {
    setBusy((b) => ({ ...b, [def.id]: true }));
    setErrors((e) => ({ ...e, [def.id]: null }));
    const t0 = performance.now();
    void runEngine(def)
      .then((sentences) => {
        setResults((r) => ({
          ...r,
          [def.id]: { sentences, ms: Math.round(performance.now() - t0) },
        }));
      })
      .catch((err: unknown) => {
        setErrors((e) => ({ ...e, [def.id]: err instanceof Error ? err.message : String(err) }));
      })
      .finally(() => setBusy((b) => ({ ...b, [def.id]: false })));
  };

  const hasResults = useMemo(() => Object.values(results).some(Boolean), [results]);

  const downloadResults = () => {
    const payload = {
      text,
      locale,
      engines: ENGINES.filter((d) => results[d.id]).map((d) => ({
        engine: d.id,
        sentences: results[d.id]!.sentences,
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
          Bring your own source — pick a sample, paste text, or upload a file — then run each engine
          to see how it splits the content into sentences.
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

      {/* Unified input: sample / paste / upload */}
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <label className="text-sm text-muted-foreground" htmlFor="seg-sample">
            Source
          </label>
          <select
            id="seg-sample"
            value={sampleId}
            onChange={(e) => pickSample(e.target.value)}
            className="rounded border border-border bg-background px-2 py-1 text-sm"
          >
            {sampleId === "" && <option value="">Custom text</option>}
            {SAMPLES.map((s) => (
              <option key={s.id} value={s.id}>
                {s.label}
              </option>
            ))}
          </select>
          <button
            type="button"
            onClick={() => fileRef.current?.click()}
            className="rounded border border-border px-2 py-1 text-sm hover:bg-muted/60"
          >
            Upload file
          </button>
          <input
            ref={fileRef}
            type="file"
            accept=".txt,.md,.html,.json,text/*"
            onChange={onUpload}
            className="hidden"
          />
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
        <textarea
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            setSampleId("");
          }}
          rows={4}
          className="w-full rounded border border-border bg-background p-2 text-sm"
          aria-label="Text to segment"
        />
      </div>

      {/* Engine grid: each segments the same input, inline */}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {ENGINES.map((d) => {
          const st = d.plugin ? mgr.state.plugins[d.plugin] : undefined;
          const isBusy = !!busy[d.id];
          return (
            <div key={d.id} className="flex flex-col rounded-lg border border-border p-3">
              <div className="mb-1 flex items-center justify-between gap-2">
                <span className="text-sm font-semibold">{d.label}</span>
                <button
                  type="button"
                  onClick={() => run(d)}
                  disabled={isBusy}
                  className="rounded border border-border px-2 py-1 text-xs hover:bg-muted/60 disabled:opacity-50"
                >
                  {isBusy ? "Running…" : "Run"}
                </button>
              </div>
              <p className="mb-2 text-xs text-muted-foreground">{d.note}</p>
              <DownloadBar st={st} />
              <SegmentedSentences
                result={results[d.id] ?? null}
                busy={isBusy}
                error={errors[d.id] ?? null}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
