// The facade over an engine in a Worker, against a stand-in Worker that speaks
// the protocol (protocol.ts): what a tab does when another tab takes its
// workspace over, and when it takes the workspace over itself.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FromWorker, ToWorker, WorkerBoot } from "./protocol.ts";
import type { StorageInfo } from "./storage.ts";

const enc = new TextEncoder();
const dec = new TextDecoder();

/** What each stand-in Worker reports as its storage when it boots. */
let storageFor: (boot: WorkerBoot) => StorageInfo;

class FakeWorker {
  static all: FakeWorker[] = [];
  onmessage: ((ev: { data: FromWorker }) => void) | null = null;
  onerror: ((ev: { message: string }) => void) | null = null;
  received: ToWorker[] = [];
  boot: WorkerBoot | null = null;
  terminated = false;

  constructor() {
    FakeWorker.all.push(this);
  }

  postMessage(msg: ToWorker): void {
    this.received.push(msg);
    if (msg.t === "boot") {
      this.boot = msg.boot;
      const storage = storageFor(msg.boot);
      // A Worker that opened the kept workspace finds its files there.
      const ops =
        storage.kind === "opfs"
          ? [
              { op: "dir" as const, path: "/kept" },
              { op: "file" as const, path: "/kept/a.txt", data: enc.encode("kept"), mtime: 1 },
            ]
          : [];
      queueMicrotask(() =>
        this.emit({
          t: "ready",
          storage,
          ops,
          cwd: "/",
          abi: { abi: 1, version: "test", functions: ["kapiRun", "kapiPreview", "labInspect"] },
        }),
      );
    }
  }

  emit(msg: FromWorker): void {
    this.onmessage?.({ data: msg });
  }

  /** Answer the call with this id. */
  answer(id: number, value: unknown): void {
    this.emit({ t: "result", id, ok: true, value, cwd: "/" });
  }

  calls(): Extract<ToWorker, { t: "call" }>[] {
    return this.received.filter((m): m is Extract<ToWorker, { t: "call" }> => m.t === "call");
  }

  terminate(): void {
    this.terminated = true;
  }
}

