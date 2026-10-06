// The calls an engine answers beside its globals, over its file system and
// the SQLite bridge. They run where the engine runs: on the page's thread, or
// in the engine's Worker.

import type { MemFS } from "./memfs.ts";
import { folderRemote } from "./folderremote.ts";
import type { FolderHandle } from "./folderremote.ts";
import "./globals.ts";

/**
 * The calls an engine answers beside its globals, over its file system and
 * the SQLite bridge. The Worker runs them where the engine is.
 */
export function localCalls(mem: MemFS): Record<string, (...args: never[]) => unknown> {
  return {
    /** Remove the database at `path`: false when none is held there. */
    removeDatabase(path: string): boolean {
      const sql = globalThis.__kapiSQL;
      if (!sql?.exists(path)) return false;
      const res = sql.remove(path);
      if (typeof res === "object") throw new Error(res.err);
      return true;
    },
    /** Start `dir` over: the engine's reset, then its files. */
    async reset(dir: string): Promise<void> {
      const fn = globalThis.kapiReset;
      if (typeof fn === "function") {
        const failure = await fn(dir);
        if (failure) throw new Error(failure);
      }
      const base = dir.replace(/\/$/, "");
      try {
        for (const name of mem.vol.readdir(dir)) mem.vol.remove(`${base}/${name}`);
      } catch {
        /* nothing to clear */
      }
    },
    /**
     * Sync a project's context with a folder: the engine's kapiSyncContext
     * over a remote on the folder's handle, which a Worker receives intact.
     */
    async syncContext(folder: FolderHandle, options: string): Promise<string> {
      const fn = globalThis.kapiSyncContext;
      if (typeof fn !== "function") {
        throw new Error("this engine predates context sync (kapiSyncContext)");
      }
      return await fn(folderRemote(folder), options);
    },
    /** Read a file once and remove it (a trace a lab reads back). */
    take(path: string): Uint8Array | null {
      try {
        const data = mem.vol.readFile(path);
        mem.vol.remove(path);
        return data;
      } catch {
        return null;
      }
    },
  };
}

/** Call a local call or an engine global on this thread. */
export async function invokeEngine(
  local: Record<string, (...args: never[]) => unknown>,
  fn: string,
  args: unknown[],
): Promise<unknown> {
  const own = local[fn];
  if (own) return await (own as (...a: unknown[]) => unknown)(...args);
  const target = (globalThis as Record<string, unknown>)[fn];
  if (typeof target !== "function") throw new Error(`engine global ${fn} is not registered`);
  return await (target as (...a: unknown[]) => unknown)(...args);
}
