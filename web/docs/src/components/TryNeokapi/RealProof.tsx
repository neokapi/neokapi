import React, { useState } from "react";
import { Download, FileDown, Loader2 } from "lucide-react";
import { TRY_SAMPLES, type TrySample } from "@neokapi/kapi-playground";
import {
  buildSearchReplaceRecipe,
  downloadBytes,
  type LabRuntime,
  type LabRuntimeAssets,
} from "@neokapi/kapi-lab";
import styles from "./styles.module.css";

// The real downloadable proof beneath the live showcase. "Download source" hands
// the visitor a genuine Office/text file (one of TRY_SAMPLES). "Download result"
// runs the REAL search-replace (find → replace) over that source through the
// canonical Go engine (the same WASM runtime the showcase booted) and downloads
// the transformed file — so a visitor can open it in PowerPoint / Excel / any
// editor and confirm neokapi round-trips real files.

interface RealProofProps {
  /** Resolved wasm asset URLs (kept for symmetry / future use). */
  assets: LabRuntimeAssets;
  /** The shared, already-booted modal runtime. */
  runtime: LabRuntime;
  /** The find/replace pair driving the real run (mirrors the showcase). */
  find: string;
  replace: string;
}

function resultName(filename: string, replace: string): string {
  const tag = (replace || "out")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
  const dot = filename.lastIndexOf(".");
  if (dot === -1) return `${filename}-${tag}`;
  return `${filename.slice(0, dot)}-${tag}${filename.slice(dot)}`;
}

export default function RealProof({ runtime, find, replace }: RealProofProps): React.ReactElement {
  const [sample, setSample] = useState<TrySample>(TRY_SAMPLES[0]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  function downloadSource(): void {
    downloadBytes(sample.filename, sample.bytes());
  }

  async function downloadResult(): Promise<void> {
    setErr(null);
    setBusy(true);
    try {
      if (!runtime.ready) throw new Error("engine is still starting — try again in a moment");
      runtime.writeFile(sample.filename, sample.bytes());
      runtime.writeFile("try.kapi", buildSearchReplaceRecipe(find, replace, false));
      const out = `out-${sample.filename}`;
      const code = await runtime.run([
        "run",
        "lab",
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
      const bytes = runtime.readBytes(`/project/${out}`);
      if (!bytes) throw new Error("no output produced");
      downloadBytes(resultName(sample.filename, replace), bytes);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
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
        className={`${styles.btn} ${styles.btnPrimary}`}
        onClick={downloadResult}
        disabled={busy || !runtime.ready}
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
            PowerPoint, Excel, or any editor to confirm neokapi round-trips the file.
          </>
        )}
      </p>
    </div>
  );
}
