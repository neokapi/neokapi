import { describe, it, expect, vi } from "vite-plus/test";
import { render, fireEvent, screen, within } from "./testUtils";
import userEvent from "@testing-library/user-event";

import { ContextDigestView } from "../components/ContextDigest";
import { defaultRowKey, digestRows, nextRowKey, scopeParts } from "../lib/digestRows";
import {
  CONTEXT_DIGEST,
  EMPTY_DIGEST,
  LAST_LOOKED,
  QUIET_DIGEST,
  SAMPLE_DIGEST,
  SAMPLE_IDS,
  manySuggestions,
  studioRule,
} from "../stories/fixtures/contextDigest";
import type { ContextDigest } from "../types/api";

function renderDigest(digest: ContextDigest = SAMPLE_DIGEST, extra: Record<string, unknown> = {}) {
  const handlers = {
    onKeep: vi.fn(),
    onKeepGroup: vi.fn(),
    onDrop: vi.fn(),
    onChoose: vi.fn(),
    onWiden: vi.fn(),
    onOpenFile: vi.fn(),
  };
  render(<ContextDigestView digest={digest} since={LAST_LOOKED} {...handlers} {...extra} />);
  return handlers;
}

const slot = (name: string) => document.querySelector<HTMLElement>(`[data-slot='${name}']`);
const all = (name: string) =>
  Array.from(document.querySelectorAll<HTMLElement>(`[data-slot='${name}']`));
const selectedRow = () =>
  document.querySelector<HTMLElement>("[data-slot='digest-item'][aria-selected='true']");
const row = (id: string) =>
  document.querySelector<HTMLElement>(`[data-slot='digest-item'][data-id='${id}']`)!;
const toolbar = () => slot("list-detail-toolbar")!;
const key = (k: string) => fireEvent.keyDown(window, { key: k });

