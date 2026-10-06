import { describe, expect, it } from "vitest";
import { createMemFS } from "./memfs.ts";
import { applyChanges, changesOf, describeStorage, storageName } from "./storage.ts";

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
  });
});
