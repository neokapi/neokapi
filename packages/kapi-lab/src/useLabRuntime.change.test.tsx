// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";

// The change contract's calls go to the booted engine's facade, in turn with
// the commands the lab runs: the engine has one App, so a read or an apply
// waits for the command before it.
const engine = {
  run: vi.fn<(argv: string[]) => Promise<number>>(),
  read: vi.fn(),
  apply: vi.fn(),
  describe: vi.fn(),
};

vi.mock("@neokapi/kapi-playground/runtime", () => ({
  onBootProgress: vi.fn(() => () => {}),
}));
vi.mock("@neokapi/kapi-playground/plugins", () => ({
  configurePlugins: vi.fn(),
  bootEngine: vi.fn(async () => engine as never),
}));

import { useLabRuntime } from "./useLabRuntime";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const assets = { wasmExecUrl: "/x/exec.js", wasmUrl: "/x/k.wasm" };

test("the change contract's calls reject before the engine is up", async () => {
  const { result } = renderHook(() => useLabRuntime(assets));
  await expect(result.current.read({ doc: "/project/a.json" })).rejects.toThrow(
    "runtime not ready",
  );
  await expect(result.current.apply({ ops: [] })).rejects.toThrow("runtime not ready");
  await expect(result.current.describe({ format: "json" })).rejects.toThrow("runtime not ready");
});

test("an apply waits for the command the lab started before it", async () => {
  let finishRun: (code: number) => void = () => {};
  engine.run.mockImplementation(() => new Promise<number>((resolve) => (finishRun = resolve)));
  const result = {
    schema: "kapi.change-result/v1",
    status: "applied",
    record: "op_1",
    docs: [],
    ops: [],
  };
  engine.apply.mockResolvedValue(result);

  const { result: hook } = renderHook(() => useLabRuntime(assets, { autoBoot: true }));
  await waitFor(() => expect(hook.current.ready).toBe(true));

  let running!: Promise<number>;
  let applying!: Promise<unknown>;
  const set = { ops: [] };
  act(() => {
    running = hook.current.run(["extract", "-p", "/project/kapi.yaml"]);
    applying = hook.current.apply(set, { project: "/project" });
  });
  await waitFor(() => expect(engine.run).toHaveBeenCalled());
  expect(engine.apply).not.toHaveBeenCalled();

  finishRun(0);
  await expect(running).resolves.toBe(0);
  await expect(applying).resolves.toEqual(result);
  expect(engine.apply).toHaveBeenCalledWith(set, { project: "/project" });
});
