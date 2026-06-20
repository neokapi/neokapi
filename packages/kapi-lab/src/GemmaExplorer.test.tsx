// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import GemmaExplorer from "./GemmaExplorer";

afterEach(cleanup);

describe("GemmaExplorer", () => {
  // assets=null keeps the wasm runtime un-booted, so we exercise the render +
  // gating without needing the wasm engine (the real run is validated in-browser).
  it("renders the local-Gemma UI with seed text and an in-browser note", () => {
    render(<GemmaExplorer assets={null} defaultText="Hello world" defaultTargetLang="de" />);
    expect(screen.getByText(/Gemma 4|Gemma 4/)).toBeTruthy();
    expect(screen.getByText(/locally in your browser/i)).toBeTruthy();
    // seed text + target language are wired into the inputs
    expect(screen.getByDisplayValue("Hello world")).toBeTruthy();
    expect(screen.getByDisplayValue("de")).toBeTruthy();
  });

  it("disables the translate button until the engine is ready", () => {
    render(<GemmaExplorer assets={null} />);
    const btn = screen.getByRole("button", { name: /local Gemma/i }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    // a booting hint is shown while assets are null
    expect(screen.getByText(/booting engine/i)).toBeTruthy();
  });
});
