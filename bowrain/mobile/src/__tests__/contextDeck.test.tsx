import type { ContextCard as Card } from "@neokapi/bowrain-client";
import { fireEvent, render, screen } from "@testing-library/react-native";

import "../lib/i18n";
import { ContextCard } from "../components/ContextCard";
import { SinceCard, Summary } from "../components/ContextSession";
import {
  contextCardId,
  contextCopyPrefix,
  contextDeckView,
  mergeContextPages,
  reachShare,
  refusalCode,
  sessionThrough,
  sinceHasNews,
  widestReach,
} from "../lib/contextDeck";

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

  it("labels a proposed term and shows how many blocks use it, by collection", async () => {
    const candidate: Card = {
      item: { kind: "concept", id: "c9" },
      kind: "concept",
      revision: "u1",
      terms: [{ text: "Dashpanel", locale: "en", status: "proposed" }],
      text: "The panel listing a project's files.",
      language: "en",
      where: { level: "project", coordinates: { product: "bowrain" } },
      author: { kind: "tool", name: "context-scan", agent_authored: true },
      own_work: false,
      basis: "agent_authored",
      actions: ["keep", "reject", "later"],
      proposed: true,
      reach: {
        blocks: 1204,
        collections: [
          { name: "bowrain-app", blocks: 1200 },
          { name: "", blocks: 4 },
        ],
      },
    };
    await render(<ContextCard card={candidate} />);
    expect(screen.getByTestId("context-kind")).toHaveTextContent("Proposed term");
    expect(screen.getByTestId("context-reach")).toHaveTextContent(/Used in 1,204 blocks/);
    const cols = screen.getAllByTestId("context-reach-collection");
    expect(cols[0]).toHaveTextContent("bowrain-app: 1,200");
    expect(cols[1]).toHaveTextContent("Files outside a collection: 4");
    expect(screen.getByTestId("context-where")).toHaveTextContent(
      "This project · product: bowrain",
    );

    await render(<ContextCard card={{ ...candidate, reach: undefined }} />);
    expect(screen.getByTestId("context-reach")).toHaveTextContent("Not counted yet");

    await render(<ContextCard card={concept} />);
    expect(screen.queryByTestId("context-reach")).toBeNull();
    expect(screen.getByTestId("context-kind")).toHaveTextContent("Term");
  });

  it("shows a proposed translation with its source term, language and reach", async () => {
    const translation: Card = {
      item: { kind: "term_proposal", id: "c9/nb" },
      kind: "term_translation",
      revision: "p1",
      text: "A shared place for a team's projects.",
      language: "nb",
      translation: { term: "workspace", proposed: "arbeidsområde", locale: "nb" },
      where: { level: "project" },
      author: { kind: "tool", name: "term-proposal", agent_authored: true },
      own_work: false,
      basis: "agent_authored",
      actions: ["keep", "reject", "later"],
      reach: { blocks: 40, collections: [{ name: "docs", blocks: 40 }] },
    };
    await render(<ContextCard card={translation} />);
    expect(screen.getByTestId("context-kind")).toHaveTextContent("Proposed translation");
    expect(screen.getByText("NB")).toBeTruthy();
    expect(screen.getByTestId("context-translation-source")).toHaveTextContent("workspace");
    expect(screen.getByTestId("context-translation-proposed")).toHaveTextContent("arbeidsområde");
    expect(screen.getByTestId("context-reach")).toHaveTextContent(/Used in 40 blocks/);
    expect(contextCopyPrefix(translation)).toBe("context.translation");
    expect(contextCopyPrefix({ kind: "concept", proposed: true })).toBe("context.candidate");
    expect(contextCopyPrefix(concept)).toBe("context");
  });

  it("marks a note recorded in an agent host's shell, changed since its review", async () => {
    await render(<ContextCard card={note} />);
    expect(screen.getByTestId("context-note")).toHaveTextContent(note.text ?? "");
    expect(screen.getByText("Changed since Bea reviewed it")).toBeTruthy();
    expect(screen.getByTestId("context-author")).toHaveTextContent(/In claude-code/);
  });

  it("shows a session card's reach as a bar against the widest term, its channels and proposals", async () => {
    const candidate: Card = {
      ...concept,
      item: { kind: "concept", id: "c7" },
      terms: [{ text: "workspace", locale: "en", status: "proposed" }],
      proposed: true,
      actions: ["keep", "reject", "later"],
      reach: { blocks: 103, collections: [{ name: "cli-pages", blocks: 103 }] },
    };
    await render(
      <ContextCard
        card={candidate}
        session={{
          widest: 412,
          channels: ["cli", "docs"],
          proposals: [{ term: "workspace", proposed: "arbeidsområde", locale: "nb" }],
          isNew: true,
        }}
      />,
    );
    expect(screen.getByTestId("context-session-reach")).toHaveTextContent(/Used in 103 blocks/);
    expect(screen.getByTestId("context-reach-bar")).toHaveStyle({ width: "25%" });
    expect(screen.getByTestId("context-channels")).toHaveTextContent(/cli.*docs/);
    expect(screen.getByTestId("context-proposal")).toHaveTextContent(/arbeidsområde/);
    expect(screen.getByTestId("context-new")).toHaveTextContent("New since the last session");
    expect(screen.queryByTestId("context-reach")).toBeNull();
  });
});

