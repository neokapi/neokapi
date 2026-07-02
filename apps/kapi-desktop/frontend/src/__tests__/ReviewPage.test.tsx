import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { ReviewPage, type ReviewDecision } from "../components/ReviewPage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { ReviewItem, ReviewUnitDetail } from "../types/api";

const ITEMS: ReviewItem[] = [
  {
    locale: "de-DE",
    file: "locales/de-DE.json",
    key: "greeting",
    collection: "App",
    source: "Hello {name}",
    target: "Hallo",
    hasFindings: true,
  },
  {
    locale: "fr-FR",
    file: "locales/fr-FR.json",
    key: "greeting",
    collection: "App",
    source: "Hello {name}",
    target: "Bonjour {name}",
    hasFindings: false,
  },
  {
    locale: "fr-FR",
    file: "locales/fr-FR.json",
    key: "farewell",
    collection: "Docs",
    source: "Goodbye",
    target: "Au revoir",
    hasFindings: false,
  },
];

function unitFor(item: ReviewItem): ReviewUnitDetail {
  return {
    locale: item.locale,
    file: item.file,
    key: item.key,
    collection: item.collection,
    source: item.source,
    target: item.target ?? "",
    status: "translated",
    findings: item.hasFindings
      ? [
          {
            category: "placeholder",
            severity: "major",
            message: "placeholder {name} missing from target",
            fixable: false,
          },
        ]
      : [],
    editable: true,
  };
}

