// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import GemmaExplorer from "./GemmaExplorer";

afterEach(cleanup);

// Press the explicit Run gate to reveal the explorer body. assets=null keeps the
// wasm runtime un-booted, so we exercise render + gating without the engine.
function pressRun() {
  fireEvent.click(screen.getByRole("button", { name: /run/i }));
}

describe("GemmaExplorer", () => {
  it("gates the lab behind an explicit Run (nothing on page load)", () => {
    render(<GemmaExplorer assets={null} />);
    // Before Run: the gate is shown, the Gemma UI is not.
    expect(screen.getByRole("button", { name: /run/i })).toBeTruthy();
    expect(screen.queryByText(/locally in your browser/i)).toBeNull();
  });

  it("renders the local-Gemma UI with seed text and an in-browser note after Run", () => {
    render(<GemmaExplorer assets={null} defaultText="Hello world" defaultTargetLang="de" />);
    pressRun();
    expect(screen.getByText(/Gemma 4|Gemma 4/)).toBeTruthy();
    expect(screen.getByText(/locally in your browser/i)).toBeTruthy();
    // seed text + target language are wired into the inputs
    expect(screen.getByDisplayValue("Hello world")).toBeTruthy();
    expect(screen.getByDisplayValue("de")).toBeTruthy();
  });

  it("disables the translate button until the engine is ready", () => {
    render(<GemmaExplorer assets={null} />);
    pressRun();
    const btn = screen.getByRole("button", { name: /local Gemma/i }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    // a booting hint is shown while assets are null
    expect(screen.getByText(/booting engine/i)).toBeTruthy();
  });
});
