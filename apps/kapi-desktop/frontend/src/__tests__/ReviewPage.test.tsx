import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "./testUtils";
import userEvent from "@testing-library/user-event";

import { ReviewPage, type ReviewDecision } from "../components/ReviewPage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { PreReviewResult, ReviewItem, ReviewUnitDetail } from "../types/api";

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

  it("renders the queue row's source preview in the item's own source locale", async () => {
    const rtlItems: ReviewItem[] = [
      {
        locale: "de-DE",
        file: "locales/de-DE.json",
        key: "greeting",
        collection: "App",
        sourceLocale: "ar-EG",
        source: "مرحبا",
        target: "Hallo",
        hasFindings: false,
      },
    ];
    renderPage({ items: rtlItems, loadUnit: vi.fn(async (item) => unitFor(item)) });
    await screen.findAllByText("مرحبا");
    const row = document.querySelector("[data-slot='review-queue-item']");
    const preview = row?.querySelector("[data-slot='tooltip-trigger']");
    expect(preview).toHaveTextContent("مرحبا");
    expect(preview).toHaveAttribute("dir", "rtl");
    expect(preview).toHaveAttribute("lang", "ar-EG");
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

  it("runs Explain and renders the explanation text (read-only)", async () => {
    const onAIAction = vi.fn(async () => ({ explanation: "Score: 87/100\n• [minor] literal" }));
    renderPage({ onAIAction });
    const explain = await screen.findByRole("button", { name: /Explain/ });
    await userEvent.click(explain);
    await waitFor(() => expect(onAIAction).toHaveBeenCalledTimes(1));
    expect(onAIAction.mock.calls[0][1]).toBe("explain");
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ai-explanation']")).not.toBeNull(),
    );
    expect(document.querySelector("[data-slot='review-ai-explanation']")?.textContent).toContain(
      "Score: 87/100",
    );
    expect(document.querySelector("[data-slot='review-ai-proposal']")).toBeNull();
  });

  it("retranslate prompts for an instruction, shows the diff, and Accept saves", async () => {
    vi.spyOn(window, "prompt").mockReturnValue("more informal");
    const onAIAction = vi.fn(async () => ({ proposed_target: "Hallo {name}!" }));
    const onSaveTarget = vi.fn(async () => {});
    renderPage({ onAIAction, onSaveTarget });
    const retranslate = await screen.findByRole("button", { name: /Retranslate/ });
    await userEvent.click(retranslate);
    await waitFor(() => expect(onAIAction).toHaveBeenCalledTimes(1));
    expect(onAIAction.mock.calls[0][1]).toBe("retranslate");
    expect(onAIAction.mock.calls[0][2]).toBe("more informal");

    // The diff shows current vs proposed; Accept routes through the save path.
    const proposal = await waitFor(() => {
      const el = document.querySelector("[data-slot='review-ai-proposal']");
      expect(el).not.toBeNull();
      return el!;
    });
    expect(proposal.textContent).toContain("Hallo");
    expect(proposal.textContent).toContain("Hallo {name}!");
    await userEvent.click(screen.getByRole("button", { name: /^Accept$/ }));
    await waitFor(() => expect(onSaveTarget).toHaveBeenCalledTimes(1));
    expect(onSaveTarget.mock.calls[0][1]).toBe("Hallo {name}!");
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ai-proposal']")).toBeNull(),
    );
  });

  // A reviewer accepting a proposal is accepting an answer. The question has to
  // be reachable, or the only basis for the decision is that the model said so.
  it("discloses what was sent to the model behind an AI proposal", async () => {
    vi.spyOn(window, "prompt").mockReturnValue("more informal");
    const onAIAction = vi.fn(async () => ({
      proposed_target: "Hallo {name}!",
      exchanges: [
        {
          id: 1,
          at: "2026-09-01T18:00:00Z",
          scope: { surface: "review", action: "retranslate", locale: "de", key: "greeting" },
          provider: "anthropic",
          model: "claude-opus-5",
          prompt: "translate.single",
          exchange: {
            provider: "anthropic",
            model: "claude-opus-5",
            messages: [
              { role: "system", content: "You translate from en to de." },
              { role: "user", content: "Hello {name}!\nInstruction: more informal" },
            ],
            response: "Hallo {name}!",
            usage: { input_tokens: 120, output_tokens: 8 },
          },
        },
      ],
    }));
    renderPage({ onAIAction });
    await userEvent.click(await screen.findByRole("button", { name: /Retranslate/ }));
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ai-proposal']")).not.toBeNull(),
    );

    // Collapsed by default: the reviewer wants the proposal first.
    const disclosure = document.querySelector("[data-slot='ai-exchange-disclosure']");
    expect(disclosure).not.toBeNull();
    expect(disclosure!.textContent).not.toContain("You translate from en to de.");

    await userEvent.click(screen.getByRole("button", { name: /What was sent to the model/ }));
    await waitFor(() =>
      expect(document.querySelector("[data-slot='ai-exchange-disclosure']")!.textContent).toContain(
        "You translate from en to de.",
      ),
    );
    const text = document.querySelector("[data-slot='ai-exchange-disclosure']")!.textContent!;
    expect(text).toContain("Instruction: more informal");
    expect(text).toContain("claude-opus-5");
    expect(text).toContain("Hallo {name}!");
  });

  it("discarding an AI proposal writes nothing", async () => {
    vi.spyOn(window, "prompt").mockReturnValue("shorter");
    const onAIAction = vi.fn(async () => ({ proposed_target: "Hi" }));
    const onSaveTarget = vi.fn(async () => {});
    renderPage({ onAIAction, onSaveTarget });
    await userEvent.click(await screen.findByRole("button", { name: /Retranslate/ }));
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ai-proposal']")).not.toBeNull(),
    );
    await userEvent.click(screen.getByRole("button", { name: /Discard/ }));
    expect(document.querySelector("[data-slot='review-ai-proposal']")).toBeNull();
    expect(onSaveTarget).not.toHaveBeenCalled();
  });

  it("cancelling the retranslate prompt calls nothing", async () => {
    vi.spyOn(window, "prompt").mockReturnValue(null);
    const onAIAction = vi.fn(async () => ({ proposed_target: "Hi" }));
    renderPage({ onAIAction });
    await userEvent.click(await screen.findByRole("button", { name: /Retranslate/ }));
    await new Promise((r) => setTimeout(r, 20));
    expect(onAIAction).not.toHaveBeenCalled();
  });

  it("shows the AI review score in CONTEXT and on queue items", async () => {
    const items: ReviewItem[] = ITEMS.map((it) =>
      it.key === "greeting" && it.locale === "fr-FR"
        ? { ...it, aiScore: 92, aiModel: "claude-x" }
        : it,
    );
    const loadUnit = vi.fn(async (item: ReviewItem) => ({
      ...unitFor(item),
      ai_review_score: item.aiScore,
      ai_review_model: item.aiModel,
    }));
    render(
      <ErrorProvider>
        <ReviewPage tabID="t1" items={items} loadUnit={loadUnit} />
      </ErrorProvider>,
    );
    await screen.findByText("ai 92");
    // Select the annotated unit → CONTEXT shows "AI review: 92 (claude-x)".
    const annotated = Array.from(document.querySelectorAll("[data-slot='review-queue-item']")).find(
      (el) => el.textContent?.includes("ai 92"),
    ) as HTMLElement;
    await userEvent.click(annotated);
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ai-score']")).not.toBeNull(),
    );
    expect(document.querySelector("[data-slot='review-ai-score']")?.textContent).toContain(
      "92 (claude-x)",
    );
  });

  it("opens the pre-review modal and runs annotate-only by default", async () => {
    const result: PreReviewResult = {
      model: "claude-x",
      reviewed: 3,
      auto_approved: 0,
      remaining: 3,
    };
    const onPreReview = vi.fn(async () => result);
    renderPage({ onPreReview });
    await userEvent.click(await screen.findByRole("button", { name: /AI pre-review/ }));
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-prereview-modal']")).not.toBeNull(),
    );
    await userEvent.click(screen.getByRole("button", { name: /Run pre-review/ }));
    await waitFor(() => expect(onPreReview).toHaveBeenCalledTimes(1));
    // Annotate-only is the default policy.
    expect(onPreReview.mock.calls[0][2]).toEqual({ autoApprove: false, minScore: 90 });
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-prereview-result']")).not.toBeNull(),
    );
    expect(document.querySelector("[data-slot='review-prereview-result']")?.textContent).toContain(
      "0 auto-approved",
    );
    await userEvent.click(screen.getByRole("button", { name: /Close/ }));
    expect(document.querySelector("[data-slot='review-prereview-modal']")).toBeNull();
  });

  it("pre-review passes the auto-approve policy and scope filters", async () => {
    const onPreReview = vi.fn(async () => ({
      model: "claude-x",
      reviewed: 1,
      auto_approved: 1,
      remaining: 0,
    }));
    renderPage({ onPreReview, scope: { locale: "fr-FR", collection: "Docs" } });
    await userEvent.click(await screen.findByRole("button", { name: /AI pre-review/ }));
    const auto = await waitFor(() => {
      const el = document.querySelector("[data-slot='review-prereview-auto']") as HTMLInputElement;
      expect(el).not.toBeNull();
      return el;
    });
    await userEvent.click(auto);
    await userEvent.click(screen.getByRole("button", { name: /Run pre-review/ }));
    await waitFor(() => expect(onPreReview).toHaveBeenCalledTimes(1));
    expect(onPreReview.mock.calls[0][0]).toBe("fr-FR");
    expect(onPreReview.mock.calls[0][1]).toEqual({ collection: "Docs" });
    expect(onPreReview.mock.calls[0][2]).toEqual({ autoApprove: true, minScore: 90 });
  });
});
