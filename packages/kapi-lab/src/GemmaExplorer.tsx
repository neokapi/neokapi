import React, { useCallback, useState } from "react";
import { useLabRuntime } from "./useLabRuntime";
import RunGate from "./RunGate";
import { useRunGate } from "./useRunGate";
import type { LabRuntimeAssets } from "./useLabRuntime";
import type { GemmaProgress } from "@neokapi/kapi-playground/gemmaBridge";

export interface GemmaExplorerProps {
  /** wasm engine assets (exec + module URLs); null until the host resolves them. */
  assets: LabRuntimeAssets | null;
  /** Seed text to translate. */
  defaultText?: string;
  /** Seed target language (BCP-47). */
  defaultTargetLang?: string;
}

// GemmaExplorer runs `kapi translate --provider gemma` entirely in the
// browser: the kapi wasm engine drives the AI tool, and the Gemma 4 model itself
// runs via transformers.js + WebGPU (gemmaBridge installs the host hook the wasm
// `gemma` provider calls). It is the in-browser twin of the native kapi-llm
// plugin — the same model, no server, no API key.
//
// The model is an opt-in, multi-GB download (cached by the browser after the
// first run), so generation is gated behind an explicit button and reports
// download progress, mirroring VisionExplorer's layout-model opt-in.
export default function GemmaExplorer({
  assets,
  defaultText = "Our new dashboard helps teams ship faster.",
  defaultTargetLang = "fr",
}: GemmaExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);
  const [text, setText] = useState(defaultText);
  const [lang, setLang] = useState(defaultTargetLang);
  const [out, setOut] = useState<string | null>(null);
  const [busy, setBusy] = useState<"" | "running">("");
  const [progress, setProgress] = useState<GemmaProgress | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const run = useCallback(async () => {
    setErr(null);
    setOut(null);
    setProgress(null);
    setBusy("running");
    try {
      // Install the transformers.js host hook the wasm `gemma` provider calls.
      // The model downloads lazily on the first generate (progress streams here).
      const { installGemmaBridge } = await import("@neokapi/kapi-playground/gemmaBridge");
      installGemmaBridge({ onProgress: (p) => setProgress(p) });

      const inPath = runtime.writeFile("source.json", JSON.stringify({ message: text }, null, 2));
      const outPath = "/project/source.translated.json";
      const code = await runtime.run([
        "translate",
        inPath,
        "--provider",
        "gemma",
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
  }, [runtime, text, lang]);

  const progressLabel = (() => {
    if (!progress) return busy === "running" ? "Preparing…" : "";
    if (progress.status === "ready") return "Model ready — generating…";
    const pct = progress.progress != null ? ` ${Math.round(progress.progress)}%` : "";
    return `Downloading Gemma model…${pct}${progress.file ? ` (${progress.file})` : ""}`;
  })();

  if (!gate.armed) {
    return (
      <RunGate
        gate={gate}
        title="Local LLM (Gemma)"
        description="Translate with the real Gemma model running in your browser."
      />
    );
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <p style={{ margin: 0, fontSize: 14, opacity: 0.85 }}>
        Translate with <strong>Gemma&nbsp;4</strong> running locally in your browser (WebGPU). The
        model is a one-time multi-GB download, then cached — nothing is sent to a server.
      </p>

      <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        <span style={{ fontWeight: 600 }}>Source text</span>
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          rows={3}
          style={{ width: "100%", fontFamily: "inherit", padding: 8 }}
        />
      </label>

      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
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
          {busy === "running" ? "Working…" : "Translate with local Gemma"}
        </button>
        {!runtime.ready && <span style={{ fontStyle: "italic" }}>booting engine…</span>}
      </div>

      {busy === "running" && progressLabel && (
        <p style={{ fontStyle: "italic", margin: 0 }}>{progressLabel}</p>
      )}
      {err && <p style={{ color: "#dc2626", margin: 0 }}>Error: {err}</p>}
      {out && (
        <pre
          style={{
            background: "var(--ifm-pre-background, #1e1e1e)",
            color: "var(--ifm-pre-color, #eee)",
            padding: 12,
            borderRadius: 6,
            overflowX: "auto",
            margin: 0,
          }}
        >
          {out}
        </pre>
      )}
    </div>
  );
}
