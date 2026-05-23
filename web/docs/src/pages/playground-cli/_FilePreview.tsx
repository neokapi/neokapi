import React, { useEffect, useState } from "react";
import type { KapiCli, PreviewResult } from "./_wasmCli";
import styles from "./styles.module.css";

function basename(p: string): string {
  return p.replace(/\/$/, "").split("/").pop() || p;
}

export default function FilePreview({
  cli,
  path,
  onClose,
}: {
  cli: KapiCli;
  path: string;
  onClose: () => void;
}) {
  const [data, setData] = useState<PreviewResult | null>(null);
  const [showRaw, setShowRaw] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setData(null);
    cli.preview(path).then((r) => { if (!cancelled) setData(r); });
    return () => { cancelled = true; };
  }, [cli, path]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  let rawText = "";
  if (showRaw) {
    try { rawText = new TextDecoder().decode(cli.vol.readFile(path)); }
    catch (e: any) { rawText = String(e?.message || e); }
  }

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.modalHeader}>
          <span className={styles.modalTitle}>{basename(path)}</span>
          {data?.ok && <span className={styles.badge}>{data.format}</span>}
          {data?.ok && (
            <button type="button" className="button button--sm button--secondary" onClick={() => setShowRaw((v) => !v)}>
              {showRaw ? "Blocks" : "Raw"}
            </button>
          )}
          <button type="button" className="button button--sm" onClick={onClose} aria-label="Close">✕</button>
        </div>

        <div className={styles.modalBody}>
          {!data && <p>Reading with the kapi parser…</p>}
          {data && !data.ok && <p className={styles.previewError}>Cannot preview: {data.error}</p>}

          {data?.ok && showRaw && <pre className={styles.raw}>{rawText}</pre>}

          {data?.ok && !showRaw && (
            <>
              <p className={styles.previewMeta}>
                {data.total} block{data.total === 1 ? "" : "s"} · {data.bytes} bytes · parsed as <code>{data.format}</code>
              </p>
              {data.blocks && data.blocks.length > 0 ? (
                <table className={styles.previewTable}>
                  <thead>
                    <tr><th>#</th><th>id</th><th>source text</th></tr>
                  </thead>
                  <tbody>
                    {data.blocks.map((b, i) => (
                      <tr key={i}><td>{i + 1}</td><td><code>{b.id}</code></td><td>{b.text}</td></tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p>No translatable blocks found.</p>
              )}
              {data.total !== undefined && data.blocks && data.total > data.blocks.length && (
                <p className={styles.previewMeta}>… showing first {data.blocks.length} of {data.total}.</p>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
