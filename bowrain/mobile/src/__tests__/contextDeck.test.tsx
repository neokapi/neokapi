import type { ContextCard as Card } from "@neokapi/bowrain-client";
import { render, screen } from "@testing-library/react-native";

import "../lib/i18n";
import { ContextCard } from "../components/ContextCard";
import { contextCardId, contextDeckView, mergeContextPages, refusalCode } from "../lib/contextDeck";

const rule: Card = {
  item: { kind: "op", id: "0a1" },
  kind: "term_rule",
  revision: "0a1",
  status: "suggested",
  term: { term: "translation memory", replacement: "content memory", forms: ["TM"] },
  language: "en",
  flags: ["fails"],
  where: { level: "project", coordinates: { product: "kapi" } },
  author: { kind: "agent", name: "claude-code", session: "0ab4e399ffff", agent_authored: true },
  own_work: false,
  basis: "agent_authored",
  actions: ["keep", "reject", "later"],
};

const concept: Card = {
  item: { kind: "concept", id: "c1" },
  kind: "concept",
  revision: "r3",
  terms: [
    { text: "content memory", locale: "en", status: "preferred" },
    { text: "innhaldsminne", locale: "nb", status: "preferred" },
  ],
  language: "en",
  where: { level: "workspace" },
  author: { kind: "person", name: "Asgeir", agent_authored: false },
  own_work: true,
  actions: ["reject", "later"],
};

const note: Card = {
  item: { kind: "op", id: "0a2" },
  kind: "note",
  revision: "0a3",
  status: "established",
  text: "Help pages address a reader who is stuck.",
  where: { level: "project" },
  author: { kind: "person", name: "Asgeir", host: "claude-code", agent_authored: true },
  review: {
    id: "0a9",
    by: { kind: "person", name: "Bea" },
    at: "2026-11-01T00:00:00Z",
    revision: "0a2",
    basis: "peer",
  },
  own_work: false,
  actions: ["keep", "reject", "later"],
};

describe("the context deck view", () => {
  it("names a card by its item and revision", () => {
    expect(contextCardId(rule)).toBe("op::0a1@0a1");
    expect(contextCardId({ ...rule, revision: "0b2" })).not.toBe(contextCardId(rule));
  });

  it("merges pages once each, and moves cards put off to the end", () => {
    const cards = mergeContextPages([
      [rule, concept],
      [concept, note],
    ]);
    expect(cards.map((c) => c.kind)).toEqual(["term_rule", "concept", "note"]);
    const view = contextDeckView(cards, new Set([contextCardId(note)]), [contextCardId(rule)]);
    expect(view.map((c) => c.kind)).toEqual(["concept", "term_rule"]);
  });

  it("reads the refusal code a decision came back with", () => {
    expect(refusalCode({ body: { code: "stale" } })).toBe("stale");
    expect(refusalCode(new Error("network"))).toBe("failed");
  });
});

describe("ContextCard", () => {
  it("shows a term rule with who wrote it", async () => {
    await render(<ContextCard card={rule} />);
    expect(screen.getByTestId("context-kind")).toHaveTextContent("Term rule");
    expect(screen.getByTestId("context-status")).toHaveTextContent("Suggested");
    expect(screen.getByTestId("context-use")).toHaveTextContent("content memory");
    expect(screen.getByTestId("context-avoid")).toHaveTextContent("translation memory, TM");
    expect(screen.getByTestId("context-where")).toHaveTextContent("This project · product: kapi");
    expect(screen.getByTestId("context-author")).toHaveTextContent(/claude-code/);
    expect(screen.getByTestId("context-author")).toHaveTextContent(/Agent · Session 0ab4e399/);
  });

  it("shows a concept's terms, source first, and says who keeps a person's own item", async () => {
    await render(<ContextCard card={concept} />);
    const terms = screen.getAllByTestId("context-term");
    expect(terms).toHaveLength(2);
    expect(terms[0]).toHaveTextContent(/content memory/);
    expect(screen.getByText("You wrote this, so another person keeps it.")).toBeTruthy();
  });

  it("marks a note recorded in an agent host's shell, changed since its review", async () => {
    await render(<ContextCard card={note} />);
    expect(screen.getByTestId("context-note")).toHaveTextContent(note.text ?? "");
    expect(screen.getByText("Changed since Bea reviewed it")).toBeTruthy();
    expect(screen.getByTestId("context-author")).toHaveTextContent(/In claude-code/);
  });
});
