import React, { useEffect, useState } from "react";
import { X } from "lucide-react";
import type { KapiRuntime, PreviewResult } from "./runtime";

function basename(p: string): string {
  return p.replace(/\/$/, "").split("/").pop() || p;
}

export default function FilePreview({
  runtime,
  path,
  onClose,
}: {
  runtime: KapiRuntime;
  path: string;
  onClose: () => void;
}) {
  const [data, setData] = useState<PreviewResult | null>(null);
  const [showRaw, setShowRaw] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setData(null);
    runtime.preview(path).then((r) => {
      if (!cancelled) setData(r);
    });
    return () => {
      cancelled = true;
    };
  }, [runtime, path]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, [onClose]);

  let rawText = "";
  if (showRaw) {
    try {
      rawText = new TextDecoder().decode(runtime.vol.readFile(path));
    } catch (e: any) {
      rawText = String(e?.message || e);
    }
  }

  return (
    <div className="kapi-pg-overlay" onClick={onClose}>
      <div
        className="kapi-pg-preview"
        role="dialog"
        aria-modal="true"
        aria-label={`Preview ${basename(path)}`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="kapi-pg-preview-header">
          <span className="kapi-pg-preview-title">{basename(path)}</span>
          {data?.ok && <span className="kapi-pg-badge">{data.format}</span>}
          {data?.ok && (
            <button
              type="button"
              className="kapi-pg-btn kapi-pg-btn--sm"
              onClick={() => setShowRaw((v) => !v)}
            >
              {showRaw ? "Blocks" : "Raw"}
            </button>
          )}
          <button
            type="button"
            className="kapi-pg-icon-btn"
            onClick={onClose}
            aria-label="Close preview"
            title="Close"
          >
            <X size={16} aria-hidden="true" />
          </button>
        </div>

        <div className="kapi-pg-preview-body">
          {!data && <p>Reading with the kapi parser…</p>}
          {data && !data.ok && (
            <p className="kapi-pg-preview-error">Cannot preview: {data.error}</p>
          )}

          {data?.ok && showRaw && <pre className="kapi-pg-raw">{rawText}</pre>}

          {data?.ok && !showRaw && (
            <>
              <p className="kapi-pg-preview-meta">
                {data.total} block{data.total === 1 ? "" : "s"} · {data.bytes} bytes · parsed as{" "}
                <code>{data.format}</code>
              </p>
              {data.blocks && data.blocks.length > 0 ? (
                <table className="kapi-pg-preview-table">
                  <thead>
                    <tr>
                      <th>#</th>
                      <th>id</th>
                      <th>source text</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.blocks.map((b, i) => (
                      <tr key={i}>
                        <td>{i + 1}</td>
                        <td>
                          <code>{b.id}</code>
                        </td>
                        <td>{b.text}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p>No translatable blocks found.</p>
              )}
              {data.total !== undefined && data.blocks && data.total > data.blocks.length && (
                <p className="kapi-pg-preview-meta">
                  … showing first {data.blocks.length} of {data.total}.
                </p>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
