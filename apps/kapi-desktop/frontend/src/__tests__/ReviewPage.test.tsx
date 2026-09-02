import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "./testUtils";
import userEvent from "@testing-library/user-event";

import { ReviewPage, type ReviewDecision } from "../components/ReviewPage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { PreReviewResult, ReviewContext, ReviewItem, ReviewUnitDetail } from "../types/api";

/** A date placeholder, the kind a concatenating run walk deletes silently. */
const DATE_PH = {
  id: "1",
  type: "var",
  data: "{date}",
  equiv: "date",
};

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

/** Answer the page's one-field dialog. window.prompt does nothing in the app's
 *  webview, so the dialog is the only path the real UI takes. */
async function answerAsk(text: string) {
  const input = await waitFor(() => {
    const el = document.querySelector<HTMLTextAreaElement>("[data-slot='review-ask-input']");
    expect(el).not.toBeNull();
    return el!;
  });
  if (text !== "") await userEvent.type(input, text);
  await userEvent.click(
    document.querySelector<HTMLButtonElement>("[data-slot='review-ask-confirm']")!,
  );
}

/** Dismiss the one-field dialog without answering. */
async function cancelAsk() {
  await waitFor(() =>
    expect(document.querySelector("[data-slot='review-ask-input']")).not.toBeNull(),
  );
  await userEvent.click(screen.getByRole("button", { name: /^Cancel$/ }));
}

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

  it("rejecting asks for a note and records it", async () => {
    const { onDecide } = renderPage();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-queue-item'][data-active]")).not.toBeNull(),
    );
    fireEvent.keyDown(window, { key: "r" });
    await answerAsk("too literal");
    await waitFor(() => expect(onDecide).toHaveBeenCalledTimes(1));
    expect(onDecide.mock.calls[0][1]).toBe("rejected");
    expect(onDecide.mock.calls[0][2]).toBe("too literal");
  });

  // window.prompt returns null in the app's webview without ever showing
  // anything, and both callers read null as "cancelled". Retranslate and Reject
  // therefore did nothing at all, silently. These tests mocked window.prompt, so
  // they passed against a UI that could not work.
  it("never calls window.prompt", async () => {
    const promptSpy = vi.spyOn(window, "prompt");
    renderPage();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-queue-item'][data-active]")).not.toBeNull(),
    );

    fireEvent.keyDown(window, { key: "r" });
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ask-input']")).not.toBeNull(),
    );
    await userEvent.click(screen.getByRole("button", { name: /^Cancel$/ }));

    await userEvent.click(await screen.findByRole("button", { name: /Retranslate/ }));
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ask-input']")).not.toBeNull(),
    );
    expect(promptSpy).not.toHaveBeenCalled();
  });

  // Retranslate needs an instruction; the dialog must not let an empty one
  // through, which is the case the old code silently swallowed.
  it("will not retranslate on an empty instruction", async () => {
    const onAIAction = vi.fn(async () => ({ proposed_target: "x" }));
    renderPage({ onAIAction });
    await userEvent.click(await screen.findByRole("button", { name: /Retranslate/ }));
    const confirm = await waitFor(() => {
      const el = document.querySelector<HTMLButtonElement>("[data-slot='review-ask-confirm']");
      expect(el).not.toBeNull();
      return el!;
    });
    expect(confirm.hasAttribute("disabled")).toBe(true);
    expect(onAIAction).not.toHaveBeenCalled();
  });

  it("cancelling the reject dialog records nothing", async () => {
    const { onDecide } = renderPage();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-queue-item'][data-active]")).not.toBeNull(),
    );
    fireEvent.keyDown(window, { key: "r" });
    await cancelAsk();
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
    const onAIAction = vi.fn(async () => ({ proposed_target: "Hallo {name}!" }));
    const onSaveTarget = vi.fn(async () => {});
    renderPage({ onAIAction, onSaveTarget });
    const retranslate = await screen.findByRole("button", { name: /Retranslate/ });
    await userEvent.click(retranslate);
    await answerAsk("more informal");
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
            // The shape the Go side actually serializes: a message is a list of
            // parts, never a plain string. The first version of this test
            // invented a `content` field, so it passed while the panels it was
            // meant to prove rendered empty against real backend output.
            messages: [
              { role: "system", parts: [{ kind: "text", text: "You translate from en to de." }] },
              {
                role: "user",
                parts: [{ kind: "text", text: "Hello {name}!\nInstruction: more informal" }],
              },
            ],
            response: "Hallo {name}!",
            usage: { input_tokens: 120, output_tokens: 8 },
          },
        },
      ],
    }));
    renderPage({ onAIAction });
    await userEvent.click(await screen.findByRole("button", { name: /Retranslate/ }));
    await answerAsk("more informal");
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
    const onAIAction = vi.fn(async () => ({ proposed_target: "Hi" }));
    const onSaveTarget = vi.fn(async () => {});
    renderPage({ onAIAction, onSaveTarget });
    await userEvent.click(await screen.findByRole("button", { name: /Retranslate/ }));
    await answerAsk("more informal");
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ai-proposal']")).not.toBeNull(),
    );
    await userEvent.click(screen.getByRole("button", { name: /Discard/ }));
    expect(document.querySelector("[data-slot='review-ai-proposal']")).toBeNull();
    expect(onSaveTarget).not.toHaveBeenCalled();
  });

  it("cancelling the retranslate prompt calls nothing", async () => {
    const onAIAction = vi.fn(async () => ({ proposed_target: "Hi" }));
    renderPage({ onAIAction });
    await userEvent.click(await screen.findByRole("button", { name: /Retranslate/ }));
    await new Promise((r) => setTimeout(r, 20));
    expect(onAIAction).not.toHaveBeenCalled();
  });

  it("shows the AI pre-review score beside the checks and on queue items", async () => {
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
    // Select the annotated unit: the AI pre-review reads inside the checks
    // group, with the model that scored it.
    const annotated = Array.from(document.querySelectorAll("[data-slot='review-queue-item']")).find(
      (el) => el.textContent?.includes("ai 92"),
    ) as HTMLElement;
    await userEvent.click(annotated);
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ai-score']")).not.toBeNull(),
    );
    const score = document.querySelector("[data-slot='review-ai-score']");
    expect(score?.textContent).toContain("92");
    expect(score?.textContent).toContain("claude-x");
    expect(
      document
        .querySelector("[data-slot='review-findings']")
        ?.contains(document.querySelector("[data-slot='review-ai-prereview']")),
    ).toBe(true);
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

/** One unit's review model, as the host assembles it. */
const CONTEXT: ReviewContext = {
  point: {
    path: "content/app.json",
    profile: "retail",
    channel: "web",
    collection: "App",
    ref: "retail/web",
    default: false,
    coordinates: { product: "kapimart", channel: "web" },
    voice: {
      name: "Kapimart retail",
      source: "pack:retail",
      field: "defaults.voice",
      guide: "Write in the second person. Keep sentences under twenty words.",
    },
    term_rules: [
      { term: "cart", replacement: "basket", severity: "major" },
      { term: "Kapimart", do_not_translate: true },
    ],
    terms_total: 40,
    profiles: [{ name: "retail", valid_from: "2026-01-01", state: "active" }],
    notes: ["The terms store was read three days ago."],
  },
  neighbourhood: {
    key: "greeting",
    before: [{ key: "intro", source: [{ text: "Welcome back." }] }],
    after: [
      {
        key: "credits",
        source: [{ text: "Your credits reset on " }, { ph: DATE_PH }, { text: "." }],
        target: [{ text: "Vos crédits sont réinitialisés le " }, { ph: DATE_PH }, { text: "." }],
      },
    ],
    window: 2,
  },
  history: {
    prior: {
      source: "Hello {name}",
      target: "Salut {name}",
      context_fingerprint: "abc123",
      governed: false,
    },
    match: { score: 88, source: "Hello {name}!", target: "Bonjour {name} !" },
  },
  judgement: {
    ai_score: 74,
    ai_model: "claude-x",
    ai_findings: [{ severity: "minor", message: "The greeting is more formal than the source." }],
  },
  provenance: {
    origin: { kind: "ai", engine: "claude-x" },
    review_state: "rejected",
    by: "agent/desktop",
    at: "2026-08-30T10:00:00Z",
    note: "Too formal for this surface.",
    stale: true,
  },
};

describe("ReviewPage review model", () => {
  function renderWithContext() {
    const loadUnit = vi.fn(async (item: ReviewItem) => ({
      ...unitFor(item),
      context: CONTEXT,
    }));
    return render(
      <ErrorProvider>
        <ReviewPage tabID="t1" items={ITEMS} loadUnit={loadUnit} />
      </ErrorProvider>,
    );
  }

  it("renders the point rail: ref, collection, coordinates, voice and term rules", async () => {
    renderWithContext();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-point-ref']")?.textContent).toBe(
        "retail/web",
      ),
    );
    expect(document.querySelector("[data-slot='review-point-voice']")?.textContent).toBe(
      "Kapimart retail",
    );
    const coordinates = Array.from(
      document.querySelectorAll("[data-slot='review-point-coordinate']"),
    ).map((el) => el.textContent?.trim());
    expect(coordinates).toContain("product: kapimart");
    const terms = Array.from(document.querySelectorAll("[data-slot='review-point-term']")).map(
      (el) => el.textContent,
    );
    expect(terms[0]).toContain("cart");
    expect(terms[0]).toContain("basket");
    // A capped list says what it is part of.
    expect(document.querySelector("[data-slot='review-point-terms-total']")?.textContent).toContain(
      "40",
    );
    // The rendered guidance is prose, so it sits behind a disclosure.
    expect(document.querySelector("[data-slot='review-point-guide']")).toBeNull();
    await userEvent.click(
      document.querySelector("[data-slot='review-point-guide-toggle']") as HTMLElement,
    );
    expect(document.querySelector("[data-slot='review-point-guide']")?.textContent).toContain(
      "second person",
    );
  });

  it("renders the neighbourhood in document order, placeholders and all", async () => {
    renderWithContext();
    const rows = await waitFor(() => {
      const els = document.querySelectorAll("[data-slot='review-neighbour']");
      expect(els).toHaveLength(2);
      return els;
    });
    expect(rows[0].textContent).toContain("Welcome back.");
    // The placeholder survives the projection: a concatenating loop would have
    // rendered "Your credits reset on ." with the variable gone.
    expect(rows[1].textContent).toContain("Your credits reset on");
    expect(rows[1].textContent).toContain("date");
    // The unit sits between its neighbours, keyed in full.
    expect(document.querySelector("[data-slot='review-neighbour-unit']")?.textContent).toContain(
      "greeting",
    );
  });

  it("renders the prior version and the content memory's wording", async () => {
    renderWithContext();
    const prior = await waitFor(() => {
      const el = document.querySelector("[data-slot='review-history-prior']");
      expect(el).not.toBeNull();
      return el!;
    });
    expect(prior.textContent).toContain("Salut {name}");
    // The fingerprint no longer matches, so the prompt would have withheld it.
    expect(prior.textContent).toContain("context has moved");
    const match = document.querySelector("[data-slot='review-history-match']");
    expect(match?.textContent).toContain("88%");
    expect(match?.textContent).toContain("Bonjour {name} !");
  });

  it("says the content memory is unread, not empty, before it has been compiled", async () => {
    const loadUnit = vi.fn(async (item: ReviewItem) => ({
      ...unitFor(item),
      context: { ...CONTEXT, history: { unseeded: true } },
    }));
    render(
      <ErrorProvider>
        <ReviewPage tabID="t1" items={ITEMS} loadUnit={loadUnit} />
      </ErrorProvider>,
    );
    const empty = await waitFor(() => {
      const el = document.querySelector("[data-slot='review-history-empty']");
      expect(el).not.toBeNull();
      return el!;
    });
    // A fresh clone's store answers empty for wording the project has already
    // approved; "no close match" would be a claim about the memory's contents.
    expect(empty.textContent).not.toContain("no close match");
    expect(empty.textContent).toContain("has not been read");
    expect(empty.textContent).toContain("Bring up to date");
  });

  it("names the provenance card and carries the decision in force", async () => {
    renderWithContext();
    const card = await waitFor(() => {
      const el = document.querySelector("[data-slot='review-provenance']");
      expect(el).not.toBeNull();
      return el!;
    });
    expect(card.textContent).toContain("Provenance");
    expect(card.textContent).toContain("rejected");
    expect(card.textContent).toContain("agent/desktop");
    expect(card.textContent).toContain("Too formal for this surface.");
    expect(document.querySelector("[data-slot='review-provenance-stale']")).not.toBeNull();
    // The card that called itself Context is gone, and the memory match with it.
    expect(document.querySelector("[data-slot='review-context']")).toBeNull();
    expect(card.textContent).not.toContain("88%");
  });

  it("opens the unit in the document view and comes back to the same unit", async () => {
    renderWithContext();
    const open = await waitFor(() => {
      const el = document.querySelector("[data-slot='review-open-document']") as HTMLButtonElement;
      expect(el).not.toBeNull();
      return el;
    });
    const before = document.querySelector("[data-slot='review-queue-item'][data-active]");
    expect(before?.getAttribute("data-key")).toBe("greeting");
    await userEvent.click(open);
    const focus = await waitFor(() => {
      const el = document.querySelector("[data-slot='file-preview-focus']");
      expect(el).not.toBeNull();
      return el!;
    });
    expect(focus.textContent).toContain("greeting");
    // No decision action is reachable from inside the document view: pressing
    // the approve key while it is open decides nothing.
    fireEvent.keyDown(window, { key: "a" });
    expect(document.querySelector("[data-slot='file-preview-focus']")).not.toBeNull();
    await userEvent.click(document.querySelector("[data-slot='file-preview-back']") as HTMLElement);
    await waitFor(() =>
      expect(document.querySelector("[data-slot='file-preview-focus']")).toBeNull(),
    );
    expect(
      document
        .querySelector("[data-slot='review-queue-item'][data-active]")
        ?.getAttribute("data-key"),
    ).toBe("greeting");
  });
});
