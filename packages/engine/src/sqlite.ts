// The SQLite bridge: the JavaScript half of the browser engine's database
// driver (core/storage/driver_js.go).
//
// The engine's stores are SQL behind core/storage, natively and in the
// browser alike. In the browser the database is the official SQLite
// WebAssembly build (@sqlite.org/sqlite-wasm), and Go reaches it through one
// object on globalThis, `__kapiSQL`, whose methods Go calls synchronously with
// syscall/js. Go and SQLite therefore share a thread: the page's main thread
// today, or a dedicated Worker.
//
// Databases live in SQLite's `memdb` VFS, in the module's memory. A database is
// named by its absolute path, every connection to one name shares it, and the
// bridge keeps one connection of its own on each name so a database outlives
// the pools that open and close it between commands. That connection is also
// what the driver-owned namespace answers from (exists, remove, rename, list):
// the page's file system never sees these databases. Nothing outlives the
// tab.
//
// Every method returns a number or a plain object and never throws; a failure
// is `{ err, code }` with SQLite's extended result code. Values cross in one
// packed byte buffer per call rather than one JavaScript value each. The tags
// and layout below are shared with driver_js.go; change both together.

import type { Sqlite3Static } from "@sqlite.org/sqlite-wasm";

/** Value tags in the packed argument and row buffers. */
const TAG_NULL = 0;
const TAG_INT = 1; // int64, little-endian
const TAG_FLOAT = 2; // float64, little-endian
const TAG_TEXT = 3; // uint32 length, then UTF-8 bytes
const TAG_BLOB = 4; // uint32 length, then bytes

/** A failed call: SQLite's message and extended result code. */
export interface SQLiteFailure {
  err: string;
  code: number;
}

/** What `prepare` answers: the statement and the SQL after it. */
export interface SQLitePrepared {
  /** Statement handle, or 0 when the SQL held no statement. */
  id: number;
  /** Result column names. */
  cols: string[];
  /** Declared column types, lowercased ("" for an expression). */
  decl: string[];
  /** Number of `?` parameters. */
  params: number;
  /** The SQL after this statement, "" when there is none. */
  tail: string;
}

/** The object installed as `globalThis.__kapiSQL`. */
export interface SQLiteBridge {
  /** The SQLite library version, e.g. "3.53.4". */
  version(): string;
  /**
   * Arguments in, results out. `run` and `exec` write the change count and
   * the last row id here as two int64s. Go replaces it with a larger buffer
   * when a statement's arguments do not fit.
   */
  io: Uint8Array;
  /**
   * Rows written by the last `next`: uint32 row count, uint32 done, values.
   * A batch ends at `max` rows or once it passes about 1 MiB, whichever
   * comes first, so large rows cross a few at a time.
   */
  rows: Uint8Array;
  open(name: string): number | SQLiteFailure;
  close(conn: number): number | SQLiteFailure;
  /** Runs SQL with no arguments, any number of statements. */
  exec(conn: number, sql: string): number | SQLiteFailure;
  prepare(conn: number, sql: string): SQLitePrepared | SQLiteFailure;
  /** Binds `n` bytes of packed arguments from `io` and runs to completion. */
  run(stmt: number, n: number): number | SQLiteFailure;
  /** Binds `n` bytes of packed arguments from `io`, ready for `next`. */
  query(stmt: number, n: number): number | SQLiteFailure;
  /** Steps up to `max` rows into `rows`; answers the byte length written. */
  next(stmt: number, max: number): number | SQLiteFailure;
  reset(stmt: number): number;
  finalize(stmt: number): number;
  exists(name: string): boolean;
  /**
   * Holds `data`, the bytes of a database file, under `name`. Go calls it
   * the first time a connection opens a name whose file the page's file
   * system holds. A name already held keeps its database.
   */
  load(name: string, data: Uint8Array): number | SQLiteFailure;
  remove(name: string): number | SQLiteFailure;
  rename(from: string, to: string): number | SQLiteFailure;
  list(dir: string): string[];
}

