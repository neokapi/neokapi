import React, { useRef, useState } from "react";
import type { KapiCli } from "./_wasmCli";
import FilePreview from "./_FilePreview";
import styles from "./styles.module.css";

function joinPath(dir: string, name: string): string {
  return dir.replace(/\/$/, "") + "/" + name;
}

export default function FilesPanel({
  cli,
  refreshKey,
  onChange,
}: {
  cli: KapiCli;
  refreshKey: number;
  onChange: () => void;
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [previewPath, setPreviewPath] = useState<string | null>(null);
  const cwd = cli.cwd();

  // refreshKey is a dependency only to force re-render when the fs changes.
  void refreshKey;

  let entries: string[] = [];
  try {
    entries = cli.vol.readdir(cwd);
  } catch {
    entries = [];
  }

  async function onUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(e.target.files || []);
    for (const f of files) {
      const buf = new Uint8Array(await f.arrayBuffer());
      cli.vol.writeFile(joinPath(cwd, f.name), buf);
    }
    if (fileInput.current) fileInput.current.value = "";
    onChange();
  }

  function download(name: string) {
    const data = cli.vol.readFile(joinPath(cwd, name));
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
    cli.vol.remove(joinPath(cwd, name));
    onChange();
  }

  function enter(name: string) {
    cli.chdir(joinPath(cwd, name));
    onChange();
  }

  return (
    <div className={styles.files}>
      <div className={styles.filesHeader}>
        <span className={styles.filesTitle}>Files</span>
        <button type="button" className="button button--sm button--primary" onClick={() => fileInput.current?.click()}>
          Upload
        </button>
        <input ref={fileInput} type="file" multiple hidden onChange={onUpload} />
      </div>
      <div className={styles.filesCwd}>{cwd}</div>
      <ul className={styles.fileList}>
        {cwd !== "/" && (
          <li className={styles.fileRow}>
            <button type="button" className={styles.dirLink} onClick={() => { cli.chdir(".."); onChange(); }}>
              ../
            </button>
          </li>
        )}
        {entries.length === 0 && <li className={styles.empty}>empty — upload a file or run a command</li>}
        {entries.map((name) => {
          const isDir = cli.vol.isDir(joinPath(cwd, name));
          return (
            <li key={name} className={styles.fileRow}>
              {isDir ? (
                <button type="button" className={styles.dirLink} onClick={() => enter(name)}>
                  {name}/
                </button>
              ) : (
                <>
                  <button type="button" className={styles.fileName} title="Preview with the kapi parser" onClick={() => setPreviewPath(joinPath(cwd, name))}>
                    {name}
                  </button>
                  <span className={styles.fileActions}>
                    <button type="button" className={styles.linkBtn} onClick={() => download(name)}>
                      download
                    </button>
                    <button type="button" className={styles.linkBtn} onClick={() => remove(name)}>
                      delete
                    </button>
                  </span>
                </>
              )}
            </li>
          );
        })}
      </ul>
      {previewPath && <FilePreview cli={cli} path={previewPath} onClose={() => setPreviewPath(null)} />}
    </div>
  );
}
