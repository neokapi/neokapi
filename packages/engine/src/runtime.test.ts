// Unit tests for the KapiRuntime facade against a mocked global surface: no
// wasm is booted; the engine globals are stubbed exactly as the Go binary
// registers them (converted JS objects / JSON strings, see abi.ts).

import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemFS } from "./memfs.ts";
import { makeRuntime } from "./runtime.ts";

const enc = new TextEncoder();

const ENGINE_GLOBALS = [
  "kapiRun",
  "kapiPreview",
  "labInspect",
  "labInspectAnnotated",
  "labSegment",
  "labSegmentEngines",
  "kbf",
  "kapiEngineABI",
] as const;

function clearEngineGlobals() {
  for (const name of ENGINE_GLOBALS) {
    delete (globalThis as Record<string, unknown>)[name];
  }
}

/** Install the minimal required globals (makeRuntime demands these three). */
function installCoreGlobals() {
  globalThis.kapiRun = vi.fn(() => Promise.resolve(0));
  globalThis.kapiPreview = vi.fn(() => Promise.resolve({ ok: true }));
  globalThis.labInspect = vi.fn(() => Promise.resolve({ ok: true }));
}

afterEach(clearEngineGlobals);

describe("makeRuntime", () => {
  it("throws a clear error when a required engine global is missing", () => {
    expect(() => makeRuntime(createMemFS())).toThrow(/kapiRun is not registered/);
  });

  it("run delegates argv to the kapiRun global and returns its exit code", async () => {
    installCoreGlobals();
    globalThis.kapiRun = vi.fn(() => Promise.resolve(3));
    const rt = makeRuntime(createMemFS());
    await expect(rt.run(["version"])).resolves.toBe(3);
    expect(globalThis.kapiRun).toHaveBeenCalledWith(["version"]);
  });

  it("preview passes through the wire object", async () => {
    installCoreGlobals();
    globalThis.kapiPreview = vi.fn(() =>
      Promise.resolve({ ok: true, format: "json", blocks: [{ id: "b1", text: "Hello" }] }),
    );
    const rt = makeRuntime(createMemFS());
    await expect(rt.preview("/p/a.json")).resolves.toEqual({
      ok: true,
      format: "json",
      blocks: [{ id: "b1", text: "Hello" }],
    });
  });

  it("inspect parses the serialized ContentTree from the json field", async () => {
    installCoreGlobals();
    globalThis.labInspect = vi.fn(() =>
      Promise.resolve({ ok: true, format: "json", json: `{"nodes":[]}`, bytes: 12 }),
    );
    const rt = makeRuntime(createMemFS());
    await expect(rt.inspect("/p/a.json")).resolves.toEqual({
      ok: true,
      format: "json",
      tree: { nodes: [] },
      bytes: 12,
    });
  });

  it("inspect surfaces engine errors and malformed trees as ok:false", async () => {
    installCoreGlobals();
    globalThis.labInspect = vi.fn(() => Promise.resolve({ ok: false, error: "no such file" }));
    const rt = makeRuntime(createMemFS());
    await expect(rt.inspect("/missing")).resolves.toEqual({ ok: false, error: "no such file" });

    globalThis.labInspect = vi.fn(() => Promise.resolve({ ok: true, json: "{not json" }));
    const rt2 = makeRuntime(createMemFS());
    const res = await rt2.inspect("/p/a.json");
    expect(res.ok).toBe(false);
    expect(res.error).toMatch(/parse content tree/);
  });

  it("inspectAnnotated forwards options as a JSON string and omits them by default", async () => {
    installCoreGlobals();
    const fn = vi.fn(() => Promise.resolve({ ok: true, json: "{}" }));
    globalThis.labInspectAnnotated = fn;
    const rt = makeRuntime(createMemFS());

    await rt.inspectAnnotated("/p/a.json");
    expect(fn).toHaveBeenLastCalledWith("/p/a.json");

    await rt.inspectAnnotated("/p/a.json", { term: false, segment: true });
    expect(fn).toHaveBeenLastCalledWith("/p/a.json", `{"term":false,"segment":true}`);
  });

  it("inspectAnnotated degrades when the global is absent (older wasm build)", async () => {
    installCoreGlobals();
    const rt = makeRuntime(createMemFS());
    await expect(rt.inspectAnnotated("/p/a.json")).resolves.toEqual({
      ok: false,
      error: "labInspectAnnotated unavailable in this wasm build",
    });
  });

  it("kbf round-trips JSON strings and degrades when the endpoint is absent", () => {
    installCoreGlobals();
    const rt = makeRuntime(createMemFS());
    expect(rt.kbf({ op: "roundtrip" }).ok).toBe(false);

    globalThis.kbf = vi.fn((reqJSON: string) => {
      const req = JSON.parse(reqJSON) as { op: string };
      return JSON.stringify({ ok: true, op: req.op });
    });
    const rt2 = makeRuntime(createMemFS());
    expect(rt2.kbf({ op: "renderHtml" })).toEqual({ ok: true, op: "renderHtml" });
    expect(globalThis.kbf).toHaveBeenCalledWith(`{"op":"renderHtml"}`);
  });

  it("segment maps the wire segments and reports the engine that ran", () => {
    installCoreGlobals();
    globalThis.labSegment = vi.fn(() => ({
      ok: true,
      engine: "srx",
      segments: [{ text: "One." }, { text: "Two." }],
    }));
    const rt = makeRuntime(createMemFS());
    expect(rt.segment("One. Two.", "", "en")).toEqual({
      ok: true,
      engine: "srx",
      segments: [{ text: "One." }, { text: "Two." }],
    });
    expect(globalThis.labSegment).toHaveBeenCalledWith("One. Two.", "", "en");
  });

  it("segment and segmentEngines degrade when the globals are absent", () => {
    installCoreGlobals();
    const rt = makeRuntime(createMemFS());
    expect(rt.segment("x", "", "en").ok).toBe(false);
    expect(rt.segmentEngines()).toEqual([]);

    globalThis.labSegmentEngines = vi.fn(() => ["srx", "uax29"]);
    const rt2 = makeRuntime(createMemFS());
    expect(rt2.segmentEngines()).toEqual(["srx", "uax29"]);
  });

  it("runWithTrace appends --trace, reads the trace back, and uses fresh paths per run", async () => {
    installCoreGlobals();
    const mem = createMemFS();
    const calls: string[][] = [];
    globalThis.kapiRun = vi.fn((argv: string[]) => {
      calls.push(argv);
      const tracePath = argv[argv.indexOf("--trace") + 1];
      mem.vol.mkdirp("/.lab");
      mem.vol.writeFile(tracePath, enc.encode(`{"steps":[{"id":"s1"}]}`));
      return Promise.resolve(0);
    });
    const rt = makeRuntime(mem);

    const first = await rt.runWithTrace(["pseudo-translate", "/p/in.json"]);
    expect(first.code).toBe(0);
    expect(first.trace).toEqual({ steps: [{ id: "s1" }] });
    expect(calls[0].slice(0, 2)).toEqual(["pseudo-translate", "/p/in.json"]);
    expect(calls[0][2]).toBe("--trace");

    const second = await rt.runWithTrace(["pseudo-translate", "/p/in.json"]);
    expect(calls[1][3]).not.toBe(calls[0][3]); // unique trace path per run
    expect(second.trace).toEqual({ steps: [{ id: "s1" }] });
  });

  it("runWithTrace returns a null trace when the run produced no trace file", async () => {
    installCoreGlobals();
    globalThis.kapiRun = vi.fn(() => Promise.resolve(1));
    const rt = makeRuntime(createMemFS());
    await expect(rt.runWithTrace(["run", "x"])).resolves.toEqual({ code: 1, trace: null });
  });

  it("exposes the volume and cwd/chdir over the memfs process", () => {
    installCoreGlobals();
    const mem = createMemFS();
    const rt = makeRuntime(mem);
    expect(rt.cwd()).toBe("/project");
    rt.vol.mkdirp("/project/sub");
    rt.chdir("/project/sub");
    expect(rt.cwd()).toBe("/project/sub");
    rt.vol.writeFile("/project/sub/a.txt", enc.encode("hi"));
    expect(rt.vol.readdir("/project/sub")).toEqual(["a.txt"]);
  });
});
