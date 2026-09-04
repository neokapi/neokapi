// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import {
  CoordinatesEditor,
  incompleteAxes,
  type CoordinateAxisOption,
} from "../components/governance";

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

const axes: CoordinateAxisOption[] = [
  {
    axis: "product",
    declarable: false,
    refusal: 'recipe: "product" is derived from a collection\'s channel, not declared',
  },
  { axis: "brand" },
  { axis: "mode", values: ["tutorial", "how-to", "reference"] },
];

describe("CoordinatesEditor", () => {
  it("says what an empty map means", () => {
    render(
      <CoordinatesEditor
        value={{}}
        onChange={vi.fn()}
        axes={axes}
        emptyText="Every collection sits at the project's own point."
        note="product comes from a collection's channel."
      />,
    );
    expect(screen.getByText("Every collection sits at the project's own point.")).toBeTruthy();
    expect(screen.getByText("product comes from a collection's channel.")).toBeTruthy();
    expect(screen.queryAllByTestId("coordinate-row")).toHaveLength(0);
  });

  it("edits a declared axis by name", async () => {
    const onChange = vi.fn();
    render(<CoordinatesEditor value={{ brand: "northsea" }} onChange={onChange} axes={axes} />);
    const input = within(screen.getByTestId("coordinates-editor")).getByLabelText("brand");
    await userEvent.clear(input);
    expect(onChange).toHaveBeenLastCalledWith({ brand: "" });
  });

  it("offers a closed value set as a select", async () => {
    const onChange = vi.fn();
    render(<CoordinatesEditor value={{ mode: "tutorial" }} onChange={onChange} axes={axes} />);
    await userEvent.click(screen.getByLabelText("mode"));
    await userEvent.click(screen.getByRole("option", { name: "reference" }));
    expect(onChange).toHaveBeenLastCalledWith({ mode: "reference" });
  });

  it("removes an axis, and hands back undefined when it was the last", async () => {
    const onChange = vi.fn();
    render(<CoordinatesEditor value={{ brand: "northsea" }} onChange={onChange} axes={axes} />);
    await userEvent.click(screen.getByRole("button", { name: "Remove brand" }));
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it("refuses a structural axis with the consumer's own words", async () => {
    const onChange = vi.fn();
    render(<CoordinatesEditor value={{}} onChange={onChange} axes={axes} allowNewAxis />);
    await userEvent.type(screen.getByLabelText("New axis"), "product");
    expect(screen.getByTestId("axis-refusal").textContent).toContain("derived from a collection");
    expect(screen.getByRole("button", { name: /Add axis/ })).toHaveProperty("disabled", true);
    expect(onChange).not.toHaveBeenCalled();
  });

  it("adds a free-text axis when the consumer allows one", async () => {
    const onChange = vi.fn();
    render(
      <CoordinatesEditor
        value={{ brand: "northsea" }}
        onChange={onChange}
        axes={axes}
        allowNewAxis
      />,
    );
    await userEvent.type(screen.getByLabelText("New axis"), "region");
    await userEvent.click(screen.getByRole("button", { name: /Add axis/ }));
    expect(onChange).toHaveBeenLastCalledWith({ brand: "northsea", region: "" });
  });

  it("offers only the declarable axes not yet declared when free text is off", async () => {
    const onChange = vi.fn();
    render(<CoordinatesEditor value={{ brand: "northsea" }} onChange={onChange} axes={axes} />);
    await userEvent.click(screen.getByLabelText("New axis"));
    expect(screen.queryByRole("option", { name: "product" })).toBeNull();
    expect(screen.queryByRole("option", { name: "brand" })).toBeNull();
    await userEvent.click(screen.getByRole("option", { name: "mode" }));
    await userEvent.click(screen.getByRole("button", { name: /Add axis/ }));
    expect(onChange).toHaveBeenLastCalledWith({ brand: "northsea", mode: "" });
  });

  it("offers nothing to add once every declarable axis is declared", () => {
    render(
      <CoordinatesEditor
        value={{ brand: "northsea", mode: "tutorial" }}
        onChange={vi.fn()}
        axes={axes}
      />,
    );
    expect(screen.queryByLabelText("New axis")).toBeNull();
  });

  it("marks a row without a value when values are required", () => {
    render(
      <CoordinatesEditor
        value={{ brand: "", channel: "support" }}
        onChange={vi.fn()}
        allowNewAxis
        requireValues
      />,
    );
    expect(screen.getByTestId("coordinate-value-required").textContent).toBe(
      "brand needs a value.",
    );
    expect(screen.getByLabelText("brand").getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByLabelText("channel").getAttribute("aria-invalid")).toBeNull();
  });

  it("disables every control", () => {
    render(
      <CoordinatesEditor
        value={{ brand: "northsea" }}
        onChange={vi.fn()}
        axes={axes}
        allowNewAxis
        disabled
      />,
    );
    expect(screen.getByLabelText("brand")).toHaveProperty("disabled", true);
    expect(screen.getByLabelText("New axis")).toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: "Remove brand" })).toHaveProperty("disabled", true);
  });
});

describe("incompleteAxes", () => {
  it("names the axes whose value is blank", () => {
    expect(incompleteAxes({ brand: "acme", channel: " ", mode: "" })).toEqual(["channel", "mode"]);
    expect(incompleteAxes({ brand: "acme" })).toEqual([]);
    expect(incompleteAxes(undefined)).toEqual([]);
  });
});
