// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MultiLocaleSelect, type LocaleInfo } from "../components/composites/locale-select";

const locales: LocaleInfo[] = [
  { code: "en", displayName: "English" },
  { code: "fr", displayName: "French" },
  { code: "de", displayName: "German" },
];

let container: HTMLDivElement | null = null;
let root: Root | null = null;

function render(el: React.ReactElement) {
  container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    root = createRoot(container!);
    root.render(el);
  });
}

afterEach(() => {
  act(() => root?.unmount());
  container?.remove();
  container = null;
  root = null;
});

describe("MultiLocaleSelect (composite)", () => {
  it("renders a chip per selected locale", () => {
    render(<MultiLocaleSelect value={["en", "fr"]} onChange={vi.fn()} locales={locales} />);
    expect(container!.textContent).toContain("English");
    expect(container!.textContent).toContain("French");
  });

  it("renders the add-picker input when locales remain", () => {
    render(<MultiLocaleSelect value={["en"]} onChange={vi.fn()} locales={locales} />);
    expect(container!.querySelector("input")).not.toBeNull();
  });

  it("removes a locale when its chip remove control is activated", () => {
    const onChange = vi.fn();
    render(<MultiLocaleSelect value={["en", "fr"]} onChange={onChange} locales={locales} />);
    const remove = Array.from(container!.querySelectorAll('[role="button"]')).find((el) =>
      el.parentElement?.textContent?.includes("French"),
    ) as HTMLElement | undefined;
    expect(remove).toBeTruthy();
    act(() => {
      remove!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(onChange).toHaveBeenCalledWith(["en"]);
  });

  it("collapses to an inline hint when every locale is selected", () => {
    render(<MultiLocaleSelect value={["en", "fr", "de"]} onChange={vi.fn()} locales={locales} />);
    expect(container!.querySelector("input")).toBeNull();
    expect(container!.textContent).toContain("All locales selected");
  });
});
