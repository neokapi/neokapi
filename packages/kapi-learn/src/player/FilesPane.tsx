import React, { useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Download, Eye, EyeOff, FolderOpen, Upload } from "lucide-react";
import { CodeView, DocumentViewer, downloadBytes } from "@neokapi/ui-primitives/preview";
import type { ContentTree } from "@neokapi/ui-primitives/preview";
import type { InspectResult } from "@neokapi/kapi-playground/runtime";
import type { ChapterLook } from "../curriculum/types.ts";
import { buildTree, type TreeNode } from "./fileTree.ts";
import type { FileEntry } from "./types.ts";

// The files pane: the sandbox as a tree, with the files a command changed
// marked, and the selected file read through the engine (the preview, the
// blocks, the raw bytes) or shown as text when kapi has no reader for it.

export interface FilesPaneProps {
  files: FileEntry[];
  lastChanged: ReadonlySet<string>;
  selected: string | null;
  onSelect: (path: string | null) => void;
  /** Which reading to open the selected file in, from the chapter. */
  view?: ChapterLook["view"];
  showHidden: boolean;
  onShowHidden: (v: boolean) => void;
  readFile: (path: string) => Uint8Array | null;
  inspect: (path: string) => Promise<InspectResult>;
  onAddFiles?: (files: { name: string; bytes: Uint8Array }[]) => void;
  /** The pane re-reads the selected file when this changes. */
  version: number;
}

const dec = new TextDecoder();

function Tree({
  nodes,
  depth,
  selected,
  changed,
  lastChanged,
  open,
  toggle,
  onSelect,
}: {
  nodes: TreeNode[];
  depth: number;
  selected: string | null;
  changed: ReadonlySet<string>;
  lastChanged: ReadonlySet<string>;
  open: ReadonlySet<string>;
  toggle: (path: string) => void;
  onSelect: (path: string) => void;
}): React.ReactElement {
  return (
    <ul className="kl-tree" role={depth === 0 ? "tree" : "group"}>
      {nodes.map((node) => {
        if (node.dir) {
          const isOpen = open.has(node.path);
          const dirChanged = [...changed].some((p) => p.startsWith(node.path + "/"));
          return (
            <li key={node.path} role="treeitem" aria-expanded={isOpen}>
              <button
                type="button"
                className="kl-tree__row kl-tree__row--dir"
                style={{ paddingLeft: `${8 + depth * 14}px` }}
                onClick={() => toggle(node.path)}
              >
                {isOpen ? (
                  <ChevronDown size={14} aria-hidden="true" />
                ) : (
                  <ChevronRight size={14} aria-hidden="true" />
                )}
                <span className="kl-tree__name">{node.name}/</span>
                {dirChanged && !isOpen && <span className="kl-dot" aria-label="changed inside" />}
              </button>
              {isOpen && node.children && (
                <Tree
                  nodes={node.children}
                  depth={depth + 1}
                  selected={selected}
                  changed={changed}
                  lastChanged={lastChanged}
                  open={open}
                  toggle={toggle}
                  onSelect={onSelect}
                />
              )}
            </li>
          );
        }
        const isSelected = selected === node.path;
        const isLast = lastChanged.has(node.path);
        const isChanged = changed.has(node.path);
        return (
          <li key={node.path} role="treeitem" aria-selected={isSelected}>
            <button
              type="button"
              className={`kl-tree__row${isSelected ? " kl-tree__row--selected" : ""}${isLast ? " kl-tree__row--fresh" : ""}`}
              style={{ paddingLeft: `${22 + depth * 14}px` }}
              onClick={() => onSelect(node.path)}
              title={node.path}
            >
              <span className="kl-tree__name">{node.name}</span>
              {isChanged && <span className="kl-dot" aria-label="changed since the lab started" />}
            </button>
          </li>
        );
      })}
    </ul>
  );
}

const VIEW_TAB: Record<NonNullable<ChapterLook["view"]>, "preview" | "blocks" | "raw"> = {
  preview: "preview",
  blocks: "blocks",
  raw: "raw",
};

