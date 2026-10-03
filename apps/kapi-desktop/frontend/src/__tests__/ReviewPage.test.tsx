import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "./testUtils";
import userEvent from "@testing-library/user-event";

import { ReviewPage } from "../components/ReviewPage";
import { ErrorProvider } from "../components/ErrorBanner";
import type { PreReviewResult, ReviewContext, ReviewItem, ReviewUnitDetail } from "../types/api";
import { decisionsSent, queueChanges, type MemoryChanges } from "../stories/memoryChanges";

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
            fails: true,
            message: "placeholder {name} missing from target",
          },
        ]
      : [],
  };
}

function renderPage(
  overrides: Partial<Parameters<typeof ReviewPage>[0]> = {},
  changes: MemoryChanges = queueChanges(overrides.items ?? ITEMS),
) {
  const loadUnit = vi.fn(async (item: ReviewItem) => unitFor(item));
  const utils = render(
    <ErrorProvider>
      <ReviewPage tabID="t1" items={ITEMS} loadUnit={loadUnit} changes={changes} {...overrides} />
    </ErrorProvider>,
  );
  return { ...utils, loadUnit, changes };
}

/** The editor the target pane draws, once the unit has been read. */
async function targetEditor(): Promise<HTMLElement> {
  return waitFor(() => {
    const el = document.querySelector<HTMLElement>(
      "[data-slot='review-target-editor'] [contenteditable='true']",
    );
    expect(el).not.toBeNull();
    return el!;
  });
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
    expect((await targetEditor()).textContent).toBe("Hallo");
  });

  it("approves with the keyboard and advances to the next unit", async () => {
    const { changes } = renderPage();
    await targetEditor();
    fireEvent.keyDown(window, { key: "a" });
    await waitFor(() => expect(decisionsSent(changes)).toHaveLength(1));
    const [decision] = decisionsSent(changes);
    expect(decision.outcome).toBe("establish");
    expect(decision.at.edition).toBe("de-DE");
    // The decision names the revision the page read.
    const op = changes.sets[0].ops[0];
    expect(op.op === "decide" && op.if_match).toMatch(/^r:/);
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
    const { changes } = renderPage();
    await targetEditor();
    fireEvent.keyDown(window, { key: "r" });
    await answerAsk("too literal");
    await waitFor(() => expect(decisionsSent(changes)).toHaveLength(1));
    expect(decisionsSent(changes)[0]).toMatchObject({ outcome: "reject", note: "too literal" });
  });

  // Someone saved the file between the page's read and the decision. Nothing is
  // recorded; the page shows the text as it stands and asks.
  it("asks before approving text that changed since the page read it", async () => {
    const { changes } = renderPage();
    await targetEditor();
    changes.touch("locales/de-DE.json", "greeting", "Hallo Leute");
    fireEvent.keyDown(window, { key: "a" });
    await screen.findByText("Changed since you opened it");
    expect(screen.getByText("Hallo Leute")).toBeInTheDocument();
    expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(3);

    await userEvent.click(screen.getByRole("button", { name: /Approve the text as it stands/ }));
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(2),
    );
    const sent = decisionsSent(changes);
    expect(sent).toHaveLength(2);
    const [first, second] = changes.sets.map((s) => s.ops[0]);
    expect(
      first.op === "decide" && second.op === "decide" && first.if_match !== second.if_match,
    ).toBe(true);
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
    const { changes } = renderPage();
    await targetEditor();
    fireEvent.keyDown(window, { key: "r" });
    await cancelAsk();
    await new Promise((r) => setTimeout(r, 20));
    expect(changes.sets).toHaveLength(0);
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
    const { changes } = renderPage();
    const batchBtn = await screen.findByRole("button", { name: /Approve 2 clean units/ });
    await userEvent.click(batchBtn);
    await waitFor(() => expect(decisionsSent(changes)).toHaveLength(2));
    expect(decisionsSent(changes).every((d) => d.outcome === "establish")).toBe(true);
    // Only the unit with findings remains.
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(1),
    );
  });

  it("saves an edited target with the revision it read, and re-checks the unit", async () => {
    const { changes, loadUnit } = renderPage();
    const editor = await targetEditor();
    const read = await changes.read({ doc: "locales/de-DE.json", blocks: ["greeting"] });
    await userEvent.type(editor, "Welt ");
    const typed = editor.textContent;
    expect(typed).toContain("Welt");
    const save = await screen.findByRole("button", { name: /Save & re-check/ });
    await userEvent.click(save);
    await waitFor(() => expect(changes.sets).toHaveLength(1));
    expect(changes.sets[0].ops).toEqual([
      {
        op: "set_content",
        at: { doc: "locales/en-US.json", block: "greeting", edition: "de-DE" },
        if_match: read.blocks[0].rev,
        text: typed,
      },
    ]);
    // The unit is loaded again so its checks run against the edited text.
    await waitFor(() => expect(loadUnit.mock.calls.length).toBeGreaterThan(1));
  });

  // A translation with inline codes is edited around them: each code is a chip
  // the reviewer types beside, and the text goes back with the codes it read.
  it("keeps the inline codes of a formatted translation when it is edited", async () => {
    const changes = queueChanges(ITEMS, {
      "de-DE:greeting": {
        text: 'Hallo <x id="1"/>Welt<x id="/1"/>',
        codes: { "1": { kind: "paired", type: "fmt:bold" } },
      },
    });
    renderPage({}, changes);
    const editor = await targetEditor();
    expect(editor.querySelectorAll("[data-tag-chip]")).toHaveLength(2);
    await userEvent.type(editor, "Oh, ");
    await userEvent.click(await screen.findByRole("button", { name: /Save & re-check/ }));
    await waitFor(() => expect(changes.sets).toHaveLength(1));
    const op = changes.sets[0].ops[0];
    const text = op.op === "set_content" ? (op.text ?? "") : "";
    expect(text).toContain("Oh, ");
    expect(text).toContain('<x id="1"/>Welt<x id="/1"/>');
  });

  // A plural is edited a form at a time; each form that changed is its own
  // set_content, addressed by the path the read lists for it.
  it("edits one form of a plural translation by its path", async () => {
    const changes = queueChanges(ITEMS, {
      "de-DE:greeting": {
        text: '<x id="n/"/> Artikel',
        codes: { "n/": { kind: "placeholder", type: "jsx:var", equiv: "count" } },
        structures: [
          {
            path: [0],
            kind: "plural",
            pivot: "count",
            branches: { one: '<x id="n/"/> Artikel', other: '<x id="n/"/> Artikel' },
          },
        ],
      },
    });
    renderPage({}, changes);
    const one = await waitFor(() => {
      const el = screen.getByRole("textbox", { name: "one form" }) as HTMLTextAreaElement;
      expect(el.value).toBe("{count} Artikel");
      return el;
    });
    fireEvent.change(one, { target: { value: "{count} Ding" } });
    await userEvent.click(await screen.findByRole("button", { name: /Save & re-check/ }));
    await waitFor(() => expect(changes.sets).toHaveLength(1));
    expect(changes.sets[0].ops).toEqual([
      expect.objectContaining({
        op: "set_content",
        path: [0, { plural: "one" }],
        text: '<x id="n/"/> Ding',
      }),
    ]);
  });

  // Someone saved the file between the read and the save. Nothing is written;
  // the editor shows the text as it stands and asks before applying the edit.
  it("asks before saving over a translation that changed since it was read", async () => {
    const { changes } = renderPage();
    const editor = await targetEditor();
    await userEvent.type(editor, "Welt ");
    const typed = editor.textContent;
    changes.touch("locales/de-DE.json", "greeting", "Hallo Leute");
    await userEvent.click(await screen.findByRole("button", { name: /Save & re-check/ }));
    await screen.findByText("Changed since you opened it");
    expect(screen.getByText("Hallo Leute")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /Apply my change over it/ }));
    await waitFor(() => expect(changes.sets).toHaveLength(2));
    const op = changes.sets[1].ops[0];
    expect(op.op === "set_content" && op.text).toBe(typed);
    await waitFor(() =>
      expect(screen.queryByText("Changed since you opened it")).not.toBeInTheDocument(),
    );
  });

  it("lists the recorded changes to the unit", async () => {
    const { changes } = renderPage();
    const editor = await targetEditor();
    await userEvent.type(editor, "Welt ");
    await userEvent.click(await screen.findByRole("button", { name: /Save & re-check/ }));
    await waitFor(() => expect(changes.sets).toHaveLength(1));
    const card = await waitFor(() => {
      const el = document.querySelector("[data-slot='review-changes']");
      expect(el?.textContent).toContain("1 recorded change");
      return el!;
    });
    await userEvent.click(
      within(card as HTMLElement).getByRole("button", { name: /recorded changes to this text/ }),
    );
    const entries = await waitFor(() => {
      const els = document.querySelectorAll("[data-slot='review-change']");
      expect(els).toHaveLength(1);
      return els;
    });
    expect(entries[0].textContent).toContain("in Kapi Desktop");
    expect(entries[0].querySelector("[data-slot='review-change-current']")).not.toBeNull();
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
    const { changes } = renderPage({ onAIAction });
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
    await targetEditor();
    await userEvent.click(screen.getByRole("button", { name: /^Accept$/ }));
    await waitFor(() => expect(changes.sets).toHaveLength(1));
    const op = changes.sets[0].ops[0];
    expect(op.op === "set_content" && op.text).toBe("Hallo {name}!");
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
    const { changes } = renderPage({ onAIAction });
    await userEvent.click(await screen.findByRole("button", { name: /Retranslate/ }));
    await answerAsk("more informal");
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-ai-proposal']")).not.toBeNull(),
    );
    await userEvent.click(screen.getByRole("button", { name: /Discard/ }));
    expect(document.querySelector("[data-slot='review-ai-proposal']")).toBeNull();
    expect(changes.sets).toHaveLength(0);
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
    };
    const onPreReview = vi.fn(async () => result);
    renderPage({ onPreReview });
    await userEvent.click(await screen.findByRole("button", { name: /AI pre-review/ }));
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-prereview-modal']")).not.toBeNull(),
    );
    await userEvent.click(screen.getByRole("button", { name: /Run pre-review/ }));
    await waitFor(() => expect(onPreReview).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-prereview-result']")).not.toBeNull(),
    );
    expect(document.querySelector("[data-slot='review-prereview-result']")?.textContent).toContain(
      "3 units scored",
    );
    await userEvent.click(screen.getByRole("button", { name: /Close/ }));
    expect(document.querySelector("[data-slot='review-prereview-modal']")).toBeNull();
  });

  it("pre-review passes the scope filters", async () => {
    const onPreReview = vi.fn(async () => ({ model: "claude-x", reviewed: 1 }));
    renderPage({ onPreReview, scope: { locale: "fr-FR", collection: "Docs" } });
    await userEvent.click(await screen.findByRole("button", { name: /AI pre-review/ }));
    await userEvent.click(await screen.findByRole("button", { name: /Run pre-review/ }));
    await waitFor(() => expect(onPreReview).toHaveBeenCalledTimes(1));
    expect(onPreReview.mock.calls[0][0]).toBe("fr-FR");
    expect(onPreReview.mock.calls[0][1]).toEqual({ collection: "Docs" });
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
      { term: "cart", replacement: "basket" },
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
    // Each axis is a chip: the value in words, the axis name in the label.
    const coordinates = Array.from(
      document.querySelectorAll("[data-slot='review-point-coordinate']"),
    );
    const product = coordinates.find((el) => el.getAttribute("data-axis") === "product");
    expect(product?.textContent?.trim()).toBe("kapimart");
    expect(product?.getAttribute("aria-label")).toBe("Product: kapimart");
    expect(coordinates.map((el) => el.getAttribute("data-axis"))).toContain("channel");
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

  it("says the context is unread, not empty, before a copy has read it in", async () => {
    const loadUnit = vi.fn(async (item: ReviewItem) => ({
      ...unitFor(item),
      context: { ...CONTEXT, history: { unread: true } },
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
    expect(empty.textContent).toContain("have not been read");
    expect(empty.textContent).toContain("kapi context import");
  });

  it("names the provenance card and carries the decision in force", async () => {
    renderWithContext();
    const card = await waitFor(() => {
      const el = document.querySelector("[data-slot='review-provenance']");
      expect(el).not.toBeNull();
      return el!;
    });
    expect(card.textContent).toContain("Provenance");
    expect(card.textContent).toContain("Rejected");
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

/** A source row: the author's own wording awaiting attention. */
const SOURCE_ROW: ReviewItem = {
  locale: "en-US",
  language: "en-US",
  isSource: true,
  file: "locales/en-US.json",
  relative: "locales/en-US.json",
  key: "greeting",
  collection: "App",
  sourceLocale: "en-US",
  source: "Hello {name}",
  status: "written",
  held: true,
};

const UNIFIED = [...ITEMS, SOURCE_ROW];

describe("ReviewPage language selector", () => {
  function renderUnified(overrides: Partial<Parameters<typeof ReviewPage>[0]> = {}) {
    return render(
      <ErrorProvider>
        <ReviewPage
          tabID="t1"
          items={UNIFIED}
          loadUnit={vi.fn(async (item: ReviewItem) => unitFor(item))}
          {...overrides}
        />
      </ErrorProvider>,
    );
  }

  it("offers every language in the queue with its pending count, source first", async () => {
    renderUnified();
    const trigger = await waitFor(() => {
      const el = document.querySelector<HTMLElement>("[data-slot='review-language-select']");
      expect(el).not.toBeNull();
      return el!;
    });
    // Closed, it reads "All languages" and the whole queue's size.
    expect(trigger.textContent).toContain("All languages");
    expect(trigger.textContent).toContain(String(UNIFIED.length));

    await userEvent.click(trigger);
    const options = await waitFor(() => {
      const els = document.querySelectorAll("[data-slot='review-language-option']");
      expect(els.length).toBe(3);
      return els;
    });
    // The source language leads, marked as the source rather than named a lane.
    expect(options[0].getAttribute("data-language")).toBe("en-US");
    expect(options[0].textContent).toContain("English");
    expect(options[0].textContent?.toLowerCase()).toContain("source");
    // Each entry carries what is waiting behind it: two French units, one German.
    const counts = Array.from(options).map((el) => [
      el.getAttribute("data-language"),
      el.textContent?.match(/(\d+)\s*$/)?.[1],
    ]);
    expect(counts).toEqual([
      ["en-US", "1"],
      ["de-DE", "1"],
      ["fr-FR", "2"],
    ]);
    // No lane toggle survives: one control picks the language.
    expect(document.querySelector("[data-slot='review-lane-toggle']")).toBeNull();
  });

  it("narrows the queue to the chosen target language", async () => {
    renderUnified({ scope: { locale: "fr-FR" } });
    await waitFor(() => {
      const rows = document.querySelectorAll("[data-slot='review-queue-item']");
      expect(rows).toHaveLength(2);
      return rows;
    });
    const locales = Array.from(document.querySelectorAll("[data-slot='review-queue-item']")).map(
      (el) => el.getAttribute("data-language"),
    );
    expect(new Set(locales)).toEqual(new Set(["fr-FR"]));
  });

  it("puts the source rows in front of the reviewer when the source language is chosen", async () => {
    renderUnified({ scope: { locale: "en-US" } });
    const rows = await waitFor(() => {
      const els = document.querySelectorAll("[data-slot='review-queue-item']");
      expect(els).toHaveLength(1);
      return els;
    });
    // The row lists in the one queue, marked as the source rather than moved
    // into a lane of its own.
    expect(rows[0].getAttribute("data-source")).toBe("true");
    expect(rows[0].getAttribute("data-language")).toBe("en-US");
    await waitFor(() =>
      expect(document.querySelector("[data-slot='source-unit-pane']")).not.toBeNull(),
    );
    expect(document.querySelector("[data-slot='source-unit-language']")?.textContent).toContain(
      "English",
    );
    // The chips stay on the one control set: nothing is hidden by picking a
    // language.
    expect(document.querySelector<HTMLElement>("[data-slot='review-chips']")?.hidden).toBe(false);
  });

  // The defect this replaced: "All languages 4" over a list of three rows,
  // because the source rows were counted and then filtered out of the list.
  it("lists the source rows in the All view, ahead of the target rows", async () => {
    renderUnified();
    const rows = await waitFor(() => {
      const els = document.querySelectorAll("[data-slot='review-queue-item']");
      expect(els).toHaveLength(UNIFIED.length);
      return els;
    });
    expect(rows[0].getAttribute("data-source")).toBe("true");
    expect(rows[0].getAttribute("data-language")).toBe("en-US");
    // Every row after the source rows is a translation.
    expect(
      Array.from(rows)
        .slice(1)
        .every((el) => el.getAttribute("data-source") === null),
    ).toBe(true);
    // The count on the selector is the count of rows under it.
    const trigger = document.querySelector<HTMLElement>("[data-slot='review-language-select']");
    expect(trigger?.textContent).toContain(String(rows.length));
  });

  // Source rows carry no findings enrichment, so they answer neither chip.
  it("keeps the source rows out of the findings and clean chips", async () => {
    renderUnified();
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(
        UNIFIED.length,
      ),
    );
    await userEvent.click(screen.getByRole("button", { name: "Clean" }));
    let rows = document.querySelectorAll("[data-slot='review-queue-item']");
    expect(Array.from(rows).some((el) => el.getAttribute("data-source"))).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: "With findings" }));
    rows = document.querySelectorAll("[data-slot='review-queue-item']");
    expect(Array.from(rows).some((el) => el.getAttribute("data-source"))).toBe(false);
  });

  // The batch bar counts translations. It stayed hidden whenever a source row
  // was in view, which took the whole All view with it.
  it("keeps the batch bar in the All view with source rows present", async () => {
    renderUnified();
    await screen.findByRole("button", { name: /Approve 2 clean units/ });
    expect(document.querySelector("[data-slot='review-batch']")).not.toBeNull();
  });
});

describe("ReviewPage source rows", () => {
  function renderSource(overrides: Partial<Parameters<typeof ReviewPage>[0]> = {}) {
    const changes = queueChanges(UNIFIED);
    const utils = render(
      <ErrorProvider>
        <ReviewPage
          tabID="t1"
          items={UNIFIED}
          scope={{ locale: "en-US" }}
          loadUnit={vi.fn(async (item: ReviewItem) => unitFor(item))}
          changes={changes}
          {...overrides}
        />
      </ErrorProvider>,
    );
    return { ...utils, changes };
  }

  /** The source pane's editor, once the unit has been read. */
  async function sourceEditor(): Promise<HTMLElement> {
    return waitFor(() => {
      const el = document.querySelector<HTMLElement>(
        "[data-slot='source-unit-editor'] [contenteditable='true']",
      );
      expect(el).not.toBeNull();
      return el!;
    });
  }

  it("approves the selected source unit from the one action bar", async () => {
    const { changes } = renderSource();
    await sourceEditor();
    await userEvent.click(await screen.findByRole("button", { name: /Approve source/ }));
    await waitFor(() => expect(decisionsSent(changes)).toHaveLength(1));
    expect(decisionsSent(changes)[0]).toMatchObject({
      outcome: "establish",
      at: { doc: "locales/en-US.json", block: "greeting" },
    });
    // The approved unit leaves the queue.
    await waitFor(() =>
      expect(document.querySelectorAll("[data-slot='review-queue-item']")).toHaveLength(0),
    );
  });

  it("approves a source row with the a key", async () => {
    const { changes } = renderSource();
    await sourceEditor();
    fireEvent.keyDown(window, { key: "a" });
    await waitFor(() => expect(decisionsSent(changes)).toHaveLength(1));
  });

  // There is no source reject, so the button is not drawn and the key does not
  // answer.
  it("offers no reject on a source row", async () => {
    const { changes } = renderSource();
    await waitFor(() =>
      expect(document.querySelector("[data-slot='source-unit-pane']")).not.toBeNull(),
    );
    expect(document.querySelector("[data-slot='review-reject']")).toBeNull();
    fireEvent.keyDown(window, { key: "r" });
    await new Promise((r) => setTimeout(r, 20));
    expect(document.querySelector("[data-slot='review-ask-input']")).toBeNull();
    expect(changes.sets).toHaveLength(0);
  });

  // The translations stay where they are and the loop supersedes them. Naming
  // the languages is how the reviewer knows the re-draft is coming.
  it("saves an edit and names the languages awaiting a re-draft", async () => {
    const { changes } = renderSource();
    const editor = await sourceEditor();
    await userEvent.type(editor, "Oh, ");
    const typed = editor.textContent;
    await userEvent.click(screen.getByRole("button", { name: /Save and re-draft/ }));
    await waitFor(() => expect(changes.sets).toHaveLength(1));
    const op = changes.sets[0].ops[0];
    expect(op.op === "set_content" && op.text).toBe(typed);
    // The translations the edit left on an older source, named in the reader's terms.
    const awaiting = await waitFor(() => {
      const el = document.querySelector("[data-slot='source-unit-awaiting']");
      expect(el).not.toBeNull();
      return el!;
    });
    expect(awaiting.textContent).toContain("German");
    expect(awaiting.textContent).toContain("French");
  });

  it("will not save an unchanged source", async () => {
    const { changes } = renderSource();
    await sourceEditor();
    expect(screen.queryByRole("button", { name: /Save and re-draft/ })).not.toBeInTheDocument();
    expect(changes.sets).toHaveLength(0);
  });

  // Both kinds of row render one model. Approving source wording without seeing
  // the voice it is approved against is the target defect in reverse.
  it("renders the point and the neighbourhood for the selected source unit", async () => {
    const loadSourceContext = vi.fn(async (item: ReviewItem) => ({
      point: {
        collection: "App",
        ref: "retail/web",
        default: false,
        voice: { name: "Kapimart retail" },
        term_rules: [{ term: "cart", replacement: "basket" }],
        terms_total: 1,
      },
      neighbourhood: {
        key: item.key,
        after: [{ key: "farewell", source: [{ text: "Goodbye now" }] }],
        window: 2,
      },
      history: {},
      judgement: {},
      provenance: {},
    }));
    renderSource({ loadSourceContext });
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-point-voice']")?.textContent).toBe(
        "Kapimart retail",
      ),
    );
    expect(loadSourceContext.mock.calls[0][0].key).toBe("greeting");
    expect(document.querySelector("[data-slot='review-point-term']")?.textContent).toContain(
      "basket",
    );
    expect(document.querySelector("[data-slot='review-neighbour']")?.textContent).toContain(
      "Goodbye now",
    );
  });

  // The target detail asks the backend for a translation of a (locale, file,
  // key). A source row has none, and asking would have loaded the source file
  // as though it were one.
  it("asks for no target unit when a source row is selected", async () => {
    const loadUnit = vi.fn(async (item: ReviewItem) => unitFor(item));
    renderSource({ loadUnit });
    await waitFor(() =>
      expect(document.querySelector("[data-slot='source-unit-pane']")).not.toBeNull(),
    );
    expect(loadUnit).not.toHaveBeenCalled();
    expect(document.querySelector("[data-slot='review-target']")).toBeNull();
  });
});

describe("ReviewPage tone", () => {
  it("draws bound term rules as context, whatever their severity", async () => {
    render(
      <ErrorProvider>
        <ReviewPage
          tabID="t1"
          items={ITEMS}
          loadUnit={vi.fn(async (item: ReviewItem) => ({
            ...unitFor(item),
            context: {
              ...CONTEXT,
              point: {
                ...CONTEXT.point,
                // The third rule carries no severity, which is what every rule
                // resolved from a terms store looks like.
                term_rules: [
                  { term: "cart", replacement: "basket" },
                  { term: "sign in", replacement: "log in", advisory: true },
                  { term: "checkout", replacement: "pay" },
                  { term: "Kapimart", do_not_translate: true },
                ],
              },
            },
          }))}
        />
      </ErrorProvider>,
    );
    const chips = await waitFor(() => {
      const els = document.querySelectorAll<HTMLElement>("[data-slot='review-point-term']");
      expect(els).toHaveLength(4);
      return els;
    });
    // Context is neutral: no chip borrows the colour a finding uses.
    for (const chip of chips) {
      expect(chip.className).not.toMatch(/destructive|amber|teal/);
      expect(chip.className).toContain("border-border");
    }
    // How hard a rule bites stays readable, in the data and the tooltip.
    expect(Array.from(chips).map((el) => el.getAttribute("data-severity"))).toEqual([
      "blocks",
      "warns",
      "blocks",
      "blocks",
    ]);
    expect(chips[3].getAttribute("data-dnt")).toBe("true");
  });

  it("paints a major finding as hard as a critical one, and a minor one softer", async () => {
    render(
      <ErrorProvider>
        <ReviewPage
          tabID="t1"
          items={ITEMS}
          loadUnit={vi.fn(async (item: ReviewItem) => unitFor(item))}
        />
      </ErrorProvider>,
    );
    const finding = await waitFor(() => {
      const el = document.querySelector("[data-slot='review-finding']");
      expect(el).not.toBeNull();
      return el!;
    });
    // A failing finding takes the destructive tone rather than the amber a
    // reported one reads in; the tones are the shared findingOutcomeTone.
    expect(finding.querySelector(".text-destructive")).not.toBeNull();
    expect(finding.querySelector(".text-warning")).toBeNull();
  });
});
