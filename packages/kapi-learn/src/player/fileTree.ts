// Reading the sandbox for the files pane: the tree under the lab's directory
// and a fingerprint per file, so the pane can say what a command changed.

import type { MemVolume } from "@neokapi/engine";

/** Directories the pane leaves out unless asked: kapi's own cache, dependencies. */
export const HIDDEN_DIRS = new Set([".kapi", "node_modules", ".git"]);

export interface ListedFile {
  /** Path relative to the root. */
  path: string;
  size: number;
}

/** Every file under `root`, sorted by path, as relative paths. */
export function listFiles(vol: MemVolume, root: string, showHidden = false): ListedFile[] {
  const out: ListedFile[] = [];
  const base = root.replace(/\/$/, "");
  const walk = (dir: string, rel: string) => {
    let names: string[];
    try {
      names = vol.readdir(dir);
    } catch {
      return;
    }
    for (const name of names.slice().sort()) {
      const abs = `${dir}/${name}`;
      const childRel = rel ? `${rel}/${name}` : name;
      if (vol.isDir(abs)) {
        if (!showHidden && HIDDEN_DIRS.has(name)) continue;
        walk(abs, childRel);
      } else {
        let size = 0;
        try {
          size = vol.readFile(abs).length;
        } catch {
          /* unreadable: list it with no size */
        }
        out.push({ path: childRel, size });
      }
    }
  };
  walk(base, "");
  return out;
}

/** A cheap content fingerprint (FNV-1a over the bytes, plus the length). */
export function fingerprint(bytes: Uint8Array): string {
  let h = 0x811c9dc5;
  for (let i = 0; i < bytes.length; i++) {
    h ^= bytes[i];
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return `${bytes.length}:${h.toString(16)}`;
}

/** Fingerprints of every file under `root` (hidden directories included, so a change there counts). */
export function fingerprintTree(vol: MemVolume, root: string): Map<string, string> {
  const map = new Map<string, string>();
  for (const f of listFiles(vol, root, true)) {
    try {
      map.set(f.path, fingerprint(vol.readFile(`${root.replace(/\/$/, "")}/${f.path}`)));
    } catch {
      /* skip */
    }
  }
  return map;
}

/** Paths whose fingerprint differs between two snapshots, or exist in one only. */
export function changedPaths(before: Map<string, string>, after: Map<string, string>): Set<string> {
  const changed = new Set<string>();
  for (const [p, fp] of after) if (before.get(p) !== fp) changed.add(p);
  for (const p of before.keys()) if (!after.has(p)) changed.add(p);
  return changed;
}

/** A tree for rendering: directories first, then files, both sorted. */
export interface TreeNode {
  name: string;
  path: string;
  dir: boolean;
  children?: TreeNode[];
  size?: number;
}

export function buildTree(files: ListedFile[]): TreeNode[] {
  const root: TreeNode = { name: "", path: "", dir: true, children: [] };
  for (const f of files) {
    const parts = f.path.split("/");
    let node = root;
    for (let i = 0; i < parts.length; i++) {
      const name = parts[i];
      const last = i === parts.length - 1;
      const path = parts.slice(0, i + 1).join("/");
      let child = node.children!.find((c) => c.name === name);
      if (!child) {
        child = last
          ? { name, path, dir: false, size: f.size }
          : { name, path, dir: true, children: [] };
        node.children!.push(child);
      }
      node = child;
    }
  }
  const sort = (nodes: TreeNode[]) => {
    nodes.sort((a, b) => (a.dir === b.dir ? a.name.localeCompare(b.name) : a.dir ? -1 : 1));
    for (const n of nodes) if (n.children) sort(n.children);
  };
  sort(root.children!);
  return root.children!;
}
