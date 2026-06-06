import React, { useEffect, useState } from "react";
import { X } from "lucide-react";
import type { KapiRuntime, PreviewResult } from "./runtime";
import { HighlightedCode } from "./syntax";

function basename(p: string): string {
  return p.replace(/\/$/, "").split("/").pop() || p;
}

function ext(p: string): string {
  const b = basename(p);
  return b.includes(".") ? "." + b.slice(b.lastIndexOf(".") + 1).toLowerCase() : "";
}

interface RawRead {
  text: string;
  binary: boolean;
  error?: string;
}

// Read a file's bytes and decode as UTF-8. Binary files (which contain NUL
// bytes or fail a strict UTF-8 decode) are flagged so the caller can show a
// graceful "download to view" message instead of rendering garbage.
function readRaw(runtime: KapiRuntime, path: string): RawRead {
  let bytes: Uint8Array;
  try {
    bytes = runtime.vol.readFile(path);
  } catch (e: any) {
    return { text: "", binary: false, error: String(e?.message || e) };
  }
  // A NUL byte is a strong, cheap signal of binary content.
  if (bytes.includes(0)) return { text: "", binary: true };
  try {
    const text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    return { text, binary: false };
  } catch {
    return { text: "", binary: true };
  }
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

  // When the engine can't parse the file, fall back to raw text automatically
  // (no Blocks view exists to toggle to). `forcedRaw` reflects that state.
  const forcedRaw = data != null && !data.ok;
  const rawActive = forcedRaw || showRaw;

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

  const raw: RawRead | null = rawActive ? readRaw(runtime, path) : null;

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

          {/* Unparseable by the engine: don't dead-end — show raw text with a
              short note explaining there's no reader for this extension. */}
          {forcedRaw && (
            <p className="kapi-pg-preview-note">
              kapi has no reader for <code>{ext(path) || basename(path)}</code> — showing raw text.
            </p>
          )}

          {raw && raw.error && (
            <p className="kapi-pg-preview-error">Cannot read file: {raw.error}</p>
          )}

          {raw && !raw.error && raw.binary && (
            <p className="kapi-pg-preview-note">Binary file — download to view.</p>
          )}

          {raw && !raw.error && !raw.binary && (
            <HighlightedCode className="kapi-pg-raw" text={raw.text} filename={basename(path)} />
          )}

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
