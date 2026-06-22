// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import ModelsExplorer from "./ModelsExplorer";

afterEach(cleanup);

// assets=null keeps the runtime un-booted, so we exercise the rendered body
// (gated behind the zero-shift GateOverlay) without booting WASM.
describe("ModelsExplorer", () => {
  it("gates the lab behind an explicit Run play button", () => {
    render(<ModelsExplorer assets={null} />);
    expect(screen.getByLabelText("Run")).toBeTruthy();
  });

  it("models the three sources and the per-platform local engine", () => {
    render(<ModelsExplorer assets={null} defaultText="Hello world" defaultTargetLang="de" />);
    // The three sources of models, by a unique phrase from each.
    expect(screen.getByText(/Local · on-device/)).toBeTruthy();
    expect(screen.getByText(/require an API key/)).toBeTruthy();
    expect(screen.getByText(/add formats/)).toBeTruthy();
    // Per-platform local engine is made explicit (browser→WebLLM, desktop→Ollama).
    expect(screen.getAllByText(/Ollama/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/WebLLM/).length).toBeGreaterThan(0);
    // Seed text + target language are wired into the inputs.
    expect(screen.getByDisplayValue("Hello world")).toBeTruthy();
    expect(screen.getByDisplayValue("de")).toBeTruthy();
  });

  it("shows the equivalent CLI command for the selected model", () => {
    render(<ModelsExplorer assets={null} defaultTargetLang="fr" />);
    expect(
      screen.getByText(/kapi translate input\.json --provider ollama --model llama3\.2:3b/),
    ).toBeTruthy();
  });

  it("disables the translate button until the engine is ready", () => {
    render(<ModelsExplorer assets={null} />);
    const btn = screen.getByRole("button", { name: /Translate locally/i }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    expect(screen.getByText(/booting engine/i)).toBeTruthy();
  });
});