describe("a context session", () => {
  it("measures reach against the widest card and knows when it is through", () => {
    expect(widestReach([{ reach: { blocks: 40, collections: [] } }, {}])).toBe(40);
    expect(widestReach([])).toBe(0);
    expect(reachShare(10, 40)).toBe(0.25);
    expect(reachShare(undefined, 40)).toBe(0);
    expect(reachShare(5, 0)).toBe(0);

    const ids = [rule, note].map(contextCardId);
    expect(sessionThrough([rule, note], new Set([ids[0]!]), [])).toBe(false);
    expect(sessionThrough([rule, note], new Set([ids[0]!]), [ids[1]!])).toBe(true);
    expect(sessionThrough([], new Set(), [])).toBe(true);
  });

  it("opens with what changed since the last session, when anything did", async () => {
    const since = {
      at: "2026-11-17T10:00:00Z",
      by: "Ada",
      added_total: 4,
      added: [{ item: { kind: "concept" as const, id: "c1" }, term: "workspace", blocks: 412 }],
      grew_total: 1,
      grew: [
        {
          item: { kind: "concept" as const, id: "c2" },
          term: "change set",
          blocks: 90,
          before: 30,
        },
      ],
      decided_by_others: 2,
    };
    expect(sinceHasNews(since)).toBe(true);
    expect(sinceHasNews({ ...since, added_total: 0, grew_total: 0, decided_by_others: 0 })).toBe(
      false,
    );
    expect(sinceHasNews(undefined)).toBe(false);

    const onStart = jest.fn();
    await render(<SinceCard since={since} onStart={onStart} />);
    expect(screen.getByTestId("context-since-added")).toHaveTextContent(/4 new terms found/);
    expect(screen.getByTestId("context-since-added")).toHaveTextContent(/workspace: 412 blocks/);
    expect(screen.getByTestId("context-since-grew")).toHaveTextContent(
      /change set: from 30 to 90 blocks/,
    );
    expect(screen.getByTestId("context-since-others")).toHaveTextContent(/Others decided 2 items/);
    expect(screen.getByText(/Ada ended the last session/)).toBeTruthy();
    fireEvent.press(screen.getByTestId("context-since-start"));
    expect(onStart).toHaveBeenCalled();
  });

  it("ends with what the session moved, leaving out what the server did not count", async () => {
    const onMore = jest.fn();
    const end = {
      session: { id: "s1", started_at: "t0", expires_at: "t1", ended_at: "t2" },
      summary: {
        decided: 10,
        kept: 7,
        changed: 2,
        rejected: 1,
        blocks: 1930,
        waiting: 12,
        next: "change set",
      },
    };
    await render(<Summary end={end} onMore={onMore} onStop={() => {}} />);
    expect(screen.getByTestId("context-summary-decided")).toHaveTextContent(
      "You decided 10 items.",
    );
    expect(screen.getByTestId("context-summary-outcomes")).toHaveTextContent(
      "7 kept · 2 changed · 1 turned down",
    );
    expect(screen.getByTestId("context-summary-blocks")).toHaveTextContent(
      "1,930 blocks now follow your terms.",
    );
    expect(screen.queryByTestId("context-summary-findings")).toBeNull();
    expect(screen.getByTestId("context-summary-left")).toHaveTextContent(
      "12 items left · next: change set",
    );
    fireEvent.press(screen.getByTestId("context-summary-more"));
    expect(onMore).toHaveBeenCalled();

    await render(
      <Summary
        end={{ ...end, summary: { ...end.summary, waiting: 0, blocks: undefined } }}
        onMore={onMore}
        onStop={() => {}}
      />,
    );
    expect(screen.queryByTestId("context-summary-more")).toBeNull();
    expect(screen.queryByTestId("context-summary-blocks")).toBeNull();
  });
});