export default function FilesPane({
  files,
  lastChanged,
  selected,
  onSelect,
  view,
  showHidden,
  onShowHidden,
  readFile,
  inspect,
  onAddFiles,
  version,
}: FilesPaneProps): React.ReactElement {
  const tree = useMemo(() => buildTree(files), [files]);
  const changed = useMemo(
    () => new Set(files.filter((f) => f.changed).map((f) => f.path)),
    [files],
  );
  const [open, setOpen] = useState<Set<string>>(() => new Set());
  const [doc, setDoc] = useState<{
    path: string;
    tree: ContentTree | null;
    text: string | null;
    bytes: Uint8Array | null;
    error?: string;
  } | null>(null);
  const inputRef = React.useRef<HTMLInputElement>(null);

  // Open every directory on the way to the selected file.
  useEffect(() => {
    if (!selected) return;
    const parts = selected.split("/");
    if (parts.length < 2) return;
    setOpen((prev) => {
      const next = new Set(prev);
      for (let i = 1; i < parts.length; i++) next.add(parts.slice(0, i).join("/"));
      return next;
    });
  }, [selected]);

  // Read the selected file through the engine; fall back to its text.
  useEffect(() => {
    if (!selected) {
      setDoc(null);
      return;
    }
    let cancelled = false;
    const bytes = readFile(selected);
    if (!bytes) {
      setDoc({ path: selected, tree: null, text: null, bytes: null, error: "This file is gone." });
      return;
    }
    let text: string | null = null;
    try {
      text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    } catch {
      text = null;
    }
    const wantsRaw =
      view === "raw" || /\.(jsonl|yaml|yml|csv|tmx|xliff|xlf|js|ts|kpz)$/i.test(selected);
    if (wantsRaw) {
      setDoc({ path: selected, tree: null, text, bytes });
      return;
    }
    setDoc({ path: selected, tree: null, text, bytes });
    void inspect(selected).then((res) => {
      if (cancelled) return;
      // The engine's generated ContentTree and the preview kit's refinement of
      // it are the same projection; the kit types a few fields more loosely.
      if (res.ok && res.tree)
        setDoc({ path: selected, tree: res.tree as unknown as ContentTree, text, bytes });
    });
    return () => {
      cancelled = true;
    };
  }, [selected, version, view, readFile, inspect]);

  const toggle = (path: string) =>
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });

  const onPick = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const list = Array.from(e.target.files ?? []);
    e.target.value = "";
    if (!onAddFiles || list.length === 0) return;
    const out: { name: string; bytes: Uint8Array }[] = [];
    for (const f of list) out.push({ name: f.name, bytes: new Uint8Array(await f.arrayBuffer()) });
    onAddFiles(out);
  };

  return (
    <div className="kl-files">
      <div className="kl-files__tree">
        <div className="kl-files__bar">
          <span className="kl-files__title">
            <FolderOpen size={14} aria-hidden="true" /> Files
          </span>
          <span className="kl-files__actions">
            {onAddFiles && (
              <>
                <button
                  type="button"
                  className="kl-iconbtn"
                  onClick={() => inputRef.current?.click()}
                  title="Add your own files to the sandbox"
                  aria-label="Add files"
                >
                  <Upload size={14} aria-hidden="true" />
                </button>
                <input
                  ref={inputRef}
                  type="file"
                  multiple
                  hidden
                  onChange={(e) => void onPick(e)}
                />
              </>
            )}
            <button
              type="button"
              className="kl-iconbtn"
              onClick={() => onShowHidden(!showHidden)}
              title={showHidden ? "Hide kapi's own files (.kapi)" : "Show kapi's own files (.kapi)"}
              aria-pressed={showHidden}
              aria-label="Toggle hidden files"
            >
              {showHidden ? (
                <EyeOff size={14} aria-hidden="true" />
              ) : (
                <Eye size={14} aria-hidden="true" />
              )}
            </button>
          </span>
        </div>
        <Tree
          nodes={tree}
          depth={0}
          selected={selected}
          changed={changed}
          lastChanged={lastChanged}
          open={open}
          toggle={toggle}
          onSelect={onSelect}
        />
        {files.length === 0 && <p className="kl-files__empty">No files yet.</p>}
      </div>
      <div className="kl-files__view">
        {!doc ? (
          <p className="kl-files__placeholder">
            Select a file to read it through the engine: its preview, its blocks, and the bytes on
            disk. A dot marks a file a command changed.
          </p>
        ) : doc.error ? (
          <p className="kl-files__placeholder">{doc.error}</p>
        ) : doc.tree ? (
          <DocumentViewer
            key={`${doc.path}:${view ?? "preview"}:${version}`}
            tree={doc.tree}
            filename={doc.path}
            bytes={doc.bytes}
            defaultTab={VIEW_TAB[view ?? "preview"]}
            className="kl-files__doc"
          />
        ) : (
          <div className="kl-files__raw">
            <div className="kl-files__rawbar">
              <code>{doc.path}</code>
              {doc.bytes && (
                <button
                  type="button"
                  className="kl-iconbtn"
                  onClick={() => downloadBytes(doc.path.split("/").pop() ?? "file", doc.bytes!)}
                  title="Download this file"
                  aria-label="Download this file"
                >
                  <Download size={14} aria-hidden="true" />
                </button>
              )}
            </div>
            {doc.text !== null ? (
              <CodeView text={doc.text} filename={doc.path} maxHeight="22rem" wrap />
            ) : (
              <p className="kl-files__placeholder">
                A binary file of {doc.bytes?.length ?? 0} bytes. Download it to open it.
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
