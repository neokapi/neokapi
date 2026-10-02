// @vitest-environment jsdom
import React, { createRef, useImperativeHandle } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import KapiEmbed, {
  type KapiEmbedHandle,
} from "../../../../packages/kapi-playground/src/KapiEmbed";

const mocks = vi.hoisted(() => ({ boot: vi.fn(), run: vi.fn(), write: vi.fn() }));
vi.mock("../../../../packages/kapi-playground/src/plugins", () => ({
  configurePlugins: vi.fn(),
  bootEngine: mocks.boot,
}));
vi.mock("../../../../packages/kapi-playground/src/runtime", () => ({
  isBooted: () => false,
  onBootProgress: () => () => {},
}));
vi.mock("../../../../packages/kapi-playground/src/FilesPanel", () => ({
  default: () => <p>Files ready</p>,
}));
vi.mock("../../../../packages/kapi-playground/src/KapiTerminal", () => ({
  default: ({ ref }: { ref: React.Ref<unknown> }) => {
    useImperativeHandle(ref, () => ({ runCommand: mocks.run, focus: vi.fn() }));
    return <p>Terminal ready</p>;
  },
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it("retries a failed engine load and honors a sample selected during loading", async () => {
  let finish: (value: unknown) => void = () => {};
  mocks.boot.mockRejectedValueOnce(new Error("Download interrupted")).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  mocks.run.mockResolvedValue(undefined);
  const ref = createRef<KapiEmbedHandle>();
  render(
    <KapiEmbed
      ref={ref}
      wasmUrl="engine.wasm"
      wasmExecUrl="wasm_exec.js"
      bootOnMount
      cmd="kapi stats original.json"
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "Retry loading" }));
  ref.current?.openWith({
    files: [{ path: "chosen.json", content: "{}" }],
    cmd: "kapi stats chosen.json",
    autoRun: false,
  });
  finish({
    cwd: () => "/project",
    vol: { exists: () => false, writeFile: mocks.write, mkdirp: vi.fn() },
  });
  await screen.findByText("Terminal ready");
  await waitFor(() => expect(mocks.run).toHaveBeenCalledWith("kapi stats chosen.json", false));
  expect(mocks.boot).toHaveBeenCalledTimes(2);
  expect(mocks.write.mock.calls[0][0]).toBe("/project/chosen.json");
  expect(Array.from(mocks.write.mock.calls[0][1])).toEqual([123, 125]);
});