function renderPage(overrides: Partial<Parameters<typeof ReviewPage>[0]> = {}) {
  const decisions: Array<{ item: ReviewItem; decision: ReviewDecision; note?: string }> = [];
  const onDecide = vi.fn(async (item: ReviewItem, decision: ReviewDecision, note?: string) => {
    decisions.push({ item, decision, note });
  });
  const loadUnit = vi.fn(async (item: ReviewItem) => unitFor(item));
  const utils = render(
    <ErrorProvider>
      <ReviewPage tabID="t1" items={ITEMS} loadUnit={loadUnit} onDecide={onDecide} {...overrides} />
    </ErrorProvider>,
  );
  return { ...utils, onDecide, loadUnit, decisions };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ReviewPage", () => {
  it("renders the queue grouped by file, findings-first", async () => {
    renderPage();
    const items = await screen.findAllByRole("button", { name: /greeting|farewell/i });
    expect(items.length).toBeGreaterThanOrEqual(3);
    const queueItems = document.querySelectorAll("[data-slot='review-queue-item']");
    expect(queueItems).toHaveLength(3);
    // Findings-first ordering: the de-DE unit (has findings) leads the queue.
    expect(queueItems[0].getAttribute("data-key")).toBe("greeting");
    expect(queueItems[0].textContent).toContain("de-DE");
    // Grouped by file: both file headers render.
    expect(screen.getByText("locales/de-DE.json")).toBeInTheDocument();
    expect(screen.getByText("locales/fr-FR.json")).toBeInTheDocument();
  });

  it("selects the first unit and shows its source, target and findings", async () => {
    const { loadUnit } = renderPage();
    await waitFor(() => expect(loadUnit).toHaveBeenCalled());
    expect(loadUnit.mock.calls[0][0].locale).toBe("de-DE");
    await screen.findByText("placeholder {name} missing from target");
    const source = document.querySelector("[data-slot='review-source']");
    expect(source?.textContent).toBe("Hello {name}");
    const target = document.querySelector("[data-slot='review-target']") as HTMLTextAreaElement;
    expect(target.value).toBe("Hallo");
  });

  it("approves with the keyboard and advances to the next unit", async () => {
    const { onDecide } = renderPage();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-queue-item'][data-active]")).not.toBeNull(),
    );
    fireEvent.keyDown(window, { key: "a" });
    await waitFor(() => expect(onDecide).toHaveBeenCalledTimes(1));
    expect(onDecide.mock.calls[0][1]).toBe("approved");
    expect(onDecide.mock.calls[0][0].locale).toBe("de-DE");
    // The approved unit left the queue; the next one is selected.
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(2),
    );
    await waitFor(() => {
      const active = document.querySelector("[data-slot='review-queue-item'][data-active]");
      expect(active).not.toBeNull();
    });
  });

  it("navigates with j/k", async () => {
    renderPage();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-queue-item'][data-active]")).not.toBeNull(),
    );
    const activeKey = () =>
      document
        .querySelector("[data-slot='review-queue-item'][data-active]")
        ?.getAttribute("data-key");
    const first = activeKey();
    fireEvent.keyDown(window, { key: "j" });
    await waitFor(() => expect(activeKey()).not.toBe(first));
    fireEvent.keyDown(window, { key: "k" });
    await waitFor(() => expect(activeKey()).toBe(first));
  });

  it("rejecting prompts for a note and records it", async () => {
    const promptSpy = vi.spyOn(window, "prompt").mockReturnValue("too literal");
    const { onDecide } = renderPage();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-queue-item'][data-active]")).not.toBeNull(),
    );
    fireEvent.keyDown(window, { key: "r" });
    await waitFor(() => expect(onDecide).toHaveBeenCalledTimes(1));
    expect(promptSpy).toHaveBeenCalled();
    expect(onDecide.mock.calls[0][1]).toBe("rejected");
    expect(onDecide.mock.calls[0][2]).toBe("too literal");
  });

  it("cancelling the reject prompt records nothing", async () => {
    vi.spyOn(window, "prompt").mockReturnValue(null);
    const { onDecide } = renderPage();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-queue-item'][data-active]")).not.toBeNull(),
    );
    fireEvent.keyDown(window, { key: "r" });
    await new Promise((r) => setTimeout(r, 20));
    expect(onDecide).not.toHaveBeenCalled();
  });

  it("signs off with s", async () => {
    const { onDecide } = renderPage();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-queue-item'][data-active]")).not.toBeNull(),
    );
    fireEvent.keyDown(window, { key: "s" });
    await waitFor(() => expect(onDecide).toHaveBeenCalledTimes(1));
    expect(onDecide.mock.calls[0][1]).toBe("signed-off");
  });

  it("filters with the findings chips", async () => {
    renderPage();
    await screen.findByText("locales/de-DE.json");
    await userEvent.click(screen.getByRole("button", { name: "With findings" }));
    expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(1);
    await userEvent.click(screen.getByRole("button", { name: "Clean" }));
    expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(2);
    await userEvent.click(screen.getByRole("button", { name: "All" }));
    expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(3);
  });

  it("narrows to the entry scope (collection, locale)", async () => {
    renderPage({ scope: { collection: "Docs", locale: "fr-FR" } });
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(1),
    );
    expect(
      document.querySelector("[data-slot='review-queue-item']")?.getAttribute("data-key"),
    ).toBe("farewell");
  });

  it("batch-approves every clean unit in the current view", async () => {
    const { onDecide } = renderPage();
    const batchBtn = await screen.findByRole("button", { name: /Approve 2 clean units/ });
    await userEvent.click(batchBtn);
    await waitFor(() => expect(onDecide).toHaveBeenCalledTimes(2));
    expect(onDecide.mock.calls.every((c) => c[1] === "approved")).toBe(true);
    // Only the unit with findings remains.
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(1),
    );
  });

  it("saves an edited target and re-checks the unit", async () => {
    const onSaveTarget = vi.fn(async () => {});
    renderPage({ onSaveTarget });
    const target = (await waitFor(() => {
      const el = document.querySelector("[data-slot='review-target']") as HTMLTextAreaElement;
      expect(el).not.toBeNull();
      expect(el.value).toBe("Hallo");
      return el;
    })) as HTMLTextAreaElement;
    fireEvent.change(target, { target: { value: "Hallo {name}" } });
    const save = await screen.findByRole("button", { name: /Save & re-check/ });
    await userEvent.click(save);
    await waitFor(() => expect(onSaveTarget).toHaveBeenCalledTimes(1));
    expect(onSaveTarget.mock.calls[0][1]).toBe("Hallo {name}");
  });

  it("shows the empty state when the queue is empty", async () => {
    render(
      <ErrorProvider>
        <ReviewPage tabID="t1" items={[]} />
      </ErrorProvider>,
    );
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-empty']")).not.toBeNull(),
    );
  });
});