describe("Decide as a list", () => {
  it("groups the rows as Needs you, Suggested, Established, Earlier and Drift, with counts", () => {
    renderDigest(CONTEXT_DIGEST);
    const groups = all("list-detail-group").map((g) => g.textContent?.trim());
    expect(groups).toEqual(["Needs you1", "Suggested4", "Established1", "Earlier1", "Drift1"]);
    const order = [
      "digest-conflicts",
      "digest-suggested",
      "digest-established",
      "digest-earlier",
      "digest-drift",
    ].map((name) => slot(name)!);
    for (let i = 1; i < order.length; i++) {
      expect(
        order[i - 1].compareDocumentPosition(order[i]) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    }
    expect(slot("digest-numbers")).toBeNull();
  });

  it("puts one row per disagreement, naming both forms and both assistants", () => {
    renderDigest();
    const rows = within(slot("digest-conflicts")!).getAllByRole("option");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("log in or sign in");
    expect(rows[0].querySelector("[data-slot='kind-icon']")).toHaveAttribute(
      "data-kind",
      "conflict",
    );
    expect(
      Array.from(rows[0].querySelectorAll("[data-slot='actor-mark']")).map((m) =>
        m.getAttribute("data-actor"),
      ),
    ).toEqual(["codex", "claude-code"]);
  });

  it("lists every suggestion theme by theme and collection, on one line with who noticed it", () => {
    renderDigest();
    const ids = within(slot("digest-suggested")!)
      .getAllByRole("option")
      .map((r) => r.getAttribute("data-id"));
    expect(ids).toEqual([
      SAMPLE_IDS.payPal,
      SAMPLE_IDS.checkout,
      SAMPLE_IDS.kapiMart,
      SAMPLE_IDS.cart,
      SAMPLE_IDS.shopper,
    ]);
    const kapiMart = row(SAMPLE_IDS.kapiMart);
    expect(kapiMart).toHaveTextContent("KapiMart, not Kapi Mart");
    expect(kapiMart.querySelector("s")).toHaveTextContent("Kapi Mart");
    expect(kapiMart.querySelector("[data-slot='kind-icon']")).toHaveAttribute("data-kind", "name");
    expect(kapiMart.querySelector("[data-slot='actor-mark']")).toHaveAttribute(
      "data-actor",
      "claude-code",
    );
    expect(row(SAMPLE_IDS.cart).querySelector("[data-slot='kind-icon']")).toHaveAttribute(
      "data-kind",
      "word",
    );
    expect(row(SAMPLE_IDS.shopper).querySelector("[data-slot='kind-icon']")).toHaveAttribute(
      "data-kind",
      "note",
    );
  });

  it("says how much is new since the person last looked", () => {
    renderDigest();
    expect(slot("digest-new")?.textContent).toMatch(/^7 new since /);
  });

  it("says nothing is new in one line, and folds the rules from before under Earlier", async () => {
    renderDigest(QUIET_DIGEST);
    expect(slot("digest-quiet")?.textContent).toMatch(/^Nothing new since /);
    const earlier = slot("digest-earlier")!;
    expect(within(earlier).queryAllByRole("option")).toHaveLength(0);
    await userEvent.click(within(earlier).getByRole("button", { name: "Earlier" }));
    expect(within(earlier).getAllByRole("option")).toHaveLength(1);
    expect(within(earlier).getByRole("option")).toHaveTextContent("workspace, not project folder");
    expect(within(earlier).getByRole("option")).toHaveTextContent("kept by you");
  });

  it("splits a long list of suggestions under a sticky header per theme", () => {
    renderDigest(manySuggestions(40));
    const themes = all("digest-theme").map((h) => h.textContent);
    expect(themes).toEqual([
      "Names and spellings10",
      "Words to avoid10",
      "How the project writes10",
      "Wording in other languages10",
    ]);
    expect(all("digest-theme")[0].className).toContain("sticky");
  });

  it("keeps a short list free of theme headers", () => {
    renderDigest();
    expect(all("digest-theme")).toHaveLength(0);
  });
});

describe("the selection", () => {
  it("opens on the row that needs a person", () => {
    renderDigest();
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.logIn);
    expect(toolbar()).toHaveTextContent("log in or sign in?");
  });

  it("opens on the first suggestion when nothing needs a person", () => {
    renderDigest({ ...SAMPLE_DIGEST, conflicts: [] });
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.payPal);
  });

  it("says what the list holds when nothing waits for a decision", () => {
    renderDigest({ ...CONTEXT_DIGEST, conflicts: [], suggested: [] });
    expect(selectedRow()).toBeNull();
    expect(slot("list-detail-empty")).toHaveTextContent("Nothing needs you");
  });

  it("moves with j and k and the arrows, from anywhere on the page", () => {
    renderDigest();
    key("j");
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.payPal);
    key("ArrowDown");
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.checkout);
    key("k");
    key("ArrowUp");
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.logIn);
  });

  it("follows a click", async () => {
    renderDigest();
    await userEvent.click(row(SAMPLE_IDS.cart));
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.cart);
    expect(toolbar()).toHaveTextContent("Write cart, not basket");
  });

  it("leaves the keys alone while they are off", () => {
    const h = renderDigest(SAMPLE_DIGEST, { keyboard: false });
    key("j");
    key("a");
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.logIn);
    expect(h.onChoose).not.toHaveBeenCalled();
  });
});

