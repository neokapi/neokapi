import { describe, expect, it, vi } from "vitest";
import { createMemFS } from "./memfs.ts";
import {
  applyChanges,
  changesOf,
  describeStorage,
  openPool,
  storageName,
  takeLock,
} from "./storage.ts";
import type { OwnerLocks } from "./storage.ts";

const enc = new TextEncoder();
const dec = new TextDecoder();

/** Run a Node-style fs call to completion. */
function fsCall(fn: (cb: (e: unknown, v?: unknown) => void) => void): unknown {
  let out: unknown;
  fn((e, v) => {
    if (e) throw e;
    out = v;
  });
  return out;
}

describe("the engine's file system changes", () => {
  it("reports every path a write, a rename and a removal touch", () => {
    const seen: string[] = [];
    const mem = createMemFS({ onChange: (p) => seen.push(p) });
    seen.length = 0;
    mem.vol.mkdirp("/p/docs");
    const fd = fsCall((cb) => mem.fs.open("/p/docs/a.json", 0o1101, 0o644, cb)) as number;
    const data = enc.encode("{}");
    fsCall((cb) => mem.fs.write(fd, data, 0, data.length, null, cb));
    fsCall((cb) => mem.fs.rename("/p/docs/a.json", "/p/docs/b.json", cb));
    fsCall((cb) => mem.fs.unlink("/p/docs/b.json", cb));
    expect(new Set(seen)).toEqual(new Set(["/p", "/p/docs", "/p/docs/a.json", "/p/docs/b.json"]));
  });

  it("carries a renamed directory's tree to another file system", () => {
    const from = createMemFS();
    from.vol.mkdirp("/w/sub");
    from.vol.writeFile("/w/sub/x.txt", enc.encode("x"));
    from.setMtime("/w/sub/x.txt", 1234);
    const to = createMemFS();
    applyChanges(to, changesOf(from, ["/w", "/gone"]));
    expect(dec.decode(to.vol.readFile("/w/sub/x.txt"))).toBe("x");
    expect(to.mtime("/w/sub/x.txt")).toBe(1234);

    from.vol.remove("/w/sub");
    applyChanges(to, changesOf(from, ["/w/sub"]));
    expect(to.vol.exists("/w/sub")).toBe(false);
    expect(to.vol.isDir("/w")).toBe(true);
  });
});

describe("storage", () => {
  it("names a key stably and safely", () => {
    expect(storageName("kapi")).toBe(storageName("kapi"));
    expect(storageName("kapi")).not.toBe(storageName("/next/"));
    expect(storageName("/next/")).toMatch(/^kapi-[0-9a-f]{8}$/);
  });

  it("says where the workspace is", () => {
    expect(describeStorage({ kind: "opfs" })).toMatch(/kept in this browser/);
    expect(describeStorage({ kind: "memory", reason: "another-tab" })).toMatch(/Another tab/);
    expect(describeStorage({ kind: "memory", reason: "taken" })).toMatch(/took over/);
  });
});

/**
 * Exclusive Web Locks as the specification grants them: requests queue per
 * name, a signal that aborts a waiting request rejects it with AbortError,
 * and `steal` releases the held lock (its request rejects with AbortError)
 * and is granted at once. Node 22 has no navigator.locks; Node 24 does, and
 * the same cases run against it there.
 */
function specLocks(): OwnerLocks {
  type Waiter = { grant: () => void; reject: (e: Error) => void };
  const held = new Map<string, { reject: (e: Error) => void } | undefined>();
  const queues = new Map<string, Waiter[]>();
  const abort = () => Object.assign(new Error("aborted"), { name: "AbortError" });
  const next = (name: string) => {
    held.delete(name);
    const w = queues.get(name)?.shift();
    w?.grant();
  };
  return {
    request(name, options, callback) {
      return new Promise((resolve, reject) => {
        const run = () => {
          const slot = { reject };
          held.set(name, slot);
          callback().then(
            (v) => {
              if (held.get(name) !== slot) return;
              resolve(v);
              next(name);
            },
            (e: Error) => {
              if (held.get(name) !== slot) return;
              reject(e);
              next(name);
            },
          );
        };
        if (options.steal) {
          held.get(name)?.reject(abort());
          held.delete(name);
          run();
          return;
        }
        if (!held.has(name)) return run();
        const waiter: Waiter = { grant: run, reject };
        const q = queues.get(name) ?? [];
        q.push(waiter);
        queues.set(name, q);
        options.signal?.addEventListener("abort", () => {
          const i = q.indexOf(waiter);
          if (i >= 0) {
            q.splice(i, 1);
            reject(abort());
          }
        });
      });
    },
  };
}

const nodeLocks = (globalThis as { navigator?: { locks?: OwnerLocks } }).navigator?.locks;
const lockManagers: [string, OwnerLocks][] = [["the specification's locks", specLocks()]];
if (nodeLocks) lockManagers.push(["the runtime's navigator.locks", nodeLocks]);

describe.each(lockManagers)("the owner's lock, over %s", (_, locks) => {
  let n = 0;
  const unique = () => `kapi-test-${process.pid}-${++n}`;
  const never = () => {};

  it("is granted to the first tab, and a tab that runs in memory does not get it", async () => {
    const name = unique();
    await expect(takeLock(locks, name, "memory", never, 50)).resolves.toBe(true);
    await expect(takeLock(locks, name, "memory", never, 50)).resolves.toBe(false);
  });

  it("is taken over by a tab that asks to, and the owner is told it lost it", async () => {
    const name = unique();
    let lost = 0;
    await expect(takeLock(locks, name, "memory", () => lost++, 50)).resolves.toBe(true);
    await expect(takeLock(locks, name, "take", never, 50)).resolves.toBe(true);
    await vi.waitFor(() => expect(lost).toBe(1));
    // The tab that took it holds it now.
    await expect(takeLock(locks, name, "memory", never, 50)).resolves.toBe(false);
  });

  it("waits, when asked to, until the owner lets go", async () => {
    const name = unique();
    let release = () => {};
    await new Promise<void>((held) => {
      void locks.request(name, {}, () => {
        held();
        return new Promise<void>((r) => (release = r));
      });
    });
    let granted = false;
    const waiting = takeLock(locks, name, "wait", never).then((ok) => (granted = ok));
    await new Promise((r) => setTimeout(r, 50));
    expect(granted).toBe(false);
    release();
    await waiting;
    expect(granted).toBe(true);
  });
});

// A page without Web Locks (an older runtime, or a context that is not
// secure) cannot name an owner, so the workspace stays in memory.
describe("a page without Web Locks", () => {
  it("keeps the workspace in memory and says the browser cannot keep it", async () => {
    vi.stubGlobal(
      "FileSystemFileHandle",
      class {
        createSyncAccessHandle() {}
      },
    );
    vi.stubGlobal("navigator", { storage: { getDirectory: async () => ({}) } });
    try {
      await expect(openPool({} as never, "kapi")).resolves.toEqual({
        kind: "memory",
        reason: "unsupported",
      });
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
