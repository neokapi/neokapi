import React, { useEffect, useState } from "react";
import { X } from "lucide-react";
import { CodeView, DocumentViewer, type ContentTree } from "@neokapi/ui-primitives/preview";
import type { KapiRuntime } from "./runtime";

function basename(p: string): string {
  return p.replace(/\/$/, "").split("/").pop() || p;
}

// Read a file's bytes; decode as UTF-8, flagging binary content (NUL bytes or a
// failed strict decode) so the raw fallback shows a note instead of garbage.
function readRaw(runtime: KapiRuntime, path: string): { text: string; binary: boolean } {
  let bytes: Uint8Array;
  try {
    bytes = runtime.vol.readFile(path);
  } catch {
    return { text: "", binary: false };
  }
  if (bytes.includes(0)) return { text: "", binary: true };
  try {
    return { text: new TextDecoder("utf-8", { fatal: true }).decode(bytes), binary: false };
  } catch {
    return { text: "", binary: true };
  }
}

// FilePreview shows a file in the SHARED preview editor (DocumentViewer — the
// same Preview / Blocks / Raw / Stats / Download editor used by the docs modal,
// the lab explorers and the desktop app). The editor lives in
// @neokapi/ui-primitives, so the playground reuses it directly (no duplicate
// block/raw views, no syntax-highlighter fork). Files kapi can't parse fall back
// to a raw CodeView.
export default function FilePreview({
  runtime,
  path,
  onClose,
}: {
  runtime: KapiRuntime;
  path: string;
  onClose: () => void;
}) {
  const [tree, setTree] = useState<ContentTree | null>(null);
  const [bytes, setBytes] = useState<Uint8Array | null>(null);
  const [loading, setLoading] = useState(true);
  const [unparseable, setUnparseable] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setTree(null);
    setUnparseable(false);
    try {
      setBytes(runtime.vol.readFile(path));
    } catch {
      setBytes(null);
    }
    void runtime.inspect(path).then((r) => {
      if (cancelled) return;
      if (r.ok && r.tree) setTree(r.tree as ContentTree);
      else setUnparseable(true);
      setLoading(false);
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

  const raw = unparseable ? readRaw(runtime, path) : null;

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
          {loading && <p>Reading with the kapi parser…</p>}

          {tree && <DocumentViewer tree={tree} filename={basename(path)} bytes={bytes} />}

          {!loading && unparseable && (
            <>
              <p className="kapi-pg-preview-note">
                kapi has no reader for this file — showing raw text.
              </p>
              {raw?.binary ? (
                <p className="kapi-pg-preview-note">Binary file — download to view.</p>
              ) : (
                <CodeView text={raw?.text ?? ""} filename={basename(path)} />
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
