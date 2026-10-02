// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./memfs.ts", () => ({
  createMemFS: () => ({ fs: {}, process: {}, vol: {} }),
}));

// SQLite is loaded and bridged beside the engine; these tests are about the
// engine's own boot, so the bridge is a stand-in.
const sqlite = vi.hoisted(() => ({
  loadSQLite: vi.fn(async () => ({})),
  installSQLiteBridge: vi.fn(),
}));
vi.mock("./sqlite.ts", () => sqlite);

const originalProcess = (globalThis as Record<string, unknown>).process;
const originalFS = (globalThis as Record<string, unknown>).fs;
const wasmBytes = new Uint8Array([0, 97, 115, 109, 1, 0, 0, 0]);

function installGo(fail = false) {
  vi.stubGlobal(
    "Go",
    class {
      importObject = {};
      env = {};
      async run() {
        if (fail) throw new Error("startup failed");
        globalThis.__kapiCliReady?.();
        await new Promise(() => {});
      }
    },
  );
}

function loaded() {
  const script = document.querySelector("script[data-kapi-wasm-exec]");
  expect(script).not.toBeNull();
  script!.dispatchEvent(new Event("load"));
}

beforeEach(() => {
  vi.resetModules();
  sqlite.loadSQLite.mockClear();
  sqlite.installSQLiteBridge.mockClear();
  vi.stubGlobal("DecompressionStream", undefined);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(wasmBytes)),
  );
  vi.stubGlobal("kapiRun", vi.fn());
  vi.stubGlobal("kapiPreview", vi.fn());
  vi.stubGlobal("labInspect", vi.fn());
  installGo();
});

afterEach(() => {
  document.querySelectorAll("script[data-kapi-wasm-exec]").forEach((script) => script.remove());
  (globalThis as Record<string, unknown>).process = originalProcess;
  (globalThis as Record<string, unknown>).fs = originalFS;
  delete globalThis.__kapiCliReady;
  vi.unstubAllGlobals();
});

describe("engine boot", () => {
  it("loads SQLite beside the engine and bridges it before Go starts", async () => {
    let bridgedBeforeRun = false;
    vi.stubGlobal(
      "Go",
      class {
        importObject = {};
        env = {};
        async run() {
          bridgedBeforeRun = sqlite.installSQLiteBridge.mock.calls.length === 1;
          globalThis.__kapiCliReady?.();
          await new Promise(() => {});
        }
      },
    );
    const { bootKapiRuntime } = await import("./runtime.ts");
    const booted = bootKapiRuntime("/wasm_exec.js", "/assets/kapi-cli.wasm");
    loaded();
    await booted;
    expect(sqlite.loadSQLite).toHaveBeenCalledWith({ wasmUrl: "/assets/sqlite3.wasm" });
    expect(bridgedBeforeRun).toBe(true);
  });

  it("takes the SQLite asset from where the host says it is", async () => {
    const { bootKapiRuntime } = await import("./runtime.ts");
    const booted = bootKapiRuntime("/wasm_exec.js", "/engine.wasm", {
      sqliteWasmUrl: "https://cdn.example.test/sqlite3.wasm",
    });
    loaded();
    await booted;
    expect(sqlite.loadSQLite).toHaveBeenCalledWith({
      wasmUrl: "https://cdn.example.test/sqlite3.wasm",
    });
  });

  it("fails boot when SQLite does not load, and boots on a retry", async () => {
    sqlite.loadSQLite.mockRejectedValueOnce(new Error("sqlite3.wasm 404"));
    const { bootKapiRuntime } = await import("./runtime.ts");
    const failed = bootKapiRuntime("/wasm_exec.js", "/engine.wasm");
    loaded();
    await expect(failed).rejects.toThrow("sqlite3.wasm 404");
    await expect(bootKapiRuntime("/wasm_exec.js", "/engine.wasm")).resolves.toBeDefined();
  });
});

describe("engine boot retries", () => {
  it("removes a failed script and shares the successful retry", async () => {
    const { bootKapiRuntime, isBooted } = await import("./runtime.ts");
    const failed = bootKapiRuntime("/wasm_exec.js", "/engine.wasm");
    document.querySelector("script[data-kapi-wasm-exec]")!.dispatchEvent(new Event("error"));
    await expect(failed).rejects.toThrow("failed to load");
    expect(isBooted()).toBe(false);
    expect(document.querySelector("script[data-kapi-wasm-exec]")).toBeNull();
    const retry = bootKapiRuntime("/wasm_exec.js", "/engine.wasm");
    expect(bootKapiRuntime("/wasm_exec.js", "/engine.wasm")).toBe(retry);
    loaded();
    const runtime = await retry;
    expect(await bootKapiRuntime("/wasm_exec.js", "/engine.wasm")).toBe(runtime);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("retries a failed asset download without reloading the successful script", async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error("offline"));
    const { bootKapiRuntime } = await import("./runtime.ts");
    const failed = bootKapiRuntime("/wasm_exec.js", "/engine.wasm");
    loaded();
    await expect(failed).rejects.toThrow("offline");
    await expect(bootKapiRuntime("/wasm_exec.js", "/engine.wasm")).resolves.toBeDefined();
    expect(document.querySelectorAll("script[data-kapi-wasm-exec]")).toHaveLength(1);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("rejects failed Go startup and allows a fresh attempt", async () => {
    installGo(true);
    const { bootKapiRuntime } = await import("./runtime.ts");
    const failed = bootKapiRuntime("/wasm_exec.js", "/engine.wasm");
    loaded();
    await expect(failed).rejects.toThrow("startup failed");
    installGo();
    await expect(bootKapiRuntime("/wasm_exec.js", "/engine.wasm")).resolves.toBeDefined();
  });

  it("removes a loaded script that did not define Go", async () => {
    vi.stubGlobal("Go", undefined);
    const { bootKapiRuntime } = await import("./runtime.ts");
    const failed = bootKapiRuntime("/wasm_exec.js", "/engine.wasm");
    loaded();
    await expect(failed).rejects.toThrow("did not define Go");
    expect(document.querySelector("script[data-kapi-wasm-exec]")).toBeNull();
    installGo();
    const retry = bootKapiRuntime("/wasm_exec.js", "/engine.wasm");
    loaded();
    await expect(retry).resolves.toBeDefined();
  });
});
