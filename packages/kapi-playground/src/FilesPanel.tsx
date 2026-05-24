import React, { useEffect, useRef, useState } from "react";
import { Download, Trash2, Upload, FolderOpen, FileText, CornerLeftUp } from "lucide-react";
import type { KapiRuntime } from "./runtime";
import FilePreview from "./FilePreview";

function joinPath(dir: string, name: string): string {
  return dir.replace(/\/$/, "") + "/" + name;
}

// Resolve ".." against an absolute path string (no chdir needed).
function parentDir(abs: string): string {
  const trimmed = abs.replace(/\/$/, "");
  const slash = trimmed.lastIndexOf("/");
  return slash <= 0 ? "/" : trimmed.slice(0, slash);
}

export default function FilesPanel({
  runtime,
  refreshKey,
  onChange,
}: {
  runtime: KapiRuntime;
  refreshKey: number;
  onChange: () => void;
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [previewPath, setPreviewPath] = useState<string | null>(null);
  // viewDir is the panel's own browsing position — completely independent of the
  // terminal's cwd. It starts at the runtime cwd and can be browsed freely
  // without ever calling runtime.chdir, so the shell cwd stays untouched.
  const [viewDir, setViewDir] = useState<string>(() => runtime.cwd());

  // After each render (triggered by refreshKey when the fs or terminal cwd
  // changes), reconcile viewDir:
  //   • If the terminal cwd changed and the panel was still at the old cwd root,
  //     follow it — so a `cd` command keeps the panel in sync.
  //   • If viewDir no longer exists (e.g. after a Reset), fall back to the
  //     terminal cwd.
  //   • Otherwise leave the panel wherever the user browsed to.
  const prevCwdRef = useRef<string>(runtime.cwd());
  useEffect(() => {
    const runtimeCwd = runtime.cwd();
    const prevCwd = prevCwdRef.current;
    prevCwdRef.current = runtimeCwd;
    setViewDir((vd) => {
      if (!runtime.vol.exists(vd)) return runtimeCwd;
      if (runtimeCwd !== prevCwd && vd === prevCwd) return runtimeCwd;
      return vd;
    });
  });

  // refreshKey is a dependency only to force re-render when the fs changes.
  void refreshKey;

  let entries: string[] = [];
  try {
    entries = runtime.vol.readdir(viewDir);
  } catch {
    entries = [];
  }

  async function onUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files || []);
    for (const f of files) {
      const buf = new Uint8Array(await f.arrayBuffer());
      runtime.vol.writeFile(joinPath(viewDir, f.name), buf);
    }
    if (fileInput.current) fileInput.current.value = "";
    onChange();
  }

  function download(name: string) {
    const data = runtime.vol.readFile(joinPath(viewDir, name));
    // Copy into a fresh ArrayBuffer so the Blob owns contiguous bytes.
    const copy = new Uint8Array(data.length);
    copy.set(data);
    const url = URL.createObjectURL(new Blob([copy as BlobPart]));
    const a = document.createElement("a");
    a.href = url;
    a.download = name;
    a.click();
    URL.revokeObjectURL(url);
  }

  function remove(name: string) {
    runtime.vol.remove(joinPath(viewDir, name));
    onChange();
  }

  function enterDir(name: string) {
    // Navigate the panel view only — never call runtime.chdir.
    setViewDir(joinPath(viewDir, name));
  }

  return (
    <div className="kapi-pg-files">
      <div className="kapi-pg-files-header">
        <span className="kapi-pg-files-title">Files</span>
        <button
          type="button"
          className="kapi-pg-icon-btn kapi-pg-icon-btn--accent"
          onClick={() => fileInput.current?.click()}
          aria-label="Upload files"
          title="Upload files"
        >
          <Upload size={16} aria-hidden="true" />
        </button>
        <input ref={fileInput} type="file" multiple hidden onChange={onUpload} />
      </div>
      <div className="kapi-pg-files-cwd" title={viewDir}>
        {viewDir}
      </div>
      <ul className="kapi-pg-file-list">
        {viewDir !== "/" && (
          <li className="kapi-pg-file-row">
            <button
              type="button"
              className="kapi-pg-dir-link"
              onClick={() => setViewDir(parentDir(viewDir))}
            >
              <CornerLeftUp size={14} aria-hidden="true" />
              <span>..</span>
            </button>
          </li>
        )}
        {entries.length === 0 && (
          <li className="kapi-pg-empty">empty — upload a file or run a command</li>
        )}
        {entries.map((name) => {
          const isDir = runtime.vol.isDir(joinPath(viewDir, name));
          return (
            <li key={name} className="kapi-pg-file-row">
              {isDir ? (
                <button type="button" className="kapi-pg-dir-link" onClick={() => enterDir(name)}>
                  <FolderOpen size={14} aria-hidden="true" />
                  <span>{name}/</span>
                </button>
              ) : (
                <>
                  <button
                    type="button"
                    className="kapi-pg-file-name"
                    title="Preview with the kapi parser"
                    onClick={() => setPreviewPath(joinPath(viewDir, name))}
                  >
                    <FileText size={14} aria-hidden="true" className="kapi-pg-file-icon" />
                    <span className="kapi-pg-file-label">{name}</span>
                  </button>
                  <span className="kapi-pg-file-actions">
                    <button
                      type="button"
                      className="kapi-pg-icon-btn"
                      onClick={() => download(name)}
                      aria-label={`Download ${name}`}
                      title="Download"
                    >
                      <Download size={15} aria-hidden="true" />
                    </button>
                    <button
                      type="button"
                      className="kapi-pg-icon-btn kapi-pg-icon-btn--danger"
                      onClick={() => remove(name)}
                      aria-label={`Delete ${name}`}
                      title="Delete"
                    >
                      <Trash2 size={15} aria-hidden="true" />
                    </button>
                  </span>
                </>
              )}
            </li>
          );
        })}
      </ul>
      {previewPath && (
        <FilePreview runtime={runtime} path={previewPath} onClose={() => setPreviewPath(null)} />
      )}
    </div>
  );
}
