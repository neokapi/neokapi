import React, { useCallback, useMemo, useState } from "react";
import { useLabRuntime } from "./useLabRuntime";
import GateOverlay from "./GateOverlay";
import { useRunGate } from "./useRunGate";
import type { LabRuntimeAssets } from "./useLabRuntime";
import type { LocalProgress } from "@neokapi/kapi-playground/localLlmBridge";

export interface ModelsExplorerProps {
  /** wasm engine assets (exec + module URLs); null until the host resolves them. */
  assets: LabRuntimeAssets | null;
  /** Seed text to translate. */
  defaultText?: string;
  /** Seed target language (BCP-47). */
  defaultTargetLang?: string;
}

// The browser's local model lineup — the SAME model names as the native
// `kapi ollama` picks, so a model reference is identical on web and desktop. Each
// runs on whichever browser backend has a build for it: Llama/Qwen via WebLLM
// (MLC), Gemma 4 via transformers.js (ONNX) — both on WebGPU. (Mirrors
// LOCAL_MODELS in localLlmBridge; duplicated as plain data so selecting a model
// doesn't eagerly import the multi-MB engine bundles — those load on Run.)
const LOCAL_MODELS = [
  { id: "llama3.2:3b", label: "Llama 3.2 3B", size: "~2.3 GB", note: "reliable inline tags", engine: "WebLLM" },
  { id: "gemma4:e2b", label: "Gemma 4 E2B", size: "~3 GB", note: "best multilingual quality", engine: "ONNX" },
  { id: "qwen3:1.7b", label: "Qwen3 1.7B", size: "~1.4 GB", note: "fastest", engine: "WebLLM" },
];
const DEFAULT_LOCAL_MODEL = "llama3.2:3b";

// Cloud providers + their default model — the same names the native provider
// registry reports. Listed for reference; they need an API key, so the browser
// demo doesn't run them.
const CLOUD_PROVIDERS = [
  { id: "anthropic", label: "Anthropic", model: "claude-sonnet-4-20250514" },
  { id: "openai", label: "OpenAI", model: "gpt-4o" },
  { id: "gemini", label: "Gemini", model: "gemini-3-flash-preview" },
];

function hasWebGPU(): boolean {
  return typeof navigator !== "undefined" && "gpu" in navigator && Boolean(navigator.gpu);
}

