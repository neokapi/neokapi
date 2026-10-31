// @vitest-environment jsdom
import { describe, it, expect, vi } from "vite-plus/test";
import { render, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { DecideView, answerArgs, decisionForRow, type DecideViewProps } from "../DecideView";
import type { ScreenTarget } from "../port";
import {
  DECIDE_IDS,
  DECIDE_VIEW,
  DISAGREEMENT,
  EMPTY_DECIDE_VIEW,
  NAME_DECISION,
  NAME_VIEW,
  WORD_DECISION,
  WORD_VIEW,
} from "../stories/fixtures";
import type { RulesDecideView } from "../types";

function renderDecide(view: RulesDecideView = DECIDE_VIEW, extra: Partial<DecideViewProps> = {}) {
  const onAnswer = vi.fn().mockResolvedValue(undefined);
  const navigate = vi.fn<(target: ScreenTarget) => void>();
  render(<DecideView view={view} onAnswer={onAnswer} navigate={navigate} {...extra} />);
  return { onAnswer, navigate };
}

const slot = (name: string) => document.querySelector<HTMLElement>(`[data-slot='${name}']`);
const all = (name: string) =>
  Array.from(document.querySelectorAll<HTMLElement>(`[data-slot='${name}']`));
const selectedId = () =>
  document
    .querySelector<HTMLElement>("[data-slot='decide-item'][aria-selected='true']")
    ?.getAttribute("data-id");
const key = (k: string) => fireEvent.keyDown(window, { key: k });

describe("Decide's question", () => {
  it("names the project and counts what waits and what is kept", () => {
    renderDecide();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("KapiMart · Decide");
    expect(slot("decide-counts")!.textContent).toBe("4 to decide · 14 kept");
  });

  it("lists one row per suggestion with its kind, its wording and how much it covers", () => {
    renderDecide();
    const rows = all("decide-item");
    expect(rows.map((r) => r.getAttribute("data-kind"))).toEqual([
      "disagreement",
      "name",
      "word",
      "note",
    ]);
    expect(rows[1].textContent).toContain("Quickcast");
    expect(rows[1].textContent).toContain("3 forms");
    expect(rows[2].textContent).toContain("business");
    expect(rows[2].textContent).toContain("1 word");
    expect(rows[0].textContent).toContain("6 of 7 blocks");
  });

  it("asks which form a name takes, the options with their blocks and files, and Other…", () => {
    renderDecide(NAME_VIEW);
    expect(slot("decide-question-text")!.textContent).toBe("Which one do your files use?");
    const options = all("decide-option");
    expect(options.map((o) => o.textContent)).toEqual([
      "1Quickcast12 blocks · 5 files",
      "2Quick cast3 blocks · 2 files",
      "3QuickCast1 block · 1 file",
    ]);
    expect(options[0].getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("textbox", { name: "Form to write" })).toBeTruthy();
  });

  it("states what keeping the picked option does before the actions", async () => {
    renderDecide(NAME_VIEW);
    const consequence = slot("decide-consequence")!;
    expect(consequence.textContent).toBe(NAME_DECISION.options[0].consequence);
    expect(
      consequence.compareDocumentPosition(slot("decide-actions")!) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    await userEvent.click(all("decide-option")[1]);
    expect(slot("decide-consequence")!.textContent).toBe(NAME_DECISION.options[1].consequence);
  });

  it("states a yes-or-no rule's consequence, and shows the quote, the file and who filed it", () => {
    renderDecide(WORD_VIEW);
    expect(slot("decide-question-text")!.textContent).toBe("Keep this as a rule?");
    expect(all("decide-option")).toHaveLength(0);
    expect(slot("decide-consequence")!.textContent).toBe(WORD_DECISION.consequence);
    expect(slot("decide-quote")!.textContent).toBe("Our business plan.");
    expect(slot("decide-open-file")!.textContent).toBe("about.html");
    expect(slot("decide-open-file")!.getAttribute("title")).toBe("site/about.html");
    expect(slot("decide-noticed")!.textContent).toContain("claude-code");
  });

  it("gives each side of a disagreement its own consequence line", () => {
    renderDecide({ ...DECIDE_VIEW, decisions: [DISAGREEMENT] });
    expect(slot("decide-reason")!.textContent).toBe(DISAGREEMENT.reason);
    expect(all("decide-side-consequence").map((l) => l.textContent)).toEqual(
      DISAGREEMENT.options.map((o) => o.consequence),
    );
    expect(slot("decide-consequence")).toBeNull();
  });

  it("says nothing waits, with the rules count and the way to the Map", async () => {
    const { navigate } = renderDecide(EMPTY_DECIDE_VIEW);
    expect(slot("decide-empty")!.textContent).toContain("Nothing to decide.");
    expect(slot("decide-empty")!.textContent).toContain("14 rules kept");
    expect(slot("decide-counts")).toBeNull();
    await userEvent.click(slot("decide-open-map")!);
    expect(navigate).toHaveBeenCalledWith({ kind: "map" });
  });
});

describe("Decide's answers", () => {
  it("records Keep, Not a rule and Later through onAnswer, and selects the next question", async () => {
    const { onAnswer } = renderDecide();
    expect(selectedId()).toBe(DECIDE_IDS.store);

    await userEvent.click(slot("decide-keep")!);
    expect(onAnswer).toHaveBeenLastCalledWith(DISAGREEMENT, "keep", { option: 0 });
    expect(selectedId()).toBe(DECIDE_IDS.quickcast);

    await userEvent.click(slot("decide-drop")!);
    expect(onAnswer).toHaveBeenLastCalledWith(NAME_DECISION, "drop", { option: 0 });
    expect(selectedId()).toBe(DECIDE_IDS.business);

    await userEvent.click(slot("decide-later")!);
    expect(onAnswer).toHaveBeenLastCalledWith(WORD_DECISION, "later", { option: 0 });
    expect(all("decide-item")).toHaveLength(1);
    expect(slot("decide-counts")!.textContent).toBe("1 to decide · 14 kept");
  });

  it("answers with k, n and l, and picks an option by its number", () => {
    const { onAnswer } = renderDecide(NAME_VIEW);
    key("2");
    expect(all("decide-option")[1].getAttribute("aria-checked")).toBe("true");
    key("k");
    expect(onAnswer).toHaveBeenLastCalledWith(NAME_DECISION, "keep", { option: 1 });
    expect(slot("decide-empty")).not.toBeNull();
  });

  it("takes n for Not a rule and l for Later", () => {
    const { onAnswer } = renderDecide();
    key("n");
    expect(onAnswer).toHaveBeenLastCalledWith(DISAGREEMENT, "drop", { option: 0 });
    key("l");
    expect(onAnswer).toHaveBeenLastCalledWith(NAME_DECISION, "later", { option: 0 });
  });

  it("keeps a form typed for Other…, and asks what keeping it does", async () => {
    const onOtherForm = vi.fn();
    const { onAnswer } = renderDecide(NAME_VIEW, {
      onOtherForm,
      otherConsequence: (_d, form) => `If you keep "${form}": it is counted.`,
    });
    key("4");
    const field = screen.getByRole<HTMLInputElement>("textbox", { name: "Form to write" });
    expect(document.activeElement).toBe(field);
    await userEvent.type(field, "Quick-cast");
    expect(onOtherForm).toHaveBeenLastCalledWith(NAME_DECISION, "Quick-cast");
    expect(slot("decide-consequence")!.textContent).toBe(
      'If you keep "Quick-cast": it is counted.',
    );
    // A letter typed in the field is a letter, not an answer.
    expect(onAnswer).not.toHaveBeenCalled();
    await userEvent.type(field, "{Enter}");
    expect(onAnswer).toHaveBeenLastCalledWith(NAME_DECISION, "keep", { other: "Quick-cast" });
  });

  it("leaves a question for later until the page opens again, though every read still holds it", () => {
    const onAnswer = vi.fn().mockResolvedValue(undefined);
    const { rerender } = render(
      <DecideView view={WORD_VIEW} onAnswer={onAnswer} navigate={vi.fn()} />,
    );
    key("l");
    expect(onAnswer).toHaveBeenLastCalledWith(WORD_DECISION, "later", { option: 0 });
    // The view is read again after the answer, and later recorded nothing.
    rerender(
      <DecideView
        view={{ ...WORD_VIEW, decisions: [...WORD_VIEW.decisions] }}
        onAnswer={onAnswer}
        navigate={vi.fn()}
      />,
    );
    expect(slot("decide-empty")!.textContent).toContain("14 rules kept · 1 left for later");
  });

  it("puts a question back when its answer is not recorded", async () => {
    const onAnswer = vi.fn().mockRejectedValue(new Error("the store is read-only"));
    renderDecide(WORD_VIEW, { onAnswer });
    key("k");
    await waitFor(() => expect(screen.getByText("The answer was not recorded")).toBeTruthy());
    expect(selectedId()).toBe(DECIDE_IDS.business);
    // The reason is drawn in the question's own pane.
    expect(slot("decide-refused")!.textContent).toContain("the store is read-only");
  });
});

describe("what an answer sends to DecideRule", () => {
  it("names the suggestion, the form a kept name writes, or the side a keep chooses", () => {
    expect(answerArgs(NAME_DECISION, "keep", { option: 1 })).toEqual({
      id: DECIDE_IDS.quickcast,
      form: "Quick cast",
    });
    expect(answerArgs(NAME_DECISION, "keep", { other: " Quick-cast " })).toEqual({
      id: DECIDE_IDS.quickcast,
      form: "Quick-cast",
    });
    expect(answerArgs(NAME_DECISION, "drop", { option: 1 })).toEqual({
      id: DECIDE_IDS.quickcast,
      form: "",
    });
    expect(answerArgs(WORD_DECISION, "keep", { option: 0 })).toEqual({
      id: DECIDE_IDS.business,
      form: "",
    });
    expect(answerArgs(DISAGREEMENT, "keep", { option: 1 })).toEqual({
      id: DECIDE_IDS.market,
      form: "",
    });
    expect(answerArgs(DISAGREEMENT, "later", { option: 1 })).toEqual({
      id: DECIDE_IDS.store,
      form: "",
    });
  });
});

describe("the Map's row", () => {
  it("opens the question it names: a suggestion's own id, or one side's", () => {
    const decisions = DECIDE_VIEW.decisions;
    expect(decisionForRow(decisions, `suggested:${DECIDE_IDS.business}`)).toBe(WORD_DECISION);
    expect(decisionForRow(decisions, `conflict:${DECIDE_IDS.market}`)).toBe(DISAGREEMENT);
    expect(decisionForRow(decisions, "suggested:nothing")).toBeUndefined();
    expect(decisionForRow(decisions, undefined)).toBeUndefined();
  });

  it("selects that question when Decide opens", () => {
    renderDecide(DECIDE_VIEW, { defaultSelected: `suggested:${DECIDE_IDS.business}` });
    expect(selectedId()).toBe(DECIDE_IDS.business);
    expect(slot("decide-question-text")!.textContent).toBe("Keep this as a rule?");
  });
});

describe("Decide without answers", () => {
  it("draws the question and its evidence and no Keep, Not a rule or Later", () => {
    renderDecide(NAME_VIEW, { onAnswer: undefined });
    expect(slot("decide-question-text")!.textContent).toBe("Which one do your files use?");
    expect(all("decide-option")).toHaveLength(3);
    expect(slot("decide-actions")).toBeNull();
    expect(screen.queryByRole("button", { name: /Keep/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Not a rule/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Later/ })).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Form to write" })).toBeNull();
  });

  it("takes no answer from the keyboard", () => {
    renderDecide(DECIDE_VIEW, { onAnswer: undefined });
    key("k");
    key("n");
    expect(selectedId()).toBe(DECIDE_IDS.store);
    expect(all("decide-item")).toHaveLength(4);
  });
});

describe("where Decide leads", () => {
  it("opens the file a suggestion was seen in, through the host", async () => {
    const { navigate } = renderDecide(WORD_VIEW);
    await userEvent.click(slot("decide-open-file")!);
    expect(navigate).toHaveBeenCalledWith({ kind: "reader", path: "site/about.html" });
  });

  it("shows the file without a link, and no way to the Map, when the host opens neither", () => {
    renderDecide(WORD_VIEW, { canNavigate: () => false });
    expect(slot("decide-open-file")).toBeNull();
    expect(slot("decide-evidence")!.textContent).toContain("about.html");
  });

  it("shows a person with no name by the name the host gives", () => {
    const view: RulesDecideView = {
      ...WORD_VIEW,
      decisions: [{ ...WORD_DECISION, noticed_by: { kind: "person" } }],
    };
    renderDecide(view, { nameOf: () => "Ada Lovelace" });
    expect(slot("decide-noticed")!.textContent).toContain("Ada Lovelace");
  });

  it("shows a person with no name as you when the host gives none", () => {
    const view: RulesDecideView = {
      ...WORD_VIEW,
      decisions: [{ ...WORD_DECISION, noticed_by: { kind: "person" } }],
    };
    renderDecide(view);
    expect(slot("decide-noticed")!.textContent).toContain("you");
  });
});
