// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { ALL_LANGUAGES, ReviewLanguageSelect, type ReviewLanguageLane } from "../components/review";

beforeAll(() => {
  // cmdk measures its list and scrolls the active item into view; Radix's
  // popover captures the pointer. jsdom has neither API.
  if (typeof globalThis.ResizeObserver === "undefined") {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  }
  Element.prototype.scrollIntoView ??= () => {};
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.setPointerCapture ??= () => {};
  Element.prototype.releasePointerCapture ??= () => {};
});

afterEach(cleanup);

/** The shape `convergence.SummarizeReviewLanguages` hands a surface. */
const lanes: ReviewLanguageLane[] = [
  { language: "en-US", source: true, pending: 3 },
  { language: "fr-FR", pending: 24 },
  { language: "de-DE", pending: 11 },
];

function options() {
  return Array.from(document.querySelectorAll("[data-slot='review-language-option']"));
}

describe("ReviewLanguageSelect", () => {
  it("offers every lane with its pending count, in the order the caller gave", async () => {
    const user = userEvent.setup();
    render(
      <ReviewLanguageSelect value={ALL_LANGUAGES} lanes={lanes} onChange={vi.fn()} allowAll />,
    );

    await user.click(screen.getByRole("combobox"));
    const listed = options();
    expect(listed).toHaveLength(3);
    expect(listed.map((el) => el.getAttribute("data-language"))).toEqual([
      "en-US",
      "fr-FR",
      "de-DE",
    ]);
    // The source lane is marked as the source rather than named a lane of its own.
    expect(listed[0].textContent).toContain("English");
    expect(listed[0].textContent?.toLowerCase()).toContain("source");
    expect(listed.map((el) => el.textContent)).toEqual([
      expect.stringContaining("3"),
      expect.stringContaining("24"),
      expect.stringContaining("11"),
    ]);
  });

  it("shows the total behind All languages and reports the sentinel when it is chosen", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <ReviewLanguageSelect
        value="fr-FR"
        lanes={lanes}
        onChange={onChange}
        allowAll
        allPending={38}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    const all = document.querySelector("[data-slot='review-language-all']");
    expect(all?.textContent).toContain("All languages");
    expect(all?.textContent).toContain("38");

    await user.click(all as HTMLElement);
    expect(onChange).toHaveBeenCalledWith(ALL_LANGUAGES);
  });

  it("leaves All languages out when the surface does not offer it", async () => {
    const user = userEvent.setup();
    render(<ReviewLanguageSelect value="fr-FR" lanes={lanes} onChange={vi.fn()} />);

    await user.click(screen.getByRole("combobox"));
    expect(document.querySelector("[data-slot='review-language-all']")).toBeNull();
  });

  it("reports the tag of the lane a reviewer picks", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <ReviewLanguageSelect value={ALL_LANGUAGES} lanes={lanes} onChange={onChange} allowAll />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(options()[2] as HTMLElement);
    expect(onChange).toHaveBeenCalledWith("de-DE");
  });

  it("names the chosen lane and its count on the closed trigger", () => {
    render(
      <ReviewLanguageSelect
        value="fr-FR"
        lanes={lanes}
        onChange={vi.fn()}
        allowAll
        allPending={38}
      />,
    );
    const trigger = screen.getByRole("combobox");
    expect(trigger.textContent).toContain("French");
    expect(trigger.textContent).toContain("24");
    expect(trigger.textContent).not.toContain("All languages");
  });

  it("keeps an entry for a chosen language the queue no longer lists", async () => {
    const user = userEvent.setup();
    render(<ReviewLanguageSelect value="ja-JP" lanes={lanes} onChange={vi.fn()} allowAll />);

    expect(screen.getByRole("combobox").textContent).toContain("Japanese");
    await user.click(screen.getByRole("combobox"));
    const listed = options();
    expect(listed).toHaveLength(4);
    expect(listed[3].getAttribute("data-language")).toBe("ja-JP");
  });

  it("counts nothing where the surface counts nothing", async () => {
    const user = userEvent.setup();
    const uncounted: ReviewLanguageLane[] = [
      { language: "en-US", source: true },
      { language: "fr-FR" },
    ];
    render(<ReviewLanguageSelect value="fr-FR" lanes={uncounted} onChange={vi.fn()} />);

    expect(screen.getByRole("combobox").textContent).not.toMatch(/\d/);
    await user.click(screen.getByRole("combobox"));
    expect(options().every((el) => !/\d/.test(el.textContent ?? ""))).toBe(true);
  });

  it("asks for a language when none is chosen yet", () => {
    render(<ReviewLanguageSelect value="" lanes={lanes} onChange={vi.fn()} />);
    expect(screen.getByRole("combobox").textContent).toContain("Choose a language");
  });

  it("names a lane the way the workspace names it, when it has its own name", async () => {
    const user = userEvent.setup();
    render(
      <ReviewLanguageSelect
        value="fr-FR"
        lanes={[{ language: "fr-FR", displayName: "French (Canada office)", pending: 2 }]}
        onChange={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    expect(options()[0].textContent).toContain("French (Canada office)");
  });

  it("matches typed text against the language name as well as its tag", async () => {
    const user = userEvent.setup();
    render(
      <ReviewLanguageSelect value={ALL_LANGUAGES} lanes={lanes} onChange={vi.fn()} allowAll />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText("Search languages"), "German");
    expect(options().map((el) => el.getAttribute("data-language"))).toEqual(["de-DE"]);
  });

  it("says so when nothing matches what was typed", async () => {
    const user = userEvent.setup();
    render(
      <ReviewLanguageSelect value={ALL_LANGUAGES} lanes={lanes} onChange={vi.fn()} allowAll />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText("Search languages"), "Klingon");
    expect(screen.getByText("No language matches.")).toBeTruthy();
  });

  it("derives a test hook for the trigger and for every entry", async () => {
    const user = userEvent.setup();
    render(
      <ReviewLanguageSelect
        value={ALL_LANGUAGES}
        lanes={lanes}
        onChange={vi.fn()}
        allowAll
        data-testid="filter-language"
      />,
    );

    await user.click(screen.getByTestId("filter-language"));
    expect(await screen.findByTestId("filter-language-all")).toBeTruthy();
    expect(await screen.findByTestId("filter-language-de-DE")).toBeTruthy();
  });

  it("carries the accessible name a toolbar gives it", () => {
    render(
      <ReviewLanguageSelect
        value="fr-FR"
        lanes={lanes}
        onChange={vi.fn()}
        label="Language to review"
      />,
    );
    expect(screen.getByRole("combobox", { name: "Language to review" })).toBeTruthy();
  });
});
