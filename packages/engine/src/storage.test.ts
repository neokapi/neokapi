import { describe, expect, it, vi } from "vitest";
import { createMemFS } from "./memfs.ts";
import { applyChanges, changesOf, describeStorage, storageName, takeLock } from "./storage.ts";
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

// Node carries the Web Locks API, so these run against the real thing.
describe("the owner's lock", () => {
  const locks = (globalThis as unknown as { navigator: { locks: OwnerLocks } }).navigator.locks;
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