describe("a suggestion", () => {
  const kapiMart = `suggested:${SAMPLE_IDS.kapiMart}`;

  it("shows its evidence and the facts about it", () => {
    renderDigest(SAMPLE_DIGEST, { defaultSelected: kapiMart });
    const pane = slot("list-detail-pane")!;
    expect(pane).toHaveTextContent("The product name is written as one word.");
    expect(pane.querySelector("blockquote")).toHaveTextContent(
      "Welcome to the KapiMart Developer Platform!",
    );
    expect(slot("open-file")).toHaveTextContent("web/en/getting-started.md");
    const facts = Array.from(slot("digest-facts")!.querySelectorAll("dt")).map(
      (d) => d.textContent,
    );
    expect(facts).toEqual(["Noticed by", "When", "Theme", "Collection", "Scope"]);
    expect(slot("digest-facts")).toHaveTextContent("claude-code");
    expect(slot("digest-facts")).toHaveTextContent("kapimart-sample-claude-code");
    expect(slot("digest-facts")).toHaveTextContent("Names and spellings");
    expect(slot("digest-facts")).toHaveTextContent("Website");
  });

  it("draws its scope as the level and a chip for each axis of its point", () => {
    renderDigest(SAMPLE_DIGEST, { defaultSelected: kapiMart });
    const scope = slot("digest-scope")!;
    expect(slot("digest-scope-level")).toHaveTextContent("Project");
    const chips = Array.from(scope.querySelectorAll("[data-slot='coordinate-chip']"));
    expect(chips.map((c) => [c.getAttribute("data-axis"), c.textContent])).toEqual([
      ["brand", "kapimart"],
      ["channel", "web"],
      ["mode", "how-to"],
      ["product", "kapimart"],
    ]);
    expect(scope).not.toHaveTextContent("brand=kapimart");
  });

  it("keeps with the button and with a, and moves on to the next suggestion", () => {
    const h = renderDigest(SAMPLE_DIGEST, { defaultSelected: kapiMart });
    fireEvent.click(within(toolbar()).getByRole("button", { name: "Keep" }));
    expect(h.onKeep).toHaveBeenCalledWith(
      expect.objectContaining({ id: SAMPLE_IDS.kapiMart }),
      undefined,
    );
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.cart);
    expect(row(SAMPLE_IDS.kapiMart)).toHaveAttribute("data-decided");

    key("a");
    expect(h.onKeep).toHaveBeenLastCalledWith(
      expect.objectContaining({ id: SAMPLE_IDS.cart }),
      undefined,
    );
    // The note cannot be kept, but it is the next row that waits for a decision.
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.shopper);
  });

  it("changes the form before keeping it, in the toolbar's place", async () => {
    const h = renderDigest(SAMPLE_DIGEST, { defaultSelected: `suggested:${SAMPLE_IDS.cart}` });
    key("c");
    const input = slot("digest-change")!.querySelector("input")!;
    expect(input).toHaveValue("cart");
    expect(input).toHaveFocus();
    expect(slot("list-detail-toolbar")).toBeNull();
    await userEvent.clear(input);
    await userEvent.type(input, "shopping cart{Enter}");
    expect(h.onKeep).toHaveBeenCalledWith(
      expect.objectContaining({ id: SAMPLE_IDS.cart }),
      "shopping cart",
    );
    expect(slot("digest-change")).toBeNull();
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.shopper);
  });

  it("keeps with this form from the button, and closes the field with Esc", async () => {
    const h = renderDigest(SAMPLE_DIGEST, { defaultSelected: kapiMart });
    fireEvent.click(within(toolbar()).getByRole("button", { name: "Change" }));
    fireEvent.keyDown(slot("digest-change")!.querySelector("input")!, { key: "Escape" });
    expect(slot("digest-change")).toBeNull();
    expect(h.onKeep).not.toHaveBeenCalled();

    fireEvent.click(within(toolbar()).getByRole("button", { name: "Change" }));
    fireEvent.change(slot("digest-change")!.querySelector("input")!, {
      target: { value: "Kapimart" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Keep with this form" }));
    expect(h.onKeep).toHaveBeenCalledWith(
      expect.objectContaining({ id: SAMPLE_IDS.kapiMart }),
      "Kapimart",
    );
  });

  it("drops with the button and with d", () => {
    const h = renderDigest(SAMPLE_DIGEST, { defaultSelected: kapiMart });
    fireEvent.click(within(toolbar()).getByRole("button", { name: "Drop" }));
    expect(h.onDrop).toHaveBeenCalledWith(expect.objectContaining({ id: SAMPLE_IDS.kapiMart }));
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.cart);
    key("d");
    expect(h.onDrop).toHaveBeenLastCalledWith(expect.objectContaining({ id: SAMPLE_IDS.cart }));
  });

  it("offers Drop alone on a note, which cannot be kept as a rule", () => {
    renderDigest(SAMPLE_DIGEST, { defaultSelected: `suggested:${SAMPLE_IDS.shopper}` });
    const buttons = within(toolbar())
      .getAllByRole("button")
      .map((b) => b.getAttribute("data-slot"));
    expect(buttons).toEqual(["drop-item"]);
  });

  it("keeps every suggestion of its theme and collection with g", () => {
    const h = renderDigest(SAMPLE_DIGEST, { defaultSelected: kapiMart });
    expect(slot("digest-shortcuts")).toHaveTextContent("keeps all 2 suggestions");
    key("g");
    expect(h.onKeepGroup).toHaveBeenCalledWith([
      expect.objectContaining({ id: SAMPLE_IDS.checkout }),
      expect.objectContaining({ id: SAMPLE_IDS.kapiMart }),
    ]);
    expect(row(SAMPLE_IDS.checkout)).toHaveAttribute("data-decided");
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.cart);
  });

  it("opens the file the evidence names, from the link and with o", () => {
    const h = renderDigest(SAMPLE_DIGEST, { defaultSelected: kapiMart });
    fireEvent.click(slot("open-file")!);
    expect(h.onOpenFile).toHaveBeenCalledWith("web/en/getting-started.md");
    key("j");
    key("o");
    expect(h.onOpenFile).toHaveBeenLastCalledWith("src/en/store-ui.json");
  });

  it("gives the project's own words back beside a rule", () => {
    renderDigest(CONTEXT_DIGEST, { defaultSelected: `established:${studioRule().id}` });
    expect(slot("digest-usage")?.textContent).toBe(
      'docs/ says "studio" 41 times and "business" twice',
    );
  });
});

