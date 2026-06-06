import React, { useState } from "react";
import { Download, FileDown, Loader2 } from "lucide-react";
import { TRY_SAMPLES, type TrySample } from "@neokapi/kapi-playground";
import { useLabRuntime, downloadBytes, type LabRuntimeAssets } from "@neokapi/kapi-lab";
import styles from "./styles.module.css";

// The REAL proof, separate from the faked visual showcase. "Download source"
// hands the visitor a genuine Office/text file (one of TRY_SAMPLES). "Download
// result" boots the kapi WASM engine lazily, runs the REAL search-replace
// (Acme → Globex) over that source through the canonical Go engine, and
// downloads the transformed file — so the visitor can open it in PowerPoint /
// Excel / any editor and confirm neokapi round-trips real files.
//
// wasm boots only when "Download result" is pressed: passing null assets to
// useLabRuntime keeps it idle until the reader opts in (matching the page's
// zero-wasm-on-load contract).

interface RealProofProps {
  /** Resolved wasm asset URLs (from the docs playground config). */
  assets: LabRuntimeAssets;
  /** The find/replace pair driving the real run (mirrors the showcase). */
  find: string;
  replace: string;
}

// One-step inline recipe that drives the search-replace tool over the source.
// Mirrors buildSearchReplaceRecipe in kapi-lab's SearchReplaceWidget (the
// canonical way to pass a find/replace pair to the tool from the browser).
function buildRecipe(find: string, replace: string): string {
  const pair = { search: find, replace, isRegex: false };
  return [
    "version: v1",
    "name: Try",
    "defaults:",
    "  source_language: en",
    "flows:",
    "  try:",
    "    steps:",
    "      - tool: search-replace",
    "        config:",
    `          pairs: ${JSON.stringify([pair])}`,
    "          source: true",
    "          target: false",
    "          regEx: false",
    "",
  ].join("\n");
}

function resultName(filename: string): string {
  const dot = filename.lastIndexOf(".");
  if (dot === -1) return `${filename}-globex`;
  return `${filename.slice(0, dot)}-globex${filename.slice(dot)}`;
}

export default function RealProof({ assets, find, replace }: RealProofProps): React.ReactElement {
  // The sample the buttons operate on. Default to the slide deck — the most
  // visually recognizable proof when reopened in an Office app.
  const [sample, setSample] = useState<TrySample>(TRY_SAMPLES[0]);
  // wasm stays idle (null assets) until the reader presses "Download result".
  const [boot, setBoot] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const rt = useLabRuntime(boot ? assets : null);

  function downloadSource(): void {
    downloadBytes(sample.filename, sample.bytes());
  }

  async function downloadResult(): Promise<void> {
    setErr(null);
    setBusy(true);
    try {
      // Boot lazily on first click; then wait until the runtime is ready.
      if (!boot) setBoot(true);
      // Poll readiness — useLabRuntime boots asynchronously after the assets
      // become non-null. Bounded so a failed boot surfaces rather than hangs.
      for (let i = 0; i < 600 && !rtReady(); i++) {
        await delay(100);
        if (rt.status === "error") throw new Error(rt.error ?? "engine failed to start");
      }
      if (!rtReady()) throw new Error("engine is still starting — try again in a moment");

      rt.writeFile(sample.filename, sample.bytes());
      rt.writeFile("try.kapi", buildRecipe(find, replace));
      const out = `out-${sample.filename}`;
      const code = await rt.run([
        "run",
        "try",
        "-p",
        "/project/try.kapi",
        "-i",
        `/project/${sample.filename}`,
        "-o",
        `/project/${out}`,
        "--target-lang",
        "fr",
      ]);
      if (code !== 0) throw new Error(`engine exited with code ${code}`);
      const bytes = rt.readBytes(`/project/${out}`);
      if (!bytes) throw new Error("no output produced");
      downloadBytes(resultName(sample.filename), bytes);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }

    function rtReady(): boolean {
      return rt.ready;
    }
  }

  return (
    <div className={styles.proof}>
      <span className={styles.proofLabel}>Prove it on a real file:</span>
      <select
        className={styles.input}
        style={{ width: "auto" }}
        value={sample.id}
        onChange={(e) =>
          setSample(TRY_SAMPLES.find((s) => s.id === e.target.value) ?? TRY_SAMPLES[0])
        }
        aria-label="Pick a real sample file"
      >
        {TRY_SAMPLES.map((s) => (
          <option key={s.id} value={s.id}>
            {s.label}
          </option>
        ))}
      </select>
      <button type="button" className={styles.btn} onClick={downloadSource}>
        <FileDown size={15} aria-hidden="true" /> Download source
      </button>
      <button
        type="button"
        className={clsxPrimary()}
        onClick={downloadResult}
        disabled={busy}
        aria-label="Run the real engine and download the transformed file"
      >
        {busy ? (
          <Loader2 size={15} aria-hidden="true" className="animate-spin" />
        ) : (
          <Download size={15} aria-hidden="true" />
        )}
        {busy ? "Running engine…" : "Download result"}
      </button>
      <p className={styles.proofNote}>
        {err ? (
          <span style={{ color: "var(--ifm-color-danger)" }}>Error: {err}</span>
        ) : (
          <>
            The result runs the real kapi engine in your browser (WebAssembly) — open it in
            PowerPoint, Excel, or any editor to confirm neokapi round-trips the file. The visual
            above is a simulated preview.
          </>
        )}
      </p>
    </div>
  );
}

function clsxPrimary(): string {
  return `${styles.btn} ${styles.btnPrimary}`;
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