beforeEach(() => {
  vi.resetModules();
  FakeWorker.all = [];
  vi.stubGlobal("Worker", FakeWorker);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function boot(opts: { whenHeld?: "memory" | "wait" | "take" } = {}) {
  const { bootKapiRuntime } = await import("./runtime.ts");
  return bootKapiRuntime("http://test/wasm_exec.js", "http://test/kapi-cli.wasm", opts);
}

describe("an engine in a Worker", () => {
  it("is told when another tab takes its workspace over, and carries on in memory with its files", async () => {
    storageFor = (b) => (b.persist ? { kind: "opfs", name: "kapi-x" } : { kind: "memory" });
    const rt = await boot();
    expect(rt.storage.kind).toBe("opfs");
    const owner = FakeWorker.all[0];
    rt.vol.mkdirp("/notes");
    rt.vol.writeFile("/notes/b.txt", enc.encode("mine"));

    const seen: StorageInfo[] = [];
    rt.onStorageChange((info) => seen.push(info));
    const inFlight = rt.run(["status"]);
    await vi.waitFor(() => expect(owner.calls()).toHaveLength(1));

    owner.emit({ t: "lost" });
    await expect(inFlight).rejects.toThrow(/another tab took over the workspace/);
    expect(owner.terminated).toBe(true);
    expect(rt.storage).toEqual({ kind: "memory", reason: "taken" });
    expect(seen).toEqual([{ kind: "memory", reason: "taken" }]);

    // A Worker in memory takes its place and gets the page's files.
    await vi.waitFor(() => expect(FakeWorker.all).toHaveLength(2));
    const next = FakeWorker.all[1];
    expect(next.boot?.persist).toBeNull();
    await vi.waitFor(() => expect(next.received.some((m) => m.t === "vol")).toBe(true));
    const handed = next.received.find((m) => m.t === "vol") as Extract<ToWorker, { t: "vol" }>;
    expect(handed.ops).toContainEqual(
      expect.objectContaining({ op: "file", path: "/notes/b.txt" }),
    );
    expect(dec.decode(rt.vol.readFile("/notes/b.txt"))).toBe("mine");

    // Calls go to it.
    const after = rt.run(["status"]);
    await vi.waitFor(() => expect(next.calls()).toHaveLength(1));
    next.answer(next.calls()[0].id, 0);
    await expect(after).resolves.toBe(0);
  });

  it("takes the workspace over from the tab that holds it", async () => {
    storageFor = (b) =>
      b.whenHeld === "take"
        ? { kind: "opfs", name: "kapi-x" }
        : { kind: "memory", reason: "another-tab" };
    const rt = await boot();
    expect(rt.storage).toEqual({ kind: "memory", reason: "another-tab" });
    const first = FakeWorker.all[0];
    rt.vol.writeFile("/scratch.txt", enc.encode("in memory"));

    // A call in flight finishes on the engine it started on.
    const inFlight = rt.run(["status"]);
    await vi.waitFor(() => expect(first.calls()).toHaveLength(1));
    const seen: StorageInfo[] = [];
    rt.onStorageChange((info) => seen.push(info));
    const taking = rt.takeOver();
    await new Promise((r) => setTimeout(r, 30));
    expect(FakeWorker.all).toHaveLength(1);
    first.answer(first.calls()[0].id, 0);
    await expect(inFlight).resolves.toBe(0);

    await expect(taking).resolves.toEqual({ kind: "opfs", name: "kapi-x" });
    const owner = FakeWorker.all[1];
    expect(owner.boot?.whenHeld).toBe("take");
    expect(first.terminated).toBe(true);
    expect(seen).toEqual([{ kind: "opfs", name: "kapi-x" }]);
    // The kept workspace's files replace the ones the tab held in memory.
    expect(rt.vol.exists("/scratch.txt")).toBe(false);
    expect(dec.decode(rt.vol.readFile("/kept/a.txt"))).toBe("kept");
    // A tab that holds it already resolves at once.
    await expect(rt.takeOver()).resolves.toEqual({ kind: "opfs", name: "kapi-x" });
    expect(FakeWorker.all).toHaveLength(2);
  });

  it("stays as it was when the workspace cannot be taken over", async () => {
    storageFor = (b) =>
      b.whenHeld === "take"
        ? { kind: "memory", reason: "failed", detail: "the pool's files are held elsewhere" }
        : { kind: "memory", reason: "another-tab" };
    const rt = await boot();
    rt.vol.writeFile("/scratch.txt", enc.encode("in memory"));
    await expect(rt.takeOver()).rejects.toThrow(/held elsewhere/);
    expect(FakeWorker.all[1].terminated).toBe(true);
    expect(FakeWorker.all[0].terminated).toBe(false);
    expect(rt.storage).toEqual({ kind: "memory", reason: "another-tab" });
    expect(rt.vol.exists("/scratch.txt")).toBe(true);
  });

  it("asks the Worker to wait or take over at boot when the page says so", async () => {
    storageFor = () => ({ kind: "opfs", name: "kapi-x" });
    await boot({ whenHeld: "wait" });
    expect(FakeWorker.all[0].boot?.whenHeld).toBe("wait");
  });

  it("runs in memory when the workspace is taken while it starts", async () => {
    storageFor = () => ({ kind: "memory" });
    const { bootKapiRuntime } = await import("./runtime.ts");
    const origPost = FakeWorker.prototype.postMessage;
    let firstBoot = true;
    vi.spyOn(FakeWorker.prototype, "postMessage").mockImplementation(function (
      this: FakeWorker,
      msg: ToWorker,
    ) {
      if (msg.t === "boot" && firstBoot) {
        firstBoot = false;
        this.received.push(msg);
        this.boot = msg.boot;
        queueMicrotask(() => this.emit({ t: "lost" }));
        return;
      }
      origPost.call(this, msg);
    });
    const rt = await bootKapiRuntime("http://test/wasm_exec.js", "http://test/kapi-cli.wasm");
    expect(rt.storage).toEqual({ kind: "memory", reason: "taken" });
    expect(FakeWorker.all[0].terminated).toBe(true);
    expect(FakeWorker.all[1].boot?.persist).toBeNull();
  });
});