describe("a disagreement", () => {
  const sides = SAMPLE_DIGEST.conflicts[0].sides;

  it("lays its sides beside each other under the sentence that explains it", () => {
    renderDigest();
    expect(slot("digest-conflict-reason")).toHaveTextContent(
      "These rules say different things about the same word.",
    );
    expect(all("digest-conflict-reason")).toHaveLength(1);
    const cards = all("digest-side");
    expect(cards.map((c) => c.getAttribute("data-id"))).toEqual([sides[0].id, sides[1].id]);
    expect(cards[0]).toHaveTextContent("Write log in, not sign in");
    expect(cards[0]).toHaveTextContent("Please verify your email address before signing in.");
    expect(cards[0]).toHaveTextContent("codex");
    expect(cards[1]).toHaveTextContent("A person changed this in the error messages.");
    expect(cards[1]).toHaveTextContent("claude-code");
  });

  it("chooses either side with its own button, and the first with a", () => {
    const h = renderDigest();
    const chooses = all("digest-side").map((c) =>
      within(c).getByRole("button", { name: "Choose this side" }),
    );
    expect(chooses).toHaveLength(2);
    fireEvent.click(chooses[1]);
    expect(h.onChoose).toHaveBeenCalledWith(
      expect.objectContaining({ id: sides[1].id }),
      undefined,
    );
    expect(h.onKeep).not.toHaveBeenCalled();
    expect(selectedRow()).toHaveAttribute("data-id", SAMPLE_IDS.payPal);
  });

  it("chooses the first side with a", () => {
    const h = renderDigest();
    key("a");
    expect(h.onChoose).toHaveBeenCalledWith(
      expect.objectContaining({ id: sides[0].id }),
      undefined,
    );
    expect(h.onKeep).not.toHaveBeenCalled();
  });

  it("changes a side's form before choosing it", () => {
    const h = renderDigest();
    const card = all("digest-side")[1];
    fireEvent.click(within(card).getByRole("button", { name: "Change" }));
    const input = card.querySelector("[data-slot='digest-change'] input")!;
    expect(input).toHaveValue("sign in");
    fireEvent.change(input, { target: { value: "sign-in" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(h.onChoose).toHaveBeenCalledWith(
      expect.objectContaining({ id: sides[1].id }),
      "sign-in",
    );
  });

  it("offers no Drop and no d while a rival rule stands on the other side", () => {
    const h = renderDigest();
    expect(screen.queryByRole("button", { name: "Drop" })).toBeNull();
    key("d");
    expect(h.onDrop).not.toHaveBeenCalled();
  });

  it("offers Keep and Drop for a rule the evidence alone turned against", () => {
    const side = { ...sides[1], droppable: true };
    const h = renderDigest({
      ...SAMPLE_DIGEST,
      conflicts: [
        { sides: [side], by_evidence: true, reason: 'You changed "sign in" back to "log in".' },
      ],
    });
    expect(slot("digest-conflict-reason")).toHaveTextContent(
      'You changed "sign in" back to "log in".',
    );
    fireEvent.click(screen.getByRole("button", { name: "Drop" }));
    expect(h.onDrop).toHaveBeenCalledWith(expect.objectContaining({ id: side.id }));
  });
});

describe("a rule in force", () => {
  const studio = `established:${studioRule().id}`;

  it("offers Apply more widely and Drop, and says how it was established", async () => {
    const h = renderDigest(CONTEXT_DIGEST, { defaultSelected: studio });
    expect(within(toolbar()).queryByRole("button", { name: "Keep" })).toBeNull();
    expect(slot("digest-how")).toHaveTextContent("How it was established");
    expect(slot("digest-how")).toHaveTextContent("merged in #412");
    await userEvent.click(screen.getByLabelText("Apply more widely"));
    await userEvent.click(screen.getByRole("option", { name: "To every project" }));
    expect(h.onWiden).toHaveBeenCalledWith(
      expect.objectContaining({ id: studioRule().id }),
      "workspace",
    );
    fireEvent.click(within(toolbar()).getByRole("button", { name: "Drop" }));
    expect(h.onDrop).toHaveBeenCalledWith(expect.objectContaining({ status: "established" }));
  });

  it("names how it was kept at the end of its row", () => {
    renderDigest(CONTEXT_DIGEST);
    expect(row(studioRule().id)).toHaveTextContent("merged in #412");
  });

  it("hides Apply more widely when the rule has nowhere wider to go, or nothing can widen", () => {
    renderDigest(CONTEXT_DIGEST, { defaultSelected: `drift:${CONTEXT_DIGEST.drift[0].rule.id}` });
    expect(all("widen-picker")).toHaveLength(1);
    expect(slot("digest-drift-line")).toHaveTextContent('docs/ says "recording" 11 times');
  });

  it("hides Apply more widely without a handler", () => {
    renderDigest(CONTEXT_DIGEST, { defaultSelected: studio, onWiden: undefined });
    expect(all("widen-picker")).toHaveLength(0);
  });
});

describe("without a checkout", () => {
  it("reads, and offers no decisions and no file links", () => {
    const h = renderDigest(SAMPLE_DIGEST, { canDecide: false, onOpenFile: undefined });
    expect(all("keep-item")).toHaveLength(0);
    expect(all("open-file")).toHaveLength(0);
    key("a");
    expect(h.onChoose).not.toHaveBeenCalled();
  });
});

describe("an empty digest", () => {
  it("says what would appear, and links to the Assistant section when it can", () => {
    const onConnectAssistant = vi.fn();
    renderDigest(EMPTY_DIGEST, { onConnectAssistant, since: undefined });
    expect(slot("digest-empty")).toHaveTextContent("Nothing is recorded yet");
    fireEvent.click(screen.getByRole("button", { name: "Connect an assistant" }));
    expect(onConnectAssistant).toHaveBeenCalled();
    expect(slot("list-detail")).toBeNull();
  });

  it("leaves the button out when there is no Assistant section to open", () => {
    renderDigest(EMPTY_DIGEST, { since: undefined });
    expect(screen.queryByRole("button", { name: "Connect an assistant" })).toBeNull();
  });
});

describe("a scope description", () => {
  it("splits into its level and its point", () => {
    expect(scopeParts("project")).toEqual({ level: "project", coordinates: [] });
    expect(scopeParts("workspace product=studio")).toEqual({
      level: "workspace",
      coordinates: [["product", "studio"]],
    });
    expect(scopeParts("project (every profile) brand=kapimart,channel=web")).toEqual({
      level: "project (every profile)",
      coordinates: [
        ["brand", "kapimart"],
        ["channel", "web"],
      ],
    });
  });
});

describe("the rows", () => {
  it("are listed in the order the groups are shown, each key unique", () => {
    const rows = digestRows(CONTEXT_DIGEST);
    expect(rows.map((r) => r.group)).toEqual([
      "conflict",
      "suggested",
      "suggested",
      "suggested",
      "suggested",
      "established",
      "earlier",
      "drift",
    ]);
    expect(new Set(rows.map((r) => r.key)).size).toBe(rows.length);
  });

  it("move on to the next decision, then back up, then to none", () => {
    const rows = digestRows(SAMPLE_DIGEST);
    const keys = rows.map((r) => r.key);
    expect(defaultRowKey(rows)).toBe(keys[0]);
    expect(nextRowKey(rows, keys[0])).toBe(keys[1]);
    expect(nextRowKey(rows, keys[5])).toBe(keys[4]);
    const decided = new Set(keys.slice(0, 5));
    expect(nextRowKey(rows, keys[5], decided)).toBeNull();
    expect(defaultRowKey(rows, new Set(keys))).toBeNull();
  });

  it("move on from a rule in force to the row after it", () => {
    const rows = digestRows(CONTEXT_DIGEST);
    const established = rows.find((r) => r.group === "established")!;
    expect(nextRowKey(rows, established.key)).toBe(rows[rows.indexOf(established) + 1].key);
  });
});
