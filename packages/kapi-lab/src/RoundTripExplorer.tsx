import React, { useCallback, useEffect, useState } from "react";
import { useLabRuntime } from "./useLabRuntime";
import type { LabRuntimeAssets } from "./useLabRuntime";
import FileSource from "./FileSource";
import type { FileSourceValue } from "./FileSource";
import { SAMPLES } from "./samples";
import shared from "./styles.module.css";
import styles from "./RoundTripExplorer.module.css";

export interface RoundTripExplorerProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
  /** Sample selected on first render (default: first sample). */
  defaultSampleId?: string;
  /** Restrict the offered samples. */
  sampleIds?: string[];
}

// RoundTripExplorer reads a file into blocks + a non-translatable skeleton and
// writes it back, then shows the original beside the round-tripped output. It
// drives the round trip with `pseudo-translate`, which rewrites only the
// translatable leaf text and leaves all markup and structure byte-for-byte
// intact — so the side-by-side makes the preserved skeleton visible: every tag,
// key, and delimiter is unchanged while the readable text turns into pseudo
// glyphs. The real kapi reader+writer run in WASM via the lab runtime.
export default function RoundTripExplorer({
  assets,
  defaultSampleId,
  sampleIds,
}: RoundTripExplorerProps): React.ReactElement {
  const runtime = useLabRuntime(assets);

  const initial = SAMPLES.find((s) => s.id === defaultSampleId) ?? SAMPLES[0];
  const [file, setFile] = useState<FileSourceValue>({
    filename: initial.filename,
    content: initial.content,
    label: initial.label,
  });
  const [output, setOutput] = useState<string | null>(null);
  const [blockCount, setBlockCount] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const runRoundTrip = useCallback(async () => {
    if (!runtime.ready) return;
    setBusy(true);
    setError(null);
    // Seed the source into the in-memory FS, then read → pseudo-translate →
    // write to a sibling path. trace() performs the whole round trip and leaves
    // the rendered output file on the volume for us to read back.
    const inPath = runtime.writeFile(file.filename, file.content);
    const outPath = `/project/roundtrip-${file.filename}`;
    const res = await runtime.trace(["pseudo-translate", inPath, "-o", outPath]);
    if (res.ok && res.trace) {
      const written = runtime.readFile(outPath);
      if (written === null) {
        setError("the writer produced no output file");
        setOutput(null);
        setBlockCount(null);
      } else {
        setOutput(written);
        // Surface how many translatable blocks the reader found — the count of
        // Block parts in the trace. Everything else in the file is skeleton.
        const blocks = Object.values(res.trace.parts).filter(
          (p) => p.initial.type === "Block",
        ).length;
        setBlockCount(blocks);
      }
    } else {
      setError(res.error ?? "the round trip produced no output");
      setOutput(null);
      setBlockCount(null);
    }
    setBusy(false);
  }, [runtime.ready, runtime.writeFile, runtime.trace, runtime.readFile, file]);

  // Auto-run once the runtime is ready, and whenever the file changes.
  // runRoundTrip is a stable callback keyed on those inputs.
  useEffect(() => {
    if (runtime.ready) void runRoundTrip();
  }, [runtime.ready, runRoundTrip]);

  return (
    <div className={shared.explorer}>
      <FileSource value={file} onChange={setFile} sampleIds={sampleIds} />

      <div className={`${shared.statusBar} ${error ? shared.statusError : ""}`}>
        {runtime.status === "booting" && "Booting kapi (first run downloads ~13 MB)…"}
        {runtime.status === "error" && `Failed to start: ${runtime.error}`}
        {runtime.ready && busy && "Reading, pseudo-translating, and writing back…"}
        {runtime.ready && !busy && error && `Error: ${error}`}
        {runtime.ready && !busy && !error && blockCount !== null && (
          <span className={shared.stats}>
            <span className={shared.statBadge}>
              <span className={shared.statCount}>{blockCount}</span>{" "}
              {blockCount === 1 ? "translatable block" : "translatable blocks"}
            </span>
          </span>
        )}
      </div>

      {output !== null && (
        <>
          <div className={styles.diff}>
            <div className={styles.column}>
              <div className={styles.columnHeader}>
                <span>Source</span>
                <span className={styles.columnTag}>{file.filename}</span>
              </div>
              <pre className={styles.code}>{file.content}</pre>
            </div>
            <div className={styles.column}>
              <div className={styles.columnHeader}>
                <span>Round-tripped</span>
                <span className={styles.columnTag}>pseudo-translate</span>
              </div>
              <pre className={styles.code}>{output}</pre>
            </div>
          </div>
          <p className={styles.note}>
            Only the translatable leaf text changed — pseudo-translation rewrote it into accented
            glyphs. Every tag, key, attribute, and delimiter belongs to the non-translatable
            skeleton the reader set aside, so the writer reproduces the surrounding structure
            exactly.
          </p>
        </>
      )}
    </div>
  );
}
