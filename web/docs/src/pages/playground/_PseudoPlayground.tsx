import React, { useCallback, useEffect, useRef, useState } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import styles from "./styles.module.css";

// The Go wasm module installs these on the global scope. They're untyped
// from TS's perspective, so we reach for them through a small typed shim.
interface KapiGlobal {
  Go?: new () => { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> };
  __kapiWasmReady?: () => void;
  kapiVersion?: () => string;
  kapiPseudoTranslate?: (
    filename: string,
    bytes: Uint8Array,
    optsJSON: string,
  ) => PseudoResult;
}

interface BlockPair {
  source: string;
  target: string;
}

interface PseudoResult {
  ok: boolean;
  error?: string;
  format?: string;
  outputBase64?: string;
  blockCount?: number;
  wordCount?: number;
  blocks?: BlockPair[];
}

type LoadState = "loading" | "ready" | "error";

function kapi(): KapiGlobal {
  return globalThis as unknown as KapiGlobal;
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = src;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error(`failed to load ${src}`));
    document.head.appendChild(s);
  });
}

function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export default function PseudoPlayground(): React.ReactElement {
  const wasmExecUrl = useBaseUrl("/wasm/wasm_exec.js");
  const wasmUrl = useBaseUrl("/wasm/kapi.wasm");

  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string>("");
  const [version, setVersion] = useState<string>("");

  const [fileName, setFileName] = useState<string>("");
  const fileBytes = useRef<Uint8Array | null>(null);

  const [targetLang, setTargetLang] = useState("fr");
  const [prefix, setPrefix] = useState("[");
  const [suffix, setSuffix] = useState("]");
  const [expansion, setExpansion] = useState(0);

  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<PseudoResult | null>(null);
  const [runError, setRunError] = useState<string>("");

  // Load wasm_exec.js then instantiate the module, once.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await loadScript(wasmExecUrl);
        const k = kapi();
        if (!k.Go) throw new Error("wasm_exec.js did not define Go");

        const ready = new Promise<void>((resolve) => {
          k.__kapiWasmReady = () => resolve();
        });

        const go = new k.Go();
        let instance: WebAssembly.Instance;
        try {
          const res = await WebAssembly.instantiateStreaming(
            fetch(wasmUrl),
            go.importObject,
          );
          instance = res.instance;
        } catch {
          // Fallback when the server doesn't send application/wasm.
          const buf = await (await fetch(wasmUrl)).arrayBuffer();
          const res = await WebAssembly.instantiate(buf, go.importObject);
          instance = res.instance;
        }
        // Intentionally not awaited: main() blocks forever (select{}) so the
        // exported functions stay callable. Readiness comes via the callback.
        void go.run(instance);
        await ready;
        if (cancelled) return;
        setVersion(k.kapiVersion ? k.kapiVersion() : "");
        setLoadState("ready");
      } catch (e) {
        if (cancelled) return;
        setLoadError(e instanceof Error ? e.message : String(e));
        setLoadState("error");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [wasmExecUrl, wasmUrl]);

  const onFile = useCallback(async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    setResult(null);
    setRunError("");
    if (!f) {
      fileBytes.current = null;
      setFileName("");
      return;
    }
    fileBytes.current = new Uint8Array(await f.arrayBuffer());
    setFileName(f.name);
  }, []);

  const run = useCallback(() => {
    const k = kapi();
    if (!k.kapiPseudoTranslate || !fileBytes.current) return;
    setBusy(true);
    setRunError("");
    setResult(null);
    // Defer to a tick so the "Working…" state paints before the (synchronous)
    // wasm call blocks the thread.
    setTimeout(() => {
      try {
        const opts = JSON.stringify({ targetLang, prefix, suffix, expansion });
        const r = k.kapiPseudoTranslate!(fileName, fileBytes.current!, opts);
        if (!r.ok) setRunError(r.error || "unknown error");
        else setResult(r);
      } catch (e) {
        setRunError(e instanceof Error ? e.message : String(e));
      } finally {
        setBusy(false);
      }
    }, 20);
  }, [fileName, targetLang, prefix, suffix, expansion]);

  const download = useCallback(() => {
    if (!result?.outputBase64) return;
    const bytes = base64ToBytes(result.outputBase64);
    const blob = new Blob([bytes as BlobPart], { type: "application/octet-stream" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `pseudo-${fileName || "output"}`;
    a.click();
    URL.revokeObjectURL(url);
  }, [result, fileName]);

  if (loadState === "error") {
    return (
      <div className={styles.notice}>
        <strong>Could not load the in-browser engine.</strong>
        <p>{loadError}</p>
        <p>
          The WebAssembly module is built by <code>make web-wasm-demo</code> and
          served from <code>/wasm/kapi.wasm</code>. If you&apos;re running the
          docs locally, build it first.
        </p>
      </div>
    );
  }

  return (
    <div className={styles.playground}>
      {loadState === "loading" && (
        <p className={styles.notice}>Loading the WebAssembly engine&hellip;</p>
      )}

      <fieldset className={styles.controls} disabled={loadState !== "ready" || busy}>
        <div className={styles.row}>
          <label className={styles.field}>
            <span>Document</span>
            <input type="file" onChange={onFile} accept=".docx,.json,.xliff,.xlf,.html,.htm,.po,.properties,.md,.srt,.vtt,.yaml,.yml,.resx,.arb,.xml" />
          </label>
        </div>
        <div className={styles.row}>
          <label className={styles.field}>
            <span>Target locale</span>
            <input value={targetLang} onChange={(e) => setTargetLang(e.target.value)} size={8} />
          </label>
          <label className={styles.field}>
            <span>Prefix</span>
            <input value={prefix} onChange={(e) => setPrefix(e.target.value)} size={4} />
          </label>
          <label className={styles.field}>
            <span>Suffix</span>
            <input value={suffix} onChange={(e) => setSuffix(e.target.value)} size={4} />
          </label>
          <label className={styles.field}>
            <span>Expansion %</span>
            <input
              type="number"
              min={0}
              value={expansion}
              onChange={(e) => setExpansion(Number(e.target.value) || 0)}
              size={4}
            />
          </label>
        </div>
        <div className={styles.row}>
          <button
            type="button"
            className="button button--primary"
            onClick={run}
            disabled={!fileBytes.current || busy}
          >
            {busy ? "Working…" : "Pseudo-translate"}
          </button>
          {version && <span className={styles.version}>engine {version}</span>}
        </div>
      </fieldset>

      {runError && (
        <div className={styles.notice}>
          <strong>Error:</strong> {runError}
        </div>
      )}

      {result && (
        <div className={styles.result}>
          <div className={styles.summary}>
            <span>
              Format: <code>{result.format}</code>
            </span>
            <span>Blocks: {result.blockCount}</span>
            <span>Words: {result.wordCount}</span>
            <button type="button" className="button button--secondary button--sm" onClick={download}>
              Download translated {result.format === "openxml" ? ".docx" : "file"}
            </button>
          </div>

          {result.blocks && result.blocks.length > 0 && (
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>Source</th>
                  <th>Pseudo-translated</th>
                </tr>
              </thead>
              <tbody>
                {result.blocks.map((b, i) => (
                  <tr key={i}>
                    <td>{b.source}</td>
                    <td>{b.target}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}
