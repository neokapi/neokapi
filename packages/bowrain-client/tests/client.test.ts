import { describe, expect, it } from "vite-plus/test";

import type { ChangeResult } from "@neokapi/contract-types";
import type { Run } from "@neokapi/kapi-format";

import {
  approveSet,
  createBowrainClient,
  diffRuns,
  fixSet,
  readOutcome,
  rejectSet,
  type DeckCard,
  type PendingReviewPage,
} from "../src/index.ts";

const ph: Run = { ph: { id: "1", type: "x-var", data: "{{name}}", equiv: "{{name}}" } };

const card: DeckCard = {
  key: "b1:nb",
  blockId: "b1",
  item: "i18n/en.json",
  locale: "nb",
  collectionId: "c1",
  source: [{ text: "Hello " }, ph],
  target: [{ text: "Hei " }, ph],
  targetRevision: "r:0123456789abcdef",
  sourceRevision: "r:aaaaaaaaaaaaaaaa",
  group: "fast",
  notes: [],
  findingsCount: 0,
  failingFindingsCount: 0,
  ownWork: false,
};

describe("change sets", () => {
  it("approves on the revision the card showed", () => {
    const set = approveSet(card);
    expect(set.ops).toEqual([
      {
        op: "decide",
        at: { doc: "i18n/en.json", block: "b1", edition: "nb" },
        if_match: "r:0123456789abcdef",
        basis: "r:aaaaaaaaaaaaaaaa",
        outcome: "establish",
      },
    ]);
    expect(set.require_basis).toBe(true);
  });

  it("corrects and approves in one set, codes without native data", () => {
    const set = fixSet(card, [{ text: "Hallo " }, ph], ["tone"]);
    expect(set.ops.map((o) => o.op)).toEqual(["set_content", "decide"]);
    const content = set.ops[0] as { runs: unknown[]; if_match: string };
    expect(content.if_match).toBe(card.targetRevision);
    expect(content.runs[1]).toEqual({ ph: { id: "1", type: "x-var", equiv: "{{name}}" } });
    expect((set.ops[1] as { reasons?: string[] }).reasons).toEqual(["tone"]);
  });

  it("reads a partial result as held for a second reviewer", () => {
    const result = {
      schema: "kapi.change-result/v1",
      status: "partial",
      record: "rec",
      docs: [],
      ops: [
        { i: 0, op: "set_content", status: "applied" },
        { i: 1, op: "decide", status: "refused", error: { code: "not_permitted", message: "sod" } },
      ],
    } as unknown as ChangeResult;
    expect(readOutcome(result).status).toBe("held");
  });

  it("rejects with reasons and a note", () => {
    const set = rejectSet(card, ["meaning"], "Says the opposite");
    expect(set.note).toBe("Says the opposite");
    expect(set.ops[0]).toMatchObject({ outcome: "reject", reasons: ["meaning"] });
  });

  it("reads a stale refusal with the current edition", () => {
    const result = {
      schema: "kapi.change-result/v1",
      status: "refused",
      record: null,
      docs: [],
      ops: [
        {
          i: 0,
          op: "decide",
          status: "refused",
          error: { code: "stale", message: "moved" },
          current: { rev: "r:fedcba9876543210", text: "Hei der" },
        },
      ],
    } as unknown as ChangeResult;
    const outcome = readOutcome(result);
    expect(outcome.status).toBe("stale");
    if (outcome.status === "stale") expect(outcome.current.rev).toBe("r:fedcba9876543210");
  });
});

describe("diffRuns", () => {
  it("marks changed words and keeps the placeholder", () => {
    const spans = diffRuns([{ text: "Hei " }, ph], [{ text: "Hallo " }, ph]);
    expect(spans.filter((s) => s.op !== "same").map((s) => s.op)).toEqual(["del", "add"]);
    expect(spans.at(-1)).toMatchObject({ op: "same", token: { kind: "code", id: "1" } });
  });
});

describe("client", () => {
  it("falls back to the pending-review queue when the deck route is absent", async () => {
    const page: PendingReviewPage = {
      total: 1,
      limit: 200,
      offset: 0,
      entries: [
        {
          block_id: "b1",
          item_name: "i18n/en.json",
          locale: "nb",
          collection_id: "c1",
          block: {
            id: "b1",
            source: "Hello {{name}}",
            source_runs: [{ text: "Hello " }, ph],
            targets: { nb: { text: "Hei {{name}}", status: "translated" } },
            targets_runs: { nb: [{ text: "Hei " }, ph] },
            target_revisions: { nb: "r:0123456789abcdef" },
          },
        },
      ],
    };
    const calls: string[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server/",
      getAccessToken: () => "t",
      fetch: (async (url: string) => {
        calls.push(url);
        if (url.includes("/review-deck/")) return new Response("{}", { status: 404 });
        return new Response(JSON.stringify(page), { status: 200 });
      }) as typeof fetch,
    });
    const deck = await client.deck({ workspace: "ws", project: "p" }, "nb");
    expect(deck.ordered).toBe(false);
    expect(deck.cards).toHaveLength(1);
    expect(deck.cards[0]).toMatchObject({ item: "i18n/en.json", group: "unsorted" });
    expect(calls[1]).toBe("http://server/api/v1/ws/p/pending-review/main?locales=nb&limit=200");
  });

  it("refreshes once on a 401", async () => {
    let token = "old";
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => token,
      onUnauthorized: async () => (token = "new"),
      fetch: (async (_url: string, init: RequestInit) => {
        const auth = (init.headers as Record<string, string>).Authorization;
        if (auth === "Bearer old") return new Response("{}", { status: 401 });
        return new Response(JSON.stringify([{ id: "w", name: "W", slug: "w" }]), { status: 200 });
      }) as typeof fetch,
    });
    expect(await client.workspaces()).toHaveLength(1);
  });
});
