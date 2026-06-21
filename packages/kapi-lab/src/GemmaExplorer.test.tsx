// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import GemmaExplorer from "./GemmaExplorer";

afterEach(cleanup);

// The lab gates work behind the shared zero-shift GateOverlay: the body lays out
// from the start and the play button (aria-label "Run") covers it until the
// engine is ready. assets=null keeps the runtime un-booted, so we exercise the
// rendered body without the engine.

describe("GemmaExplorer", () => {
  it("gates the lab behind an explicit Run play button", () => {
    render(<GemmaExplorer assets={null} />);
    expect(screen.getByLabelText("Run")).toBeTruthy();
  });

  it("renders the local-Gemma UI with seed text and an in-browser note, without booting WASM", () => {
    render(<GemmaExplorer assets={null} defaultText="Hello world" defaultTargetLang="de" />);
    expect(screen.getByText(/Gemma 4/)).toBeTruthy();
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
