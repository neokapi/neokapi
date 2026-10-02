import { beforeEach, expect, it, vi } from "vitest";

const { boot, install } = vi.hoisted(() => ({ boot: vi.fn(), install: vi.fn() }));
vi.mock("@neokapi/engine/runtime", () => ({ bootKapiRuntime: boot }));
vi.mock("./pdfiumBridge", () => ({ installPdfiumBridge: install }));

beforeEach(() => {
  vi.resetModules();
  boot.mockReset();
  install.mockReset();
});

it("retries a rejected engine boot and retains the successful runtime", async () => {
  const runtime = {};
  boot.mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce(runtime);
  const { bootKapiRuntime } = await import("./runtime");
  await expect(bootKapiRuntime("/wasm_exec.js", "/kapi.wasm")).rejects.toThrow("offline");
  const retry = bootKapiRuntime("/wasm_exec.js", "/kapi.wasm");
  expect(bootKapiRuntime("/wasm_exec.js", "/kapi.wasm")).toBe(retry);
  expect(await retry).toBe(runtime);
  expect(await bootKapiRuntime("/wasm_exec.js", "/kapi.wasm")).toBe(runtime);
  expect(boot).toHaveBeenCalledTimes(2);
  expect(install).toHaveBeenCalledOnce();
});
