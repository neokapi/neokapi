import { describe, it, expect, vi, beforeEach } from "vitest";

describe("writeClipboardText", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  it("prefers Wails' native Clipboard.SetText over the browser API", async () => {
    const { Clipboard } = await import("@wailsio/runtime");
    const writeText = vi.fn();
    Object.assign(navigator, { clipboard: { writeText } });

    const { writeClipboardText } = await import("./clipboard");
    await writeClipboardText("hello");

    expect(Clipboard.SetText).toHaveBeenCalledWith("hello");
    expect(writeText).not.toHaveBeenCalled();
  });

  it("falls back to the browser API outside a Wails window", async () => {
    vi.doMock("@wailsio/runtime", () => {
      throw new Error("no wails runtime here");
    });
    const writeText = vi.fn();
    Object.assign(navigator, { clipboard: { writeText } });

    const { writeClipboardText } = await import("./clipboard");
    await writeClipboardText("hello");

    expect(writeText).toHaveBeenCalledWith("hello");
  });
});