// ModelsExplorer is the in-browser twin of `kapi models` / the desktop model
// picker. It teaches the real model: one LOCAL provider that runs on-device (here
// via WebLLM/WebGPU; on desktop/CLI via Ollama), CLOUD providers that need a key,
// and plugins that add formats/segmenters. The Local section is live — it runs a
// real `kapi translate --provider local` in the browser.
export default function ModelsExplorer({
  assets,
  defaultText = "Configure your dashboard during onboarding, then deploy.",
  defaultTargetLang = "fr",
}: ModelsExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  // The "local" provider is built into the wasm engine; no plugin download is
  // required, so the gate only boots the engine. WebLLM fetches its own model on
  // the first run (progress streams below).
  const gate = useRunGate(runtime, { requires: [] });
  const [model, setModel] = useState(DEFAULT_LOCAL_MODEL);
  const [text, setText] = useState(defaultText);
  const [lang, setLang] = useState(defaultTargetLang);
  const [out, setOut] = useState<string | null>(null);
  const [busy, setBusy] = useState<"" | "running">("");
  const [progress, setProgress] = useState<LocalProgress | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const webgpu = useMemo(() => hasWebGPU(), []);
  // The browser model id is the same as the native Ollama name, so the desktop
  // command is the identical model reference — only the provider differs.
  const cliCommand = `kapi translate input.json --provider ollama --model ${model} --target-lang ${lang}`;

  const run = useCallback(async () => {
    setErr(null);
    setOut(null);
    setProgress(null);
    setBusy("running");
    try {
      // Install the host hook the wasm `local` provider calls. WebLLM (WebGPU)
      // runs the model; without WebGPU it falls back to transformers.js.
      const { installLocalLLMBridge } = await import("@neokapi/kapi-playground/localLlmBridge");
      installLocalLLMBridge({ model, onProgress: (p) => setProgress(p) });

      const inPath = runtime.writeFile("source.json", JSON.stringify({ message: text }, null, 2));
      const outPath = "/project/source.translated.json";
      const code = await runtime.run([
        "translate",
        inPath,
        "--provider",
        "local",
        "--model",
        model,
        "--target-lang",
        lang,
        "-o",
        outPath,
      ]);
      if (code !== 0) {
        setErr(`translation failed (exit code ${code})`);
        return;
      }
      setOut(runtime.readFile(outPath) ?? "(no output produced)");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy("");
    }
  }, [runtime, text, lang, model]);

  const progressLabel = (() => {
    if (!progress) return busy === "running" ? "Preparing…" : "";
    const engine = progress.engine === "webllm" ? "WebLLM" : "transformers.js";
    if (progress.status === "ready") return `Model ready (${engine}) — generating…`;
    const pct = progress.progress != null ? ` ${Math.round(progress.progress)}%` : "";
    return `Downloading model via ${engine}…${pct}`;
  })();

  return (
    <div
      className="kapi-reference relative"
      style={{ display: "flex", flexDirection: "column", gap: 16 }}
    >
      <p style={{ margin: 0, fontSize: 14, opacity: 0.85 }}>
        The models kapi can translate with, across three sources. The{" "}
        <strong>same local models</strong> are offered on web and desktop — only the backend
        differs: in this browser each runs on the best engine that has a build for it (WebLLM or
        ONNX, both on WebGPU); on the desktop app and CLI they run via <strong>Ollama</strong>. No
        API key, nothing sent to a server.
      </p>

      {/* Local · on-device — the live section */}
      <section style={sectionStyle}>
        <div style={sectionHeadStyle}>
          <span style={{ fontWeight: 700 }}>Local · on-device</span>
          <span style={{ fontSize: 12, opacity: 0.7 }}>
            {webgpu ? "this browser → WebGPU (WebLLM / ONNX)" : "no WebGPU → transformers.js fallback"}{" "}
            · desktop/CLI → Ollama
          </span>
        </div>

        {!webgpu && (
          <p style={{ margin: "0 0 8px", fontSize: 13, color: "#b45309" }}>
            This browser has no WebGPU, so the demo uses the slower transformers.js fallback (a
            smaller model). For the full experience use a WebGPU browser, or the desktop app / CLI
            with Ollama.
          </p>
        )}

        <div style={{ display: "flex", flexWrap: "wrap", gap: 12, alignItems: "flex-end" }}>
          <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            <span style={{ fontWeight: 600 }}>Model</span>
            <select value={model} onChange={(e) => setModel(e.target.value)} style={{ padding: 6 }}>
              {LOCAL_MODELS.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.label} ({m.size}){m.id === DEFAULT_LOCAL_MODEL ? " — default" : ""} · {m.note} ·{" "}
                  {m.engine}
                </option>
              ))}
            </select>
          </label>
          <label style={{ display: "flex", alignItems: "center", gap: 4 }}>
            <span style={{ fontWeight: 600 }}>Target</span>
            <input
              value={lang}
              onChange={(e) => setLang(e.target.value)}
              size={6}
              style={{ padding: 4 }}
            />
          </label>
          <button
            type="button"
            onClick={run}
            disabled={!runtime.ready || busy === "running"}
            style={{ padding: "6px 14px", fontWeight: 600 }}
          >
            {busy === "running" ? "Working…" : "Translate locally"}
          </button>
          {!runtime.ready && <span style={{ fontStyle: "italic" }}>booting engine…</span>}
        </div>

        <label style={{ display: "flex", flexDirection: "column", gap: 4, marginTop: 10 }}>
          <span style={{ fontWeight: 600 }}>Source text</span>
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            rows={2}
            style={{ width: "100%", fontFamily: "inherit", padding: 8 }}
          />
        </label>

        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: 10,
            minHeight: 120,
            marginTop: 10,
          }}
        >
          {busy === "running" && progressLabel && (
            <p style={{ fontStyle: "italic", margin: 0 }}>{progressLabel}</p>
          )}
          {err && <p style={{ color: "#dc2626", margin: 0 }}>Error: {err}</p>}
          {out && <pre style={preStyle}>{out}</pre>}
        </div>

        <p style={{ margin: "4px 0 0", fontSize: 12, opacity: 0.7 }}>
          Same thing from the CLI / desktop:
        </p>
        <pre style={{ ...preStyle, margin: 0 }}>{cliCommand}</pre>
      </section>

      {/* Cloud · needs key */}
      <section style={sectionStyle}>
        <div style={sectionHeadStyle}>
          <span style={{ fontWeight: 700 }}>Cloud · require an API key</span>
          <span style={{ fontSize: 12, opacity: 0.7 }}>
            highest quality · billed by the provider
          </span>
        </div>
        <table style={{ borderCollapse: "collapse", fontSize: 14 }}>
          <tbody>
            {CLOUD_PROVIDERS.map((p) => (
              <tr key={p.id}>
                <td style={{ padding: "2px 16px 2px 0", fontWeight: 600 }}>{p.label}</td>
                <td style={{ padding: "2px 0", fontFamily: "monospace", opacity: 0.85 }}>
                  {p.model}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p style={{ margin: "6px 0 0", fontSize: 12, opacity: 0.7 }}>
          e.g. <code>kapi translate input.json --provider anthropic --target-lang fr</code> (with a
          saved credential).
        </p>
      </section>

      {/* Plugins */}
      <section style={sectionStyle}>
        <div style={sectionHeadStyle}>
          <span style={{ fontWeight: 700 }}>Plugins</span>
          <span style={{ fontSize: 12, opacity: 0.7 }}>add formats &amp; segmenters</span>
        </div>
        <p style={{ margin: 0, fontSize: 14, opacity: 0.85 }}>
          Plugins extend kapi with on-device capabilities — segmentation (<code>sat</code>),
          OCR/layout (<code>vision</code>), speech (<code>asr</code>), PDF (<code>pdfium</code>).
          Retired plugins stay listed but inert and are cleaned up with{" "}
          <code>kapi plugins prune</code>. See <a href="/contribute/plugins">the plugin model</a>.
        </p>
      </section>

      <GateOverlay
        gate={gate}
        title="Models & Providers"
        description="Boot the in-browser engine to translate locally with WebLLM."
      />
    </div>
  );
}

const sectionStyle: React.CSSProperties = {
  border: "1px solid var(--ifm-color-emphasis-300, #d0d7de)",
  borderRadius: 8,
  padding: 12,
  display: "flex",
  flexDirection: "column",
};

const sectionHeadStyle: React.CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  alignItems: "baseline",
  gap: 8,
  marginBottom: 8,
  flexWrap: "wrap",
};

const preStyle: React.CSSProperties = {
  background: "var(--ifm-pre-background, #1e1e1e)",
  color: "var(--ifm-pre-color, #eee)",
  padding: 12,
  borderRadius: 6,
  overflowX: "auto",
  fontSize: 13,
};
