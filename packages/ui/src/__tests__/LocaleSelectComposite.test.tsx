// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import {
  LocaleSelect,
  MultiLocaleSelect,
  type LocaleInfo,
} from "../components/composites/locale-select";
import { LocalePill } from "../components/resource-browser/LocalePill";
import { localeLabel, resolveLocaleName } from "../lib/locale-name";

const locales: LocaleInfo[] = [
  { code: "en", displayName: "English" },
  { code: "fr", displayName: "French" },
  { code: "de", displayName: "German" },
];

// cmdk measures its list and scrolls the active item into view. jsdom has
// neither API, and only the tests that open the popover reach them.
class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as { ResizeObserver?: unknown }).ResizeObserver ??= NoopResizeObserver;
Element.prototype.scrollIntoView ??= function scrollIntoView() {};

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

describe("LocaleSelect (composite)", () => {
  it("names the selection", () => {
    render(<LocaleSelect value="fr" onChange={vi.fn()} locales={locales} />);
    expect(container!.textContent).toContain("French");
  });

  // A caller's catalog is a list of major tags; a real project target is often
  // a regional one that is not in it. Reading that as "nothing selected" hid
  // the selection behind the placeholder.
  it("names a selection its catalog does not list", () => {
    render(<LocaleSelect value="pt-BR" onChange={vi.fn()} locales={locales} />);
    expect(container!.textContent).toContain("Portuguese");
    expect(container!.textContent).not.toContain("Select locale");
  });
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

  it("names a chip whose code its catalog does not list", () => {
    render(<MultiLocaleSelect value={["pt-BR"]} onChange={vi.fn()} locales={locales} />);
    expect(container!.textContent).toContain("Portuguese");
  });

  it("collapses to an inline hint when every locale is selected", () => {
    render(<MultiLocaleSelect value={["en", "fr", "de"]} onChange={vi.fn()} locales={locales} />);
    expect(container!.querySelector("input")).toBeNull();
    expect(container!.textContent).toContain("All locales selected");
  });
});

describe("locale names", () => {
  it("names a major tag and a regional one", () => {
    expect(resolveLocaleName("fr")).toBe("French");
    expect(resolveLocaleName("pt-BR")).toBe("Brazilian Portuguese");
  });

  // qps is our pseudo locale; CLDR echoes the code back rather than naming it.
  // Doubling that into "qps (qps)" is worse than the code alone.
  it("returns an unnamed code once, not twice", () => {
    expect(localeLabel("qps")).toBe("qps");
    expect(localeLabel("fr")).toBe("French (fr)");
  });
});

describe("LocalePill", () => {
  it("keeps the name reachable from a bare pill", () => {
    render(<LocalePill locale="ar" />);
    expect(container!.textContent).toBe("ar");
    expect(container!.querySelector("[title]")?.getAttribute("title")).toBe("Arabic (ar)");
  });

  it("puts the name on the page when asked", () => {
    render(<LocalePill locale="ja" showName />);
    expect(container!.textContent).toContain("Japanese");
    expect(container!.textContent).toContain("ja");
  });
});

describe("LocaleSelect as a filter", () => {
  it("shows the clear label when nothing is selected", () => {
    render(
      <LocaleSelect value="" onChange={vi.fn()} locales={locales} clearLabel="All languages" />,
    );
    expect(container!.textContent).toContain("All languages");
  });

  // Without this the empty value is only a placeholder, and a reviewer who
  // narrowed to French has no way back to every language.
  it("returns to the empty value from the list", () => {
    const onChange = vi.fn();
    render(
      <LocaleSelect value="fr" onChange={onChange} locales={locales} clearLabel="All languages" />,
    );
    act(() => {
      container!
        .querySelector('[role="combobox"]')!
        .dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    const all = Array.from(document.querySelectorAll('[cmdk-item=""]')).find((el) =>
      el.textContent?.includes("All languages"),
    ) as HTMLElement | undefined;
    expect(all).toBeTruthy();
    act(() => {
      all!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(onChange).toHaveBeenCalledWith("");
  });
});
