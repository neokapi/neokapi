import React, { useEffect, useRef, useState } from "react";
import {
  ArrowRightLeft,
  Download,
  Trash2,
  Upload,
  FolderOpen,
  FolderSync,
  FileText,
  CornerLeftUp,
  PackageOpen,
  Package,
} from "lucide-react";
import { describeStorage } from "./runtime";
import type { FolderHandle, KapiRuntime } from "./runtime";
import FilePreview from "./FilePreview";

function joinPath(dir: string, name: string): string {
  return dir.replace(/\/$/, "") + "/" + name;
}

// The root of the project at or above dir: the nearest directory with a
// kapi.yaml, or null outside every project.
function projectAt(runtime: KapiRuntime, dir: string): string | null {
  for (let d = dir; ; d = parentDir(d)) {
    if (runtime.vol.exists(joinPath(d, "kapi.yaml"))) return d;
    if (d === "/") return null;
  }
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
  const packageInput = useRef<HTMLInputElement>(null);
  const [workspaceBusy, setWorkspaceBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadStatus, setUploadStatus] = useState("");
  const [uploadError, setUploadError] = useState("");
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

  // Another tab may take the workspace over, and this tab may take it back:
  // either changes where the files live, so the panel draws again.
  useEffect(() => runtime.onStorageChange?.(() => onChange()), [runtime, onChange]);

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
    const destination = viewDir;
    setUploading(true);
    setUploadError("");
    setUploadStatus("");
    const added: string[] = [];
    const failures: string[] = [];
    for (const file of files) {
      try {
        const bytes = new Uint8Array(await file.arrayBuffer());
        const original = file.name.replace(/[\\/]/g, "_");
        if (!original || original === "." || original === "..") {
          throw new Error("Choose a file with a valid name.");
        }
        const dot = original.lastIndexOf(".");
        const stem = dot > 0 ? original.slice(0, dot) : original;
        const extension = dot > 0 ? original.slice(dot) : "";
        let name = original;
        let suffix = 1;
        while (runtime.vol.exists(joinPath(destination, name))) {
          name = `${stem} (${suffix++})${extension}`;
        }
        runtime.vol.writeFile(joinPath(destination, name), bytes);
        added.push(name);
      } catch (error) {
        failures.push(`${file.name}: ${error instanceof Error ? error.message : String(error)}`);
      }
    }
    if (fileInput.current) fileInput.current.value = "";
    setUploading(false);
    setUploadStatus(added.length ? `Added to ${destination}: ${added.join(", ")}.` : "");
    setUploadError(failures.join(" "));
    onChange();
  }

  function save(name: string, data: Uint8Array) {
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

  function download(name: string) {
    save(name, runtime.vol.readFile(joinPath(viewDir, name)));
  }

  // The workspace in a browser is a cache: an export keeps the files and each
  // project's context somewhere else, and an import reads them back.
  async function exportWorkspace() {
    setWorkspaceBusy(true);
    setUploadError("");
    setUploadStatus("");
    try {
      const out = await runtime.exportWorkspace();
      save("workspace.kpz", out.data);
      const skipped = out.skipped.map((s) => `${s.root}: ${s.reason}`).join(" ");
      const stores = out.termStores.length
        ? `, and ${out.termStores.length} terms stores outside a project`
        : "";
      setUploadStatus(
        `Exported ${out.files} files and the context of ${out.projects.length} projects${stores} to workspace.kpz.`,
      );
      if (skipped) setUploadError(`Left out the context of ${skipped}`);
    } catch (error) {
      setUploadError(error instanceof Error ? error.message : String(error));
    } finally {
      setWorkspaceBusy(false);
    }
  }

  async function importWorkspace(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (packageInput.current) packageInput.current.value = "";
    if (!file) return;
    setWorkspaceBusy(true);
    setUploadError("");
    setUploadStatus("");
    try {
      const res = await runtime.importWorkspace(new Uint8Array(await file.arrayBuffer()));
      const merged = res.projects.reduce((n, p) => n + p.merged, 0);
      const stores = res.termStores.length
        ? `, and wrote ${res.termStores.length} terms stores`
        : "";
      setUploadStatus(
        `Imported ${res.files} files and ${merged} context operations into ${res.projects.length} projects${stores}.`,
      );
    } catch (error) {
      setUploadError(error instanceof Error ? error.message : String(error));
    } finally {
      setWorkspaceBusy(false);
      onChange();
    }
  }

  // A tab that another tab holds the workspace for can take it over; the
  // other tab carries on in memory and says so.
  async function takeOver() {
    setWorkspaceBusy(true);
    setUploadError("");
    setUploadStatus("");
    try {
      await runtime.takeOver();
      setUploadStatus("This tab now holds the workspace kept in this browser.");
    } catch (error) {
      setUploadError(error instanceof Error ? error.message : String(error));
    } finally {
      setWorkspaceBusy(false);
      onChange();
    }
  }

  // A project's context can be shared through a folder on this computer: the
  // folder keeps the layout a `file` context backend keeps, so kapi on the
  // same machine, or a sync client, shares it. The folder chosen is kept for
  // the next sync in this page.
  const folderRef = useRef<FolderHandle | null>(null);
  const projectRoot = projectAt(runtime, viewDir);
  const canPickFolder =
    typeof window !== "undefined" &&
    typeof (window as { showDirectoryPicker?: unknown }).showDirectoryPicker === "function";

  async function syncContext() {
    if (!projectRoot) return;
    setWorkspaceBusy(true);
    setUploadError("");
    setUploadStatus("");
    try {
      if (!folderRef.current) {
        const host = window as unknown as {
          showDirectoryPicker(o: object): Promise<FolderHandle>;
        };
        folderRef.current = await host.showDirectoryPicker({
          id: "kapi-context",
          mode: "readwrite",
        });
      }
      const res = await runtime.syncContext(folderRef.current, { project: projectRoot });
      const merged = res.pull?.merged ?? 0;
      const pushed = res.push?.pushed ?? 0;
      setUploadStatus(
        `Synced the context of ${projectRoot} with ${folderRef.current.name}: pulled ${merged} and pushed ${pushed} operations.`,
      );
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") return;
      folderRef.current = null;
      setUploadError(error instanceof Error ? error.message : String(error));
    } finally {
      setWorkspaceBusy(false);
      onChange();
    }
  }

  const heldElsewhere =
    runtime.storage?.kind === "memory" &&
    (runtime.storage.reason === "another-tab" || runtime.storage.reason === "taken");

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
          className="kapi-pg-btn kapi-pg-btn--sm"
          disabled={uploading}
          onClick={() => fileInput.current?.click()}
          aria-label="Upload files"
          title="Upload files"
        >
          <Upload size={16} aria-hidden="true" />
          <span>{uploading ? "Adding files…" : "Upload files"}</span>
        </button>
        <input
          ref={fileInput}
          aria-label="Choose files to upload"
          type="file"
          multiple
          hidden
          disabled={uploading}
          onChange={onUpload}
        />
      </div>
      <div className="kapi-pg-files-header">
        <button
          type="button"
          className="kapi-pg-btn kapi-pg-btn--sm"
          disabled={workspaceBusy}
          onClick={() => void exportWorkspace()}
          title="Save the files and each project's context as a .kpz"
        >
          <Package size={16} aria-hidden="true" />
          <span>Export workspace</span>
        </button>
        <button
          type="button"
          className="kapi-pg-btn kapi-pg-btn--sm"
          disabled={workspaceBusy}
          onClick={() => packageInput.current?.click()}
          title="Read a workspace .kpz back into this page"
        >
          <PackageOpen size={16} aria-hidden="true" />
          <span>Import workspace</span>
        </button>
        <input
          ref={packageInput}
          aria-label="Choose a workspace package to import"
          type="file"
          accept=".kpz"
          hidden
          disabled={workspaceBusy}
          onChange={(e) => void importWorkspace(e)}
        />
      </div>
      {runtime.storage && (
        <p className="kapi-pg-files-help" data-storage={runtime.storage.kind}>
          {describeStorage(runtime.storage)}
        </p>
      )}
      {canPickFolder && (
        <div className="kapi-pg-files-header">
          <button
            type="button"
            className="kapi-pg-btn kapi-pg-btn--sm"
            disabled={workspaceBusy || !projectRoot}
            onClick={() => void syncContext()}
            title={
              projectRoot
                ? "Pull and push this project's context through a folder on this computer"
                : "Open a folder with a kapi.yaml to sync its project's context"
            }
          >
            <FolderSync size={16} aria-hidden="true" />
            <span>Sync context with a folder</span>
          </button>
        </div>
      )}
      {heldElsewhere && (
        <div className="kapi-pg-files-header">
          <button
            type="button"
            className="kapi-pg-btn kapi-pg-btn--sm"
            disabled={workspaceBusy}
            onClick={() => void takeOver()}
            title="Open the workspace kept in this browser here; the other tab carries on in memory"
          >
            <ArrowRightLeft size={16} aria-hidden="true" />
            <span>Use the workspace here</span>
          </button>
        </div>
      )}
      <div className="kapi-pg-files-cwd" title={viewDir}>
        {viewDir}
      </div>
      <p className="kapi-pg-files-help">
        Use uploaded filenames in commands. Duplicate names get a numbered suffix.
      </p>
      {uploadStatus && (
        <p className="kapi-pg-files-help" role="status">
          {uploadStatus}
        </p>
      )}
      {uploadError && (
        <p className="kapi-pg-files-help" role="alert">
          {uploadError}
        </p>
      )}
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
