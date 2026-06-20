import React, { useState } from "react";
import { ensurePlugin, usePluginManager } from "@neokapi/kapi-playground/plugins";

// MlSegmentationCompare — the ML and LLM segmenters, side by side with a browser
// baseline, on text you can edit. The rule-based (SRX) and ICU (UAX-29) engines
// live in the engine-backed comparison above; this card adds the two that run
// off-engine in the browser:
//   • SaT — the real wtpsplit "Segment any Text" ONNX model (kapi-sat), on
//     onnxruntime-web, downloaded via the plugin manager (the navbar widget
//     reflects it).
//   • LLM — Gemma 4 (kapi-llm) prompted to split the text into sentences.
// A browser baseline (Intl.Segmenter) runs instantly for reference. Nothing is
// fetched until you press a Run button.

const DEFAULT_TEXT =
  "Dr. Smith paid $3.50 for the U.S. edition on Jan. 5, 2024. " +
  "Mr. Lee asked, “Is it ready?” It was. The next batch ships at 9 a.m.";

interface EngineResult {
  sentences: string[];
  ms: number;
}

function browserSegment(text: string): string[] {
  // The browser's built-in sentence segmenter (Intl.Segmenter) — a quick,
  // zero-download baseline. Not all engines agree; that's the point.
  const Seg = (Intl as unknown as { Segmenter?: typeof Intl.Segmenter }).Segmenter;
  if (!Seg) return text.split(/(?<=[.!?])\s+/).filter(Boolean);
  const seg = new Seg("en", { granularity: "sentence" });
  return Array.from(seg.segment(text), (s) => s.segment.trim()).filter(Boolean);
}

function SentenceList({
  result,
  busy,
  error,
}: {
  result: EngineResult | null;
  busy?: boolean;
  error?: string | null;
}): React.ReactElement {
  if (error) return <p className="text-sm text-destructive">{error}</p>;
  if (busy) return <p className="text-sm text-muted-foreground">Segmenting…</p>;
  if (!result) return <p className="text-sm text-muted-foreground">Not run yet.</p>;
  return (
    <div className="flex flex-col gap-1">
      <ol className="list-decimal pl-5 text-sm">
        {result.sentences.map((s, i) => (
          <li key={i} className="py-0.5">
            {s}
          </li>
        ))}
      </ol>
      <p className="text-xs text-muted-foreground">
        {result.sentences.length} sentence{result.sentences.length === 1 ? "" : "s"} · {result.ms}
        &nbsp;ms
      </p>
    </div>
  );
}

export default function MlSegmentationCompare(): React.ReactElement {
  const mgr = usePluginManager();
  const [text, setText] = useState(DEFAULT_TEXT);

  const [baseline, setBaseline] = useState<EngineResult | null>(null);
  const [sat, setSat] = useState<EngineResult | null>(null);
  const [llm, setLlm] = useState<EngineResult | null>(null);
  const [satBusy, setSatBusy] = useState(false);
  const [llmBusy, setLlmBusy] = useState(false);
  const [satErr, setSatErr] = useState<string | null>(null);
  const [llmErr, setLlmErr] = useState<string | null>(null);

  const runBaseline = () => {
    const t0 = performance.now();
    setBaseline({ sentences: browserSegment(text), ms: Math.round(performance.now() - t0) });
  };

  const runSat = async () => {
    setSatBusy(true);
    setSatErr(null);
    try {
      await ensurePlugin("sat");
      const { segmentSat } = await import("@neokapi/kapi-playground/satBridge");
      const t0 = performance.now();
      const res = await segmentSat(text);
      setSat({ sentences: res.sentences, ms: Math.round(performance.now() - t0) });
    } catch (e) {
      setSatErr(e instanceof Error ? e.message : String(e));
    } finally {
      setSatBusy(false);
    }
  };

  const runLlm = async () => {
    setLlmBusy(true);
    setLlmErr(null);
    try {
      await ensurePlugin("llm");
      const { generateGemmaText } = await import("@neokapi/kapi-playground/gemmaBridge");
      const prompt =
        "Split the following text into individual sentences. Output one sentence per line, " +
        "preserving the exact original wording and punctuation. Output only the sentences, " +
        "nothing else.\n\n" +
        text;
      const t0 = performance.now();
      const out = await generateGemmaText(prompt, { maxTokens: 512, temperature: 0 });
      const sentences = out
        .split("\n")
        .map((l) => l.replace(/^\s*(?:\d+[.)]\s*|[-*]\s*)?/, "").trim())
        .filter(Boolean);
      setLlm({ sentences, ms: Math.round(performance.now() - t0) });
    } catch (e) {
      setLlmErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLlmBusy(false);
    }
  };

  const satState = mgr.state.plugins.sat;
  const llmState = mgr.state.plugins.llm;
  const dlPct = (frac?: number) => (frac != null ? `${Math.round(frac * 100)}%` : "");

  return (
    <div className="kapi-reference flex flex-col gap-3 text-foreground">
      <div>
        <label className="mb-1 block text-sm font-medium" htmlFor="ml-seg-text">
          Text
        </label>
        <textarea
          id="ml-seg-text"
          value={text}
          onChange={(e) => setText(e.target.value)}
          rows={3}
          className="w-full rounded border border-border bg-background p-2 text-sm"
        />
      </div>

      <div className="grid gap-3 md:grid-cols-3">
        <div className="rounded-lg border border-border p-3">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-semibold">Browser baseline</span>
            <button
              type="button"
              onClick={runBaseline}
              className="rounded border px-2 py-1 text-xs hover:bg-muted/60"
            >
              Run
            </button>
          </div>
          <p className="mb-2 text-xs text-muted-foreground">
            Intl.Segmenter — instant, no download.
          </p>
          <SentenceList result={baseline} />
        </div>

        <div className="rounded-lg border border-border p-3">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-semibold">SaT (ML)</span>
            <button
              type="button"
              onClick={() => void runSat()}
              disabled={satBusy}
              className="rounded border px-2 py-1 text-xs hover:bg-muted/60 disabled:opacity-50"
            >
              {satBusy ? "Running…" : "Run"}
            </button>
          </div>
          <p className="mb-2 text-xs text-muted-foreground">
            wtpsplit sat-3l-sm via onnxruntime-web.
            {satState?.phase === "downloading" &&
              ` Downloading model ${dlPct(satState.progress?.frac)}…`}
          </p>
          <SentenceList result={sat} busy={satBusy && !sat} error={satErr} />
        </div>

        <div className="rounded-lg border border-border p-3">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-semibold">LLM (Gemma)</span>
            <button
              type="button"
              onClick={() => void runLlm()}
              disabled={llmBusy}
              className="rounded border px-2 py-1 text-xs hover:bg-muted/60 disabled:opacity-50"
            >
              {llmBusy ? "Running…" : "Run"}
            </button>
          </div>
          <p className="mb-2 text-xs text-muted-foreground">
            Gemma 4 prompted to split sentences.
            {llmState?.phase === "downloading" &&
              ` Downloading model ${dlPct(llmState.progress?.frac)}…`}
          </p>
          <SentenceList result={llm} busy={llmBusy && !llm} error={llmErr} />
        </div>
      </div>
    </div>
  );
}