declare global {
  /** The SQLite bridge the browser engine's database driver calls. */
  // eslint-disable-next-line no-var
  var __kapiSQL: SQLiteBridge | undefined;
}

/** Options for {@link loadSQLite}. */
export interface LoadSQLiteOptions {
  /**
   * Where `sqlite3.wasm` is served. A precompressed `<wasmUrl>.gz` beside it
   * is preferred where the platform can inflate it, as for the engine binary.
   * Without a URL the module locates the file beside its own script (or reads
   * it from the package under Node).
   */
  wasmUrl?: string;
}

/**
 * Load and initialise the SQLite WebAssembly module. The `sqlite3.wasm` it
 * loads must come from the same `@sqlite.org/sqlite-wasm` release as the
 * JavaScript, which is this package's dependency.
 */
export async function loadSQLite(opts: LoadSQLiteOptions = {}): Promise<Sqlite3Static> {
  const binary = opts.wasmUrl ? fetchWasm(opts.wasmUrl) : undefined;
  // Fetched and imported side by side; a failed fetch is reported below.
  binary?.catch(() => {});
  const { default: init } = await import("@sqlite.org/sqlite-wasm");
  if (!binary) return init();
  // The Emscripten module options pass through the wrapper; the published
  // typing declares none.
  const withOptions = init as unknown as (opts: {
    wasmBinary: ArrayBuffer;
  }) => Promise<Sqlite3Static>;
  return withOptions({ wasmBinary: await binary });
}

/** Fetch a wasm asset, preferring its precompressed `.gz` sibling. */
async function fetchWasm(url: string): Promise<ArrayBuffer> {
  if (typeof DecompressionStream !== "undefined") {
    try {
      const gz = await fetch(`${url}.gz`);
      if (gz.ok && gz.body) {
        return await new Response(
          gz.body.pipeThrough(new DecompressionStream("gzip")),
        ).arrayBuffer();
      }
    } catch {
      /* fall through to the raw asset */
    }
  }
  const resp = await fetch(url);
  if (!resp.ok) throw new Error(`failed to load ${url} (${resp.status})`);
  return resp.arrayBuffer();
}

/**
 * Install the bridge on globalThis for an initialised SQLite module. Call it
 * before the Go program starts; the driver looks the bridge up on first open.
 * Installing twice replaces the first bridge and its databases.
 */
export function installSQLiteBridge(sqlite3: Sqlite3Static): SQLiteBridge {
  const bridge = createSQLiteBridge(sqlite3);
  globalThis.__kapiSQL = bridge;
  return bridge;
}

interface Conn {
  db: number;
  stmts: Set<number>;
  /** The shared database name, absent for a private in-memory database. */
  file?: string;
}

interface Stmt {
  ptr: number;
  conn: number;
  ncol: number;
}

interface File {
  /** The bridge's own connection, which keeps the database alive. */
  pin: number;
  /** Driver connections open on it. */
  open: number;
}

// No parameter properties: Node runs this file with type stripping only.
class SQLiteError extends Error {
  readonly code: number;
  constructor(message: string, code: number) {
    super(message);
    this.code = code;
  }
}

