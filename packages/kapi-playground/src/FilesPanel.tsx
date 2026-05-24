import React, { useRef, useState } from "react";
import { Download, Trash2, Upload, FolderOpen, FileText, CornerLeftUp } from "lucide-react";
import type { KapiRuntime } from "./runtime";
import FilePreview from "./FilePreview";

function joinPath(dir: string, name: string): string {
  return dir.replace(/\/$/, "") + "/" + name;
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
  const cwd = runtime.cwd();

  // refreshKey is a dependency only to force re-render when the fs changes.
  void refreshKey;

  let entries: string[] = [];
  try {
    entries = runtime.vol.readdir(cwd);
  } catch {
    entries = [];
  }

  async function onUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files || []);
    for (const f of files) {
      const buf = new Uint8Array(await f.arrayBuffer());
      runtime.vol.writeFile(joinPath(cwd, f.name), buf);
    }
    if (fileInput.current) fileInput.current.value = "";
    onChange();
  }

  function download(name: string) {
    const data = runtime.vol.readFile(joinPath(cwd, name));
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
    runtime.vol.remove(joinPath(cwd, name));
    onChange();
  }

  function enter(name: string) {
    runtime.chdir(joinPath(cwd, name));
    onChange();
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
      <div className="kapi-pg-files-cwd" title={cwd}>
        {cwd}
      </div>
      <ul className="kapi-pg-file-list">
        {cwd !== "/" && (
          <li className="kapi-pg-file-row">
            <button
              type="button"
              className="kapi-pg-dir-link"
              onClick={() => {
                runtime.chdir("..");
                onChange();
              }}
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
          const isDir = runtime.vol.isDir(joinPath(cwd, name));
          return (
            <li key={name} className="kapi-pg-file-row">
              {isDir ? (
                <button type="button" className="kapi-pg-dir-link" onClick={() => enter(name)}>
                  <FolderOpen size={14} aria-hidden="true" />
                  <span>{name}/</span>
                </button>
              ) : (
                <>
                  <button
                    type="button"
                    className="kapi-pg-file-name"
                    title="Preview with the kapi parser"
                    onClick={() => setPreviewPath(joinPath(cwd, name))}
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
