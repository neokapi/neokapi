import { useCallback, useMemo, useRef, useState } from "react";
import { SAMPLES } from "./samples";
import { matchGlob } from "./glob";

// The file library is the in-memory corpus a learner works with: the bundled
// samples, anything they upload, and the outputs a pipeline writes back. It is
// the single model behind the file explorer and the compact selector. Files are
// addressed by a virtual path (relative to the project root, e.g.
// "messages.json" or "out/messages.json"); folders are derived from those
// paths. Outputs overwrite by path and bump `changedAt`, which the output
// viewer uses to flash what just changed.

export type FileOrigin = "sample" | "upload" | "output";

export interface LibFile {
  /** Virtual path relative to the project root. */
  path: string;
  /** Basename of the path. */
  name: string;
  bytes: Uint8Array;
  origin: FileOrigin;
  /** Monotonic sequence at which the file was added (ordering + reveal anim). */
  addedAt: number;
  /** Monotonic sequence of the last overwrite (drives the change pulse). */
  changedAt: number;
}

export interface FileLibrary {
  files: LibFile[];
  paths: string[];
  /** Distinct folder paths derived from the files (no trailing slash). */
  folders: string[];
  get(path: string): LibFile | undefined;
  /** Add a bundled sample by id; returns its path, or null if unknown/present. */
  addSample(id: string): string | null;
  /** Add or overwrite a file; returns its path. Overwrites bump changedAt. */
  addFile(name: string, bytes: Uint8Array, origin?: FileOrigin): string;
  /** Add uploaded browser File objects; returns their paths. */
  upload(files: FileList | File[]): Promise<string[]>;
  /** Record a pipeline output at an absolute or relative path. */
  setOutput(path: string, bytes: Uint8Array): void;
  remove(path: string): void;
  /** Remove every file under a folder prefix. */
  removeFolder(dir: string): void;
  /** Remove all output files (e.g. before re-running a batch). */
  clearOutputs(): void;
}

const enc = new TextEncoder();

function basename(p: string): string {
  return p.replace(/\/+$/, "").split("/").pop() || p;
}

// Normalise a path into the library's relative form: strip a leading "/project"
// (the wasm runtime's root) and any leading slashes so the explorer shows clean,
// project-relative paths.
function normalisePath(p: string): string {
  let s = p.replace(/^\/?project\//, "").replace(/^\/+/, "");
  s = s.replace(/\/{2,}/g, "/");
  return s;
}

function deriveFolders(paths: string[]): string[] {
  const set = new Set<string>();
  for (const p of paths) {
    const parts = p.split("/");
    for (let i = 1; i < parts.length; i++) {
      set.add(parts.slice(0, i).join("/"));
    }
  }
  return [...set].sort();
}

export interface UseFileLibraryOptions {
  /** Restrict (and order) the samples seeded into the library. */
  sampleIds?: string[];
  /** Seed the bundled samples on first mount (default true). */
  seedSamples?: boolean;
}

export function useFileLibrary(opts: UseFileLibraryOptions = {}): FileLibrary {
  const { sampleIds, seedSamples = true } = opts;
  const seqRef = useRef(0);
  const nextSeq = () => ++seqRef.current;

  const [files, setFiles] = useState<LibFile[]>(() => {
    if (!seedSamples) return [];
    const chosen = sampleIds ? SAMPLES.filter((s) => sampleIds.includes(s.id)) : SAMPLES;
    return chosen.map((s) => ({
      path: s.filename,
      name: basename(s.filename),
      bytes: enc.encode(s.content),
      origin: "sample" as FileOrigin,
      addedAt: ++seqRef.current,
      changedAt: 0,
    }));
  });

  const get = useCallback(
    (path: string) => files.find((f) => f.path === normalisePath(path)),
    [files],
  );

  const addFile = useCallback(
    (name: string, bytes: Uint8Array, origin: FileOrigin = "upload"): string => {
      const path = normalisePath(name);
      setFiles((prev) => {
        const idx = prev.findIndex((f) => f.path === path);
        if (idx >= 0) {
          const next = prev.slice();
          next[idx] = { ...next[idx], bytes, changedAt: nextSeq() };
          return next;
        }
        return [
          ...prev,
          { path, name: basename(path), bytes, origin, addedAt: nextSeq(), changedAt: 0 },
        ];
      });
      return path;
    },
    [],
  );

  const addSample = useCallback(
    (id: string): string | null => {
      const s = SAMPLES.find((x) => x.id === id);
      if (!s) return null;
      return addFile(s.filename, enc.encode(s.content), "sample");
    },
    [addFile],
  );

  const upload = useCallback(
    async (list: FileList | File[]): Promise<string[]> => {
      const arr = Array.from(list);
      const out: string[] = [];
      for (const f of arr) {
        const bytes = new Uint8Array(await f.arrayBuffer());
        out.push(addFile(f.name, bytes, "upload"));
      }
      return out;
    },
    [addFile],
  );

  const setOutput = useCallback(
    (path: string, bytes: Uint8Array) => {
      addFile(path, bytes, "output");
    },
    [addFile],
  );

  const remove = useCallback((path: string) => {
    const p = normalisePath(path);
    setFiles((prev) => prev.filter((f) => f.path !== p));
  }, []);

  const removeFolder = useCallback((dir: string) => {
    const d = normalisePath(dir).replace(/\/+$/, "");
    const prefix = d + "/";
    setFiles((prev) => prev.filter((f) => f.path !== d && !f.path.startsWith(prefix)));
  }, []);

  const clearOutputs = useCallback(() => {
    setFiles((prev) => prev.filter((f) => f.origin !== "output"));
  }, []);

  const paths = useMemo(() => files.map((f) => f.path), [files]);
  const folders = useMemo(() => deriveFolders(paths), [paths]);

  // Memoize the returned object so its identity is stable across renders and
  // only changes when the file set does. Consumers put `library` in effect /
  // callback dependency arrays; an unstable identity would re-fire those every
  // render and can loop.
  return useMemo(
    () => ({ files, paths, folders, get, addSample, addFile, upload, setOutput, remove, removeFolder, clearOutputs }),
    [files, paths, folders, get, addSample, addFile, upload, setOutput, remove, removeFolder, clearOutputs],
  );
}

// ── Selection model ──────────────────────────────────────────────────────────

export type SelectionMode = "single" | "multi" | "glob";

export interface FileSelection {
  mode: SelectionMode;
  /** Explicit selected paths (single = length 1). Ignored when mode is glob. */
  paths: string[];
  /** Glob pattern, used when mode is "glob". */
  pattern?: string;
}

export const EMPTY_SELECTION: FileSelection = { mode: "single", paths: [] };

/** Resolve a selection against the library to the concrete files it picks. */
export function resolveSelection(sel: FileSelection, lib: FileLibrary): LibFile[] {
  if (sel.mode === "glob") {
    const matched = new Set(matchGlob(sel.pattern ?? "", lib.paths));
    return lib.files.filter((f) => matched.has(f.path));
  }
  const want = new Set(sel.paths);
  return lib.files.filter((f) => want.has(f.path));
}

/** A short, human description of the current selection for the compact field. */
export function selectionSummary(sel: FileSelection, lib: FileLibrary): string {
  const files = resolveSelection(sel, lib);
  if (sel.mode === "glob") {
    const pat = sel.pattern?.trim() || "(no pattern)";
    return `${pat} · ${files.length} match${files.length === 1 ? "" : "es"}`;
  }
  if (files.length === 0) return "No file selected";
  if (files.length === 1) return files[0].name;
  return `${files.length} files`;
}