/** Build the bridge without installing it. */
export function createSQLiteBridge(sqlite3: Sqlite3Static): SQLiteBridge {
  const { capi, wasm } = sqlite3;
  // The raw exports take and return pointers as numbers and int64 as bigint,
  // which is what a packed buffer needs; the published typing leaves them
  // untyped.
  const x = wasm.exports as Record<string, (...args: unknown[]) => number & bigint>;
  const SQLITE_ROW = capi.SQLITE_ROW;
  const SQLITE_DONE = capi.SQLITE_DONE;
  const DEALLOC = capi.SQLITE_WASM_DEALLOC as unknown as number;
  const OPEN_FLAGS = capi.SQLITE_OPEN_READWRITE | capi.SQLITE_OPEN_CREATE;

  // memdb is the default VFS too, so `ATTACH '<path>'` and `VACUUM INTO
  // '<path>'` reach the same shared databases the driver opens by name.
  const memdb = capi.sqlite3_vfs_find("memdb");
  if (!memdb) throw new Error("sqlite-wasm: this build has no memdb VFS");
  capi.sqlite3_vfs_register(memdb, 1);

  const conns = new Map<number, Conn>();
  const stmts = new Map<number, Stmt>();
  const files = new Map<string, File>();
  let nextId = 1;
  const decoder = new TextDecoder();

  const fail = (e: unknown): SQLiteFailure => {
    if (e instanceof SQLiteError) return { err: e.message, code: e.code };
    const resultCode = (e as { resultCode?: number } | null)?.resultCode;
    return { err: String((e as Error)?.message ?? e), code: resultCode ?? 1 };
  };
  const dbFail = (db: number, rc: number): SQLiteFailure => ({
    err: capi.sqlite3_errmsg(db) || capi.sqlite3_errstr(rc),
    code: Number(capi.sqlite3_extended_errcode(db)) || rc,
  });

  const openDb = (name: string): number => {
    const sp = wasm.pstack.pointer;
    try {
      const pp = wasm.pstack.allocPtr() as number;
      const rc = capi.sqlite3_open_v2(name, pp, OPEN_FLAGS, "memdb");
      const db = wasm.peekPtr(pp) as number;
      if (rc) {
        const msg = db ? capi.sqlite3_errmsg(db) : capi.sqlite3_errstr(rc);
        if (db) capi.sqlite3_close_v2(db);
        throw new SQLiteError(`open ${name}: ${msg}`, rc);
      }
      capi.sqlite3_extended_result_codes(db, 1);
      return db;
    } finally {
      wasm.pstack.restore(sp);
    }
  };

  // A growable writer for the packed row buffer. The first eight bytes are
  // the header: rows written, and whether the statement is exhausted. A batch
  // stops once it passes BATCH_BYTES, so the buffer stays near that size; a
  // single row larger than that grows it, and the next batch starts again
  // from a small one.
  const ROW_BUF_BYTES = 1 << 16;
  const BATCH_BYTES = 1 << 20;
  let rowBuf = new Uint8Array(ROW_BUF_BYTES);
  let rowView = new DataView(rowBuf.buffer);
  let rowLen = 8;
  const grow = (n: number) => {
    if (rowLen + n <= rowBuf.length) return;
    let size = rowBuf.length * 2;
    while (size < rowLen + n) size *= 2;
    const next = new Uint8Array(size);
    next.set(rowBuf.subarray(0, rowLen));
    rowBuf = next;
    rowView = new DataView(next.buffer);
  };
  const putTag = (t: number) => {
    grow(1);
    rowBuf[rowLen++] = t;
  };
  const putBytes = (tag: number, ptr: number, len: number) => {
    grow(5 + len);
    rowBuf[rowLen] = tag;
    rowView.setUint32(rowLen + 1, len, true);
    // Read the heap view only now: a conversion inside SQLite can grow the
    // wasm memory, which detaches any earlier view.
    if (len) rowBuf.set(wasm.heap8u().subarray(ptr, ptr + len), rowLen + 5);
    rowLen += 5 + len;
  };

  const writeResult = (b: SQLiteBridge, db: number) => {
    const dv = new DataView(b.io.buffer, b.io.byteOffset, b.io.byteLength);
    dv.setBigInt64(0, BigInt(x.sqlite3_changes64(db)), true);
    dv.setBigInt64(8, BigInt(x.sqlite3_last_insert_rowid(db)), true);
  };

  const bind = (b: SQLiteBridge, stmt: number, n: number): number => {
    x.sqlite3_reset(stmt);
    x.sqlite3_clear_bindings(stmt);
    const buf = b.io;
    const dv = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    let o = 0;
    for (let i = 1; o < n; i++) {
      const tag = buf[o++];
      let rc: number;
      switch (tag) {
        case TAG_NULL:
          rc = x.sqlite3_bind_null(stmt, i);
          break;
        case TAG_INT:
          rc = x.sqlite3_bind_int64(stmt, i, dv.getBigInt64(o, true));
          o += 8;
          break;
        case TAG_FLOAT:
          rc = x.sqlite3_bind_double(stmt, i, dv.getFloat64(o, true));
          o += 8;
          break;
        case TAG_TEXT:
        case TAG_BLOB: {
          const len = dv.getUint32(o, true);
          o += 4;
          // Never a null pointer, so '' and x'' bind as values, not NULL.
          // SQLite frees the copy (SQLITE_WASM_DEALLOC), on failure too.
          const p = wasm.alloc(len || 1) as number;
          wasm.heap8u().set(buf.subarray(o, o + len), p);
          o += len;
          rc =
            tag === TAG_TEXT
              ? x.sqlite3_bind_text(stmt, i, p, len, DEALLOC)
              : x.sqlite3_bind_blob(stmt, i, p, len, DEALLOC);
          break;
        }
        default:
          throw new SQLiteError(`sqlite bridge: unknown value tag ${tag}`, 1);
      }
      if (rc) return rc;
    }
    return 0;
  };

  const finalize = (sid: number) => {
    const s = stmts.get(sid);
    if (!s) return;
    x.sqlite3_finalize(s.ptr);
    stmts.delete(sid);
    conns.get(s.conn)?.stmts.delete(sid);
  };

  const removeFile = (name: string): number | SQLiteFailure => {
    const f = files.get(name);
    if (!f) return 0;
    if (f.open > 0) return { err: `database ${name} is open`, code: capi.SQLITE_BUSY };
    capi.sqlite3_close_v2(f.pin);
    files.delete(name);
    return 0;
  };

  const bridge: SQLiteBridge = {
    io: new Uint8Array(1 << 16),
    rows: rowBuf,
    version: () => sqlite3.version.libVersion,

    open(name) {
      try {
        const shared = name.startsWith("/");
        if (shared && !files.has(name)) files.set(name, { pin: openDb(name), open: 0 });
        const db = openDb(name);
        const id = nextId++;
        conns.set(id, { db, stmts: new Set(), file: shared ? name : undefined });
        if (shared) files.get(name)!.open++;
        return id;
      } catch (e) {
        return fail(e);
      }
    },

    close(id) {
      const c = conns.get(id);
      if (!c) return 0;
      // Deleting the entry being visited is safe while iterating a Set.
      for (const sid of c.stmts) finalize(sid);
      const rc = capi.sqlite3_close_v2(c.db);
      conns.delete(id);
      const f = c.file ? files.get(c.file) : undefined;
      if (f) f.open--;
      return rc ? { err: capi.sqlite3_errstr(rc), code: rc } : 0;
    },

    exec(id, sql) {
      const c = conns.get(id);
      if (!c) return { err: "sqlite bridge: connection is closed", code: capi.SQLITE_MISUSE };
      try {
        const rc = capi.sqlite3_exec(c.db, sql, 0, 0, 0);
        if (rc) return dbFail(c.db, rc);
        writeResult(this, c.db);
        return 0;
      } catch (e) {
        return fail(e);
      }
    },

    prepare(id, sql) {
      const c = conns.get(id);
      if (!c) return { err: "sqlite bridge: connection is closed", code: capi.SQLITE_MISUSE };
      const sp = wasm.pstack.pointer;
      let pSql = 0;
      try {
        const [p, n] = wasm.allocCString(sql, true);
        pSql = p as number;
        const pp = wasm.pstack.allocPtr() as number;
        const pt = wasm.pstack.allocPtr() as number;
        const rc = x.sqlite3_prepare_v3(c.db, pSql, n, 0, pp, pt);
        if (rc) return dbFail(c.db, rc);
        const ptr = wasm.peekPtr(pp) as number;
        const tailPtr = wasm.peekPtr(pt) as number;
        const tail =
          tailPtr && tailPtr < pSql + n
            ? decoder.decode(wasm.heap8u().subarray(tailPtr, pSql + n)).trim()
            : "";
        if (!ptr) return { id: 0, cols: [], decl: [], params: 0, tail };
        const ncol = Number(x.sqlite3_column_count(ptr));
        const cols: string[] = [];
        const decl: string[] = [];
        for (let i = 0; i < ncol; i++) {
          cols.push(capi.sqlite3_column_name(ptr, i) ?? "");
          decl.push((capi.sqlite3_column_decltype(ptr, i) ?? "").toLowerCase());
        }
        const sid = nextId++;
        stmts.set(sid, { ptr, conn: id, ncol });
        c.stmts.add(sid);
        return { id: sid, cols, decl, params: Number(x.sqlite3_bind_parameter_count(ptr)), tail };
      } catch (e) {
        return fail(e);
      } finally {
        if (pSql) wasm.dealloc(pSql);
        wasm.pstack.restore(sp);
      }
    },

    run(sid, n) {
      const s = stmts.get(sid);
      if (!s) return { err: "sqlite bridge: statement is closed", code: capi.SQLITE_MISUSE };
      const db = conns.get(s.conn)!.db;
      try {
        let rc = bind(this, s.ptr, n);
        if (rc) return dbFail(db, rc);
        while ((rc = x.sqlite3_step(s.ptr)) === SQLITE_ROW) {
          /* a statement run for its effect: discard any rows */
        }
        if (rc !== SQLITE_DONE) {
          const f = dbFail(db, rc);
          x.sqlite3_reset(s.ptr);
          return f;
        }
        x.sqlite3_reset(s.ptr);
        writeResult(this, db);
        return 0;
      } catch (e) {
        x.sqlite3_reset(s.ptr);
        return fail(e);
      }
    },

    query(sid, n) {
      const s = stmts.get(sid);
      if (!s) return { err: "sqlite bridge: statement is closed", code: capi.SQLITE_MISUSE };
      try {
        const rc = bind(this, s.ptr, n);
        return rc ? dbFail(conns.get(s.conn)!.db, rc) : 0;
      } catch (e) {
        return fail(e);
      }
    },

    next(sid, max) {
      const s = stmts.get(sid);
      if (!s) return { err: "sqlite bridge: statement is closed", code: capi.SQLITE_MISUSE };
      if (rowBuf.length > 2 * BATCH_BYTES) {
        rowBuf = new Uint8Array(ROW_BUF_BYTES);
        rowView = new DataView(rowBuf.buffer);
      }
      rowLen = 8;
      let rows = 0;
      let done = 0;
      try {
        for (; rows < max && rowLen < BATCH_BYTES; rows++) {
          const rc = x.sqlite3_step(s.ptr);
          if (rc === SQLITE_DONE) {
            done = 1;
            x.sqlite3_reset(s.ptr);
            break;
          }
          if (rc !== SQLITE_ROW) {
            const f = dbFail(conns.get(s.conn)!.db, rc);
            x.sqlite3_reset(s.ptr);
            return f;
          }
          for (let i = 0; i < s.ncol; i++) {
            switch (Number(x.sqlite3_column_type(s.ptr, i))) {
              case capi.SQLITE_INTEGER:
                grow(9);
                rowBuf[rowLen] = TAG_INT;
                rowView.setBigInt64(rowLen + 1, BigInt(x.sqlite3_column_int64(s.ptr, i)), true);
                rowLen += 9;
                break;
              case capi.SQLITE_FLOAT:
                grow(9);
                rowBuf[rowLen] = TAG_FLOAT;
                rowView.setFloat64(rowLen + 1, Number(x.sqlite3_column_double(s.ptr, i)), true);
                rowLen += 9;
                break;
              case capi.SQLITE_TEXT: {
                const p = Number(x.sqlite3_column_text(s.ptr, i));
                putBytes(TAG_TEXT, p, Number(x.sqlite3_column_bytes(s.ptr, i)));
                break;
              }
              case capi.SQLITE_BLOB: {
                const p = Number(x.sqlite3_column_blob(s.ptr, i));
                putBytes(TAG_BLOB, p, Number(x.sqlite3_column_bytes(s.ptr, i)));
                break;
              }
              default:
                putTag(TAG_NULL);
            }
          }
        }
        rowView.setUint32(0, rows, true);
        rowView.setUint32(4, done, true);
        this.rows = rowBuf;
        return rowLen;
      } catch (e) {
        x.sqlite3_reset(s.ptr);
        return fail(e);
      }
    },

    reset(sid) {
      const s = stmts.get(sid);
      if (s) x.sqlite3_reset(s.ptr);
      return 0;
    },

    finalize(sid) {
      finalize(sid);
      return 0;
    },

    exists: (name) => files.has(name),

    load(name, data) {
      if (files.has(name)) return 0;
      // sqlite3_deserialize opens the bytes as a database private to one
      // connection; VACUUM INTO then copies it to the shared name, as rename
      // does. The name's own connection, opened first, keeps the copy alive.
      let tmp = 0;
      try {
        files.set(name, { pin: openDb(name), open: 0 });
        tmp = openDb(":memory:");
        const p = wasm.alloc(data.length) as number;
        const heap = wasm.heap8u();
        heap.set(data, p);
        // A file written in WAL mode says so in bytes 18 and 19 of its
        // header. memdb keeps no log, so it reads the database in rollback
        // mode, which the bytes then say instead.
        if (heap[p + 18] === 2) heap[p + 18] = 1;
        if (heap[p + 19] === 2) heap[p + 19] = 1;
        // SQLite owns the copy from here, and frees it on failure too.
        let rc = capi.sqlite3_deserialize(
          tmp,
          "main",
          p,
          data.length,
          data.length,
          capi.SQLITE_DESERIALIZE_FREEONCLOSE | capi.SQLITE_DESERIALIZE_RESIZEABLE,
        );
        if (!rc)
          rc = capi.sqlite3_exec(tmp, `VACUUM INTO '${name.replaceAll("'", "''")}'`, 0, 0, 0);
        if (rc) {
          const failure = dbFail(tmp, rc);
          removeFile(name);
          return failure;
        }
        return 0;
      } catch (e) {
        removeFile(name);
        return fail(e);
      } finally {
        if (tmp) capi.sqlite3_close_v2(tmp);
      }
    },

    remove: (name) => removeFile(name),

    rename(from, to) {
      if (from === to) return 0;
      const f = files.get(from);
      if (!f) return { err: `no database at ${from}`, code: capi.SQLITE_CANTOPEN };
      if (f.open > 0) return { err: `database ${from} is open`, code: capi.SQLITE_BUSY };
      const removed = removeFile(to);
      if (removed !== 0) return removed;
      try {
        // VACUUM INTO writes a copy to a database that must be empty; the
        // destination's own connection, opened first, keeps the copy alive.
        files.set(to, { pin: openDb(to), open: 0 });
        const rc = capi.sqlite3_exec(f.pin, `VACUUM INTO '${to.replaceAll("'", "''")}'`, 0, 0, 0);
        if (rc) {
          const failure = dbFail(f.pin, rc);
          removeFile(to);
          return failure;
        }
      } catch (e) {
        removeFile(to);
        return fail(e);
      }
      return removeFile(from);
    },

    list(dir) {
      const prefix = dir.endsWith("/") ? dir : `${dir}/`;
      return [...files.keys()].filter((name) => name.startsWith(prefix)).sort();
    },
  };
  return bridge;
}
