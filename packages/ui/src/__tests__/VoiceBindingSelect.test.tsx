// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { VoiceBindingSelect, type VoiceBindingOption } from "../components/governance";

beforeAll(() => {
  if (typeof globalThis.ResizeObserver === "undefined") {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  }
  if (typeof Element !== "undefined") {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.setPointerCapture ??= () => {};
    Element.prototype.releasePointerCapture ??= () => {};
    Element.prototype.scrollIntoView ??= () => {};
  }
});

afterEach(cleanup);

const options: VoiceBindingOption[] = [
  { value: "file:.kapi/voice.yaml", label: ".kapi/voice.yaml", group: "Files" },
  {
    value: "pack:technical-docs",
    label: "technical-docs",
    group: "Starter packs",
    hint: "read-only",
  },
  { value: "vp-1", label: "Northsea support" },
];

describe("VoiceBindingSelect", () => {
  it("shows the inherit row when nothing is bound", () => {
    render(
      <VoiceBindingSelect
        value={undefined}
        onChange={vi.fn()}
        options={options}
        inheritLabel="Workspace default"
      />,
    );
    expect(screen.getByLabelText("Voice profile").textContent).toContain("Workspace default");
  });

  it("shows the bound option's label", () => {
    render(
      <VoiceBindingSelect
        value="pack:technical-docs"
        onChange={vi.fn()}
        options={options}
        inheritLabel="None bound"
      />,
    );
    expect(screen.getByLabelText("Voice profile").textContent).toContain("technical-docs");
  });

  it("hands back the chosen key, and undefined for the inherit row", async () => {
    const onChange = vi.fn();
    render(
      <VoiceBindingSelect
        value="vp-1"
        onChange={onChange}
        options={options}
        inheritLabel="Inherit (project)"
      />,
    );
    await userEvent.click(screen.getByLabelText("Voice profile"));
    await userEvent.click(screen.getByRole("option", { name: /technical-docs/ }));
    expect(onChange).toHaveBeenLastCalledWith("pack:technical-docs");

    await userEvent.click(screen.getByLabelText("Voice profile"));
    await userEvent.click(screen.getByRole("option", { name: "Inherit (project)" }));
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it("lists grouped options under their heading, with the hint", async () => {
    render(
      <VoiceBindingSelect
        value={undefined}
        onChange={vi.fn()}
        options={options}
        inheritLabel="None bound"
      />,
    );
    await userEvent.click(screen.getByLabelText("Voice profile"));
    expect(screen.getByText("Files")).toBeTruthy();
    expect(screen.getByText("Starter packs")).toBeTruthy();
    const pack = screen.getByRole("option", { name: /technical-docs/ });
    expect(within(pack).getByText("read-only")).toBeTruthy();
  });

  it("keeps a binding no option names visible, marked as not found", async () => {
    render(
      <VoiceBindingSelect
        value="file:.kapi/gone.yaml"
        onChange={vi.fn()}
        options={options}
        inheritLabel="None bound"
      />,
    );
    expect(screen.getByLabelText("Voice profile").textContent).toContain(".kapi/gone.yaml");
    await userEvent.click(screen.getByLabelText("Voice profile"));
    expect(screen.getByTestId("voice-binding-missing").textContent).toContain("not found");
  });

  it("uses the given label and help, and can be disabled", () => {
    render(
      <VoiceBindingSelect
        value={undefined}
        onChange={vi.fn()}
        options={options}
        inheritLabel="None bound"
        label="Voice"
        help="Streams and collections can override it."
        disabled
      />,
    );
    expect(screen.getByLabelText("Voice")).toHaveProperty("disabled", true);
    expect(screen.getByText("Streams and collections can override it.")).toBeTruthy();
  });
});
