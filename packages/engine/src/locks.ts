// File locks for SQLite's opfs-sahpool VFS, within one thread.
//
// The pool's own xLock and xUnlock only remember the level asked for: the
// pool is for one connection at a time. The engine opens several connections
// to one database (a pool per store, ATTACH by path), and memdb, the VFS it
// uses in memory, locks between them, so a write beside another connection's
// transaction reports "database is locked" rather than proceeding. This module
// gives the pool the same behaviour. It keeps SQLite's five lock levels per
// database, as os_unix.c keeps them per inode for the connections of one
// process, and installs itself over the pool's methods. Nothing waits: a lock
// that cannot be granted answers SQLITE_BUSY, and the driver's busy timeout is
// zero.
//
// Kept to erasable TypeScript syntax so it runs under Node's type stripping.

import type { Sqlite3Static } from "@sqlite.org/sqlite-wasm";

export const LOCK_NONE = 0;
export const LOCK_SHARED = 1;
export const LOCK_RESERVED = 2;
export const LOCK_PENDING = 3;
export const LOCK_EXCLUSIVE = 4;

const SQLITE_OK = 0;
const SQLITE_BUSY = 5;

interface FileLocks {
  /** Handles holding at least SHARED. */
  shared: number;
  /** The handle holding RESERVED, PENDING or EXCLUSIVE, 0 for none. */
  reserved: number;
  pending: number;
  exclusive: number;
}

/**
 * The lock state of every database the pool holds, keyed by name, and the
 * level each open file handle holds. Pure bookkeeping, so it is tested
 * without a browser.
 */
export class LockTable {
  private readonly files = new Map<string, FileLocks>();
  private readonly held = new Map<number, { name: string; level: number }>();

  /** Record that handle `h` is open on the database `name`. */
  open(h: number, name: string): void {
    this.held.set(h, { name, level: LOCK_NONE });
    if (!this.files.has(name)) {
      this.files.set(name, { shared: 0, reserved: 0, pending: 0, exclusive: 0 });
    }
  }

  /** Release whatever `h` holds and forget it. */
  close(h: number): void {
    if (this.held.has(h)) this.unlock(h, LOCK_NONE);
    this.held.delete(h);
  }

  /** Whether `h` is a handle the table tracks. */
  tracks(h: number): boolean {
    return this.held.has(h);
  }

  /** The level `h` holds. */
  level(h: number): number {
    return this.held.get(h)?.level ?? LOCK_NONE;
  }

  /** Raise `h` to `level`, answering SQLITE_OK or SQLITE_BUSY. */
  lock(h: number, level: number): number {
    const hold = this.held.get(h);
    if (!hold) return SQLITE_OK;
    if (level <= hold.level) return SQLITE_OK;
    const f = this.files.get(hold.name)!;
    const other = (holder: number) => holder !== 0 && holder !== h;
    if (hold.level === LOCK_NONE) {
      // SHARED: refused while another handle writes or waits to.
      if (other(f.pending) || other(f.exclusive)) return SQLITE_BUSY;
      f.shared++;
      hold.level = LOCK_SHARED;
      if (level === LOCK_SHARED) return SQLITE_OK;
    }
    if (hold.level < LOCK_RESERVED) {
      if (other(f.reserved) || other(f.pending) || other(f.exclusive)) return SQLITE_BUSY;
      f.reserved = h;
      hold.level = LOCK_RESERVED;
      if (level === LOCK_RESERVED) return SQLITE_OK;
    }
    // PENDING keeps new readers out; EXCLUSIVE waits for the readers there.
    if (other(f.exclusive)) return SQLITE_BUSY;
    f.pending = h;
    hold.level = LOCK_PENDING;
    if (level === LOCK_PENDING) return SQLITE_OK;
    if (f.shared > 1) return SQLITE_BUSY;
    f.exclusive = h;
    f.pending = 0;
    hold.level = LOCK_EXCLUSIVE;
    return SQLITE_OK;
  }

  /** Lower `h` to `level` (SHARED or NONE). */
  unlock(h: number, level: number): number {
    const hold = this.held.get(h);
    if (!hold || level >= hold.level) return SQLITE_OK;
    const f = this.files.get(hold.name)!;
    if (hold.level > LOCK_SHARED) {
      if (f.reserved === h) f.reserved = 0;
      if (f.pending === h) f.pending = 0;
      if (f.exclusive === h) f.exclusive = 0;
    }
    if (level === LOCK_NONE && hold.level >= LOCK_SHARED) f.shared--;
    hold.level = level;
    return SQLITE_OK;
  }

  /** Whether any handle holds RESERVED or higher on the database `h` is open on. */
  reservedBy(h: number): boolean {
    const hold = this.held.get(h);
    if (!hold) return false;
    const f = this.files.get(hold.name)!;
    return f.reserved !== 0 || f.pending !== 0 || f.exclusive !== 0;
  }
}

/**
 * Install a lock table over the methods of the VFS named `vfsName` (an
 * opfs-sahpool pool). The pool's own methods still run after the table
 * grants a lock, so the pool's bookkeeping stays as it expects.
 */
export function installPoolLocks(sqlite3: Sqlite3Static, vfsName: string): LockTable {
  const { capi, wasm } = sqlite3;
  const table = new LockTable();
  const pVfs = capi.sqlite3_vfs_find(vfsName);
  if (!pVfs) throw new Error(`sqlite-wasm: no VFS named ${vfsName}`);
  // Wrapping the struct the pool registered rewrites its function pointers in
  // place, so every connection opened on the VFS goes through them.
  const vfs = new capi.sqlite3_vfs(pVfs);
  type Fn = (...args: number[]) => number;
  // A method's slot holds a function pointer, under the struct's member key.
  const entry = (struct: object, method: string) => {
    const s = struct as Record<string, unknown> & { memberKey(name: string): string };
    return wasm.functionEntry(s[s.memberKey(method)] as number) as unknown as Fn;
  };
  const xOpen = entry(vfs, "xOpen");
  let patched = 0;

  const patchIo = (pMethods: number) => {
    if (patched === pMethods) return;
    patched = pMethods;
    const io = new capi.sqlite3_io_methods(pMethods);
    const xLock = entry(io, "xLock");
    const xUnlock = entry(io, "xUnlock");
    const xClose = entry(io, "xClose");
    io.installMethods({
      xLock: (pFile: number, level: number) => {
        const rc = table.lock(pFile, level);
        // Tell the pool the level actually held, granted or not.
        xLock(pFile, table.level(pFile));
        return rc;
      },
      xUnlock: (pFile: number, level: number) => {
        table.unlock(pFile, level);
        return xUnlock(pFile, level);
      },
      xCheckReservedLock: (pFile: number, pOut: number) => {
        wasm.poke32(pOut, table.tracks(pFile) ? (table.reservedBy(pFile) ? 1 : 0) : 1);
        return SQLITE_OK;
      },
      xClose: (pFile: number) => {
        table.close(pFile);
        return xClose(pFile);
      },
    } as never);
  };

  vfs.installMethods({
    xOpen: (pV: number, zName: number, pFile: number, flags: number, pOutFlags: number) => {
      const rc = xOpen(pV, zName, pFile, flags, pOutFlags);
      if (rc === SQLITE_OK) {
        patchIo(wasm.peekPtr(pFile) as number);
        if (flags & capi.SQLITE_OPEN_MAIN_DB && zName) {
          table.open(pFile, wasm.cstrToJs(zName as never) ?? `#${pFile}`);
        }
      }
      return rc;
    },
  } as never);
  return table;
}
