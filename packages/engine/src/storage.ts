// Where the engine keeps what a page gives it.
//
// In a dedicated Worker, the engine keeps its databases and the files of its
// file system in the origin private file system, through SQLite's
// opfs-sahpool VFS, so a workspace outlives the page and the browser. One tab
// owns the pool at a time: a Web Lock names the owner. A tab that finds the
// lock held runs in memory and says so, waits for it, or takes it over, as
// its page asks (WhenHeld). A takeover steals the lock: the owner's Worker
// sees its lock released, keeps its last changes and tells its page, which
// stops it so the pool's files are free for the new owner. Everywhere else,
// and wherever the pool cannot be opened, everything lives in memory for the
// life of the page.
//
// The browser may still evict what a site keeps (Safari removes data a script
// wrote after seven days without a visit), so the workspace in a browser is a
// cache: export it as a .kpz to keep it elsewhere.
//
// Kept to erasable TypeScript syntax so it runs under Node's type stripping.

import type { SAHPoolUtil, Sqlite3Static } from "@sqlite.org/sqlite-wasm";
import type { MemFS } from "./memfs.ts";

/** Why an engine keeps its workspace in memory. */
export type MemoryReason =
  /** The page asked for no persistence. */
  | "disabled"
  /** The engine runs on the page's thread, where the pool cannot be opened. */
  | "main-thread"
  /** The browser lacks the origin private file system or Web Locks. */
  | "unsupported"
  /** Another tab of this site holds the workspace. */
  | "another-tab"
  /** This tab held the workspace until another tab took it over. */
  | "taken"
  /** Opening the pool failed; `detail` says how. */
  | "failed";

/** Where the engine keeps its databases and files. */
export interface StorageInfo {
  /**
   * `opfs`: in the origin private file system, kept across reloads and
   * browser restarts until the browser evicts them. `memory`: for the life of
   * the page.
   */
  kind: "opfs" | "memory";
  /** Why the workspace is in memory. Absent for `opfs`. */
  reason?: MemoryReason;
  /** The error behind `failed`. */
  detail?: string;
  /** The pool's name, which the storage key derives (`opfs` only). */
  name?: string;
}

/** A sentence a page can show about where its workspace is kept. */
export function describeStorage(info: StorageInfo): string {
  if (info.kind === "opfs") {
    return "Files and the workspace are kept in this browser. Export the workspace to keep a copy elsewhere.";
  }
  switch (info.reason) {
    case "another-tab":
      return "Another tab holds the workspace, so this tab keeps its files in memory and loses them when it closes.";
    case "taken":
      return "Another tab took over the workspace, so this tab keeps its files in memory and loses them when it closes.";
    case "unsupported":
      return "This browser cannot keep the workspace, so files last until the page closes.";
    case "failed":
      return "The workspace could not be opened, so files last until the page closes.";
    default:
      return "Files last until the page closes.";
  }
}

/** The bridge's database names under this prefix belong to the Worker. */
export const ENGINE_PREFIX = "/.kapi-engine/";

/** The Worker's store of the file system's files, in the pool. */
export const VOLUME_DB = `${ENGINE_PREFIX}volume.db`;

/** Directories the Worker never keeps: scratch space. */
const TRANSIENT = ["/tmp"];

/** A short, stable name for a storage key, safe in an OPFS path and a VFS name. */
export function storageName(key: string): string {
  let h = 0x811c9dc5;
  for (let i = 0; i < key.length; i++) {
    h ^= key.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return `kapi-${(h >>> 0).toString(16).padStart(8, "0")}`;
}

/** An opened pool and the lock that makes this tab its owner. */
export interface OpenedPool {
  pool: SAHPoolUtil;
  info: StorageInfo;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** How long a tab waits for the lock a reloading page is letting go of. */
const LOCK_WAIT_MS = 2500;

/**
 * What a tab does when another tab holds the workspace: run in memory
 * (`memory`, the default), wait until the owner lets go (`wait`: it closes,
 * or another tab takes the workspace over and closes), or take the workspace
 * over (`take`), which tells the owner it lost it.
 */
export type WhenHeld = "memory" | "wait" | "take";

/** The part of the Web Locks API the owner's lock uses. */
export interface OwnerLocks {
  request(
    name: string,
    options: { signal?: AbortSignal; steal?: boolean },
    callback: () => Promise<unknown>,
  ): Promise<unknown>;
}

/**
 * Take the Web Lock for `name`, held until the Worker ends, as `whenHeld`
 * says. Resolves to whether it was granted. `onLost` runs if another tab
 * later takes the lock over (a request with `steal`), which rejects the held
 * request with an AbortError.
 */
export function takeLock(
  locks: OwnerLocks,
  name: string,
  whenHeld: WhenHeld,
  onLost: () => void,
  wait = LOCK_WAIT_MS,
): Promise<boolean> {
  const hold = (options: { signal?: AbortSignal; steal?: boolean }) =>
    new Promise<boolean>((resolve) => {
      let granted = false;
      locks
        .request(name, options, () => {
          granted = true;
          resolve(true);
          // Held for the life of the Worker: the browser releases it then.
          return new Promise<never>(() => {});
        })
        .catch(() => {
          if (granted) onLost();
          else resolve(false);
        });
    });
  if (whenHeld === "wait") return hold({});
  // A reloading page lets go as it closes, so every mode waits a little first.
  return hold({ signal: AbortSignal.timeout(wait) }).then((ok) =>
    ok || whenHeld !== "take" ? ok : hold({ steal: true }),
  );
}

/**
 * Check that no other context holds the pool's files. Installing a pool whose
 * files another context still holds fails, and the failed install removes the
 * pool's directory, so the Worker checks first and waits for a page that is
 * closing to let go.
 */
async function poolFree(directory: string): Promise<boolean> {
  const root = await navigator.storage.getDirectory();
  let dir: FileSystemDirectoryHandle;
  try {
    dir = await root.getDirectoryHandle(directory);
    dir = await dir.getDirectoryHandle(".opaque");
  } catch {
    return true; // a new pool
  }
  // The async iterator is missing from the DOM typings this package builds with.
  const entries = (dir as unknown as { values(): AsyncIterable<FileSystemHandle> }).values();
  for await (const h of entries) {
    if (h.kind !== "file") continue;
    try {
      // Only a dedicated Worker has the method, so the DOM typings omit it.
      const file = h as unknown as { createSyncAccessHandle(): Promise<{ close(): void }> };
      const sah = await file.createSyncAccessHandle();
      sah.close();
    } catch {
      return false;
    }
  }
  return true;
}

/** How opening a pool goes when another tab holds it. */
export interface OpenPoolOptions {
  /** What to do when another tab holds the workspace. */
  whenHeld?: WhenHeld;
  /** Runs when another tab takes over the workspace this Worker owns. */
  onLost?: () => void;
}

/**
 * Open the opfs-sahpool pool for `key` in this Worker, or say why the
 * workspace stays in memory.
 */
export async function openPool(
  sqlite3: Sqlite3Static,
  key: string,
  opts: OpenPoolOptions = {},
): Promise<OpenedPool | StorageInfo> {
  const g = globalThis as { navigator?: Navigator; FileSystemFileHandle?: unknown };
  const hasOPFS =
    typeof g.FileSystemFileHandle === "function" &&
    "createSyncAccessHandle" in (g.FileSystemFileHandle as { prototype: object }).prototype &&
    typeof g.navigator?.storage?.getDirectory === "function";
  if (!hasOPFS || !g.navigator?.locks) return { kind: "memory", reason: "unsupported" };

  const name = storageName(key);
  const whenHeld = opts.whenHeld ?? "memory";
  const owned = await takeLock(
    g.navigator.locks as unknown as OwnerLocks,
    `${name}:owner`,
    whenHeld,
    () => opts.onLost?.(),
  );
  if (!owned) return { kind: "memory", reason: "another-tab" };
  const directory = `.${name}`;
  // A tab taking the workspace over waits longer: the owner has to notice,
  // keep its last changes and stop before the pool's files are free.
  const attempts = whenHeld === "take" ? 60 : 10;
  try {
    for (let attempt = 0; !(await poolFree(directory)); attempt++) {
      if (attempt >= attempts)
        return { kind: "memory", reason: "failed", detail: "the pool's files are held elsewhere" };
      await sleep(250);
    }
    const pool = await sqlite3.installOpfsSAHPoolVfs({ name, directory, initialCapacity: 64 });
    await pool.reserveMinimumCapacity(64);
    return { pool, info: { kind: "opfs", name } };
  } catch (e) {
    return { kind: "memory", reason: "failed", detail: e instanceof Error ? e.message : String(e) };
  }
}

/** Keep spare slots in the pool: a new database needs one, and its journal one more. */
export async function keepCapacity(pool: SAHPoolUtil): Promise<void> {
  const free = Number(pool.getCapacity()) - Number(pool.getFileCount());
  if (free < 24) await pool.addCapacity(32);
}

// ── The file system's changes ───────────────────────────────────────────────

/** One change to the file system, as the Worker reports it. */
export type VolOp =
  | { op: "dir"; path: string }
  | { op: "file"; path: string; data: Uint8Array; mtime: number }
  | { op: "rm"; path: string };

const transient = (p: string) => TRANSIENT.some((t) => p === t || p.startsWith(`${t}/`));

/**
 * The state of each changed path as operations: a file's bytes, a directory
 * with everything under it (a renamed directory brings its tree), or its
 * removal. A parent's change leaves its children's own changes to them.
 */
export function changesOf(mem: MemFS, paths: Iterable<string>): VolOp[] {
  const ops: VolOp[] = [];
  const seen = new Set<string>();
  const visit = (p: string) => {
    if (seen.has(p)) return;
    seen.add(p);
    if (!mem.vol.exists(p)) {
      ops.push({ op: "rm", path: p });
    } else if (mem.vol.isDir(p)) {
      ops.push({ op: "dir", path: p });
      for (const name of mem.vol.readdir(p)) visit(p === "/" ? `/${name}` : `${p}/${name}`);
    } else {
      ops.push({
        op: "file",
        path: p,
        data: mem.vol.readFile(p),
        mtime: mem.mtime(p) ?? Date.now(),
      });
    }
  };
  for (const p of [...paths].sort()) visit(p);
  return ops;
}

/** Apply changes to a file system, quietly when it reports its own. */
export function applyChanges(mem: MemFS, ops: VolOp[]): void {
  for (const o of ops) {
    try {
      if (o.op === "dir") {
        mem.vol.mkdirp(o.path);
      } else if (o.op === "file") {
        const slash = o.path.lastIndexOf("/");
        if (slash > 0) mem.vol.mkdirp(o.path.slice(0, slash));
        if (mem.vol.isDir(o.path)) mem.vol.remove(o.path);
        mem.vol.writeFile(o.path, o.data);
        mem.setMtime(o.path, o.mtime);
      } else if (mem.vol.exists(o.path)) {
        mem.vol.remove(o.path);
      }
    } catch {
      /* a change to a path whose parent is gone is moot */
    }
  }
}

/**
 * The file system's files, kept in a database of the pool so they outlive the
 * page with the databases beside them.
 */
export class VolumeStore {
  private readonly db: {
    exec(
      sql: string | { sql: string; bind?: unknown[]; rowMode?: string; returnValue?: string },
    ): unknown;
    selectArrays(sql: string): unknown[][];
  };

  constructor(pool: SAHPoolUtil) {
    this.db = new pool.OpfsSAHPoolDb(VOLUME_DB) as never;
    this.db.exec(
      "CREATE TABLE IF NOT EXISTS files (path TEXT PRIMARY KEY, dir INTEGER NOT NULL, data BLOB, mtime REAL NOT NULL)",
    );
  }

  /** Write every kept file and directory into `mem`. Answers how many entries. */
  restore(mem: MemFS): number {
    const rows = this.db.selectArrays("SELECT path, dir, data, mtime FROM files ORDER BY path");
    const ops: VolOp[] = rows.map(([path, dir, data, mtime]) =>
      dir
        ? { op: "dir", path: path as string }
        : {
            op: "file",
            path: path as string,
            data: (data as Uint8Array | null) ?? new Uint8Array(0),
            mtime: mtime as number,
          },
    );
    applyChanges(mem, ops);
    return rows.length;
  }

  /** Keep the changes, in one transaction. */
  save(ops: VolOp[]): void {
    const kept = ops.filter((o) => !transient(o.path));
    if (!kept.length) return;
    this.db.exec("BEGIN");
    try {
      for (const o of kept) {
        if (o.op === "rm") {
          // The path and everything below it: '/' sorts just before '0'.
          this.db.exec({
            sql: "DELETE FROM files WHERE path = ?1 OR (path >= ?1 || '/' AND path < ?1 || '0')",
            bind: [o.path],
          });
        } else {
          this.db.exec({
            sql: "INSERT INTO files (path, dir, data, mtime) VALUES (?, ?, ?, ?) ON CONFLICT(path) DO UPDATE SET dir = excluded.dir, data = excluded.data, mtime = excluded.mtime",
            bind: o.op === "dir" ? [o.path, 1, null, Date.now()] : [o.path, 0, o.data, o.mtime],
          });
        }
      }
      this.db.exec("COMMIT");
    } catch (e) {
      this.db.exec("ROLLBACK");
      throw e;
    }
  }
}
