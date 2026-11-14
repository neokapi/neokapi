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

  it("reads the server's deck page and its fix candidates", async () => {
    const deckBody = {
      locale: "nb",
      cards: [
        {
          block_id: "b1",
          item_name: "web/i18n/en.json",
          key: "settings.save",
          collection_id: "c1",
          locale: "nb",
          source_runs: [{ text: "Save " }, ph],
          target_runs: [{ text: "Lagre " }, ph],
          source_revision: "r:0a1b2c3d4e5f6a7b",
          target_revision: "r:74dbfac2ed1c9d6e",
          notes: [{ kind: "term", text: "a note" }],
          own_work: true,
          group: "fast",
          reason: "no findings, pre-review score 92",
        },
      ],
      counts: { fast: 1, closer: 0, desk: 0, total: 1 },
      next_cursor: "abc",
    };
    const fixesBody = {
      candidates: [
        { kind: "term", label: "Use x", runs: [{ text: "Lagre x" }], edit_text: "Lagre x" },
      ],
      dropped: [{ kind: "memory", why: "drops a placeholder" }],
      notes: ["no AI provider is configured"],
    };
    const urls: string[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (url: string) => {
        urls.push(url);
        return new Response(JSON.stringify(url.includes("fix-candidates") ? fixesBody : deckBody), {
          status: 200,
        });
      }) as typeof fetch,
    });
    const deck = await client.deck({ workspace: "ws", project: "p" }, "nb", {
      limit: 50,
      cursor: "x",
    });
    expect(urls[0]).toBe("http://server/api/v1/ws/p/review-deck/main?locale=nb&limit=50&cursor=x");
    expect(deck).toMatchObject({ ordered: true, nextCursor: "abc", total: 1 });
    expect(deck.cards[0]).toMatchObject({
      key: "b1:nb",
      name: "settings.save",
      sourceRevision: "r:0a1b2c3d4e5f6a7b",
      targetRevision: "r:74dbfac2ed1c9d6e",
      ownWork: true,
      group: "fast",
    });
    const fixes = await client.fixCandidates({ workspace: "ws", project: "p" }, deck.cards[0], {
      reason: "wrong_term",
    });
    expect(urls[1]).toBe(
      "http://server/api/v1/ws/p/blocks/main/b1/fix-candidates?locale=nb&reason=wrong_term",
    );
    expect(fixes?.candidates.map((c) => c.kind)).toEqual(["term"]);
    expect(fixes?.dropped).toHaveLength(1);
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

  it("signs out with the app's redirect and returns the provider's sign-out URL", async () => {
    const sent: { url: string; body?: string }[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (url: string, init: RequestInit) => {
        sent.push({ url, body: init.body as string | undefined });
        return new Response(
          JSON.stringify({ status: "logged out", end_session_url: "http://idp/logout?x=1" }),
          { status: 200 },
        );
      }) as typeof fetch,
    });
    const res = await client.logout({ postLogoutRedirectUri: "bowrain://auth/signed-out" });
    expect(res.endSessionUrl).toBe("http://idp/logout?x=1");
    expect(sent[0].url).toBe("http://server/api/v1/auth/logout");
    expect(JSON.parse(sent[0].body ?? "")).toEqual({
      post_logout_redirect_uri: "bowrain://auth/signed-out",
    });
  });

  it("signs out without a body, and with no URL when the server refuses", async () => {
    const bodies: unknown[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (_url: string, init: RequestInit) => {
        bodies.push(init.body);
        return new Response(JSON.stringify({ error: "bad" }), { status: 400 });
      }) as typeof fetch,
    });
    expect(await client.logout()).toEqual({});
    expect(bodies).toEqual([undefined]);
  });

  it("reads a feed page with its filters", async () => {
    const urls: string[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (url: string) => {
        urls.push(url);
        return new Response(
          JSON.stringify({
            entries: [
              {
                id: "a1",
                kind: "push",
                at: "2026-11-14T09:10:00.000001Z",
                actor: { id: "u1", kind: "person", name: "Ada" },
                counts: { files: 3, blocks: 40 },
                link: { screen: "stream", stream: "main" },
              },
            ],
            next_cursor: "",
          }),
          { status: 200 },
        );
      }) as typeof fetch,
    });
    const page = await client.feed("ws", "p", {
      kinds: ["push", "context_change"],
      coordinates: { product: "kapi", channel: "web" },
      actorKind: "agent",
      limit: 20,
    });
    expect(urls[0]).toBe(
      "http://server/api/v1/ws/p/feed?kind=push%2Ccontext_change&coordinate=product%3Akapi&coordinate=channel%3Aweb&actor_kind=agent&limit=20",
    );
    expect(page.entries[0].counts).toEqual({ files: 3, blocks: 40 });
    expect(page.next_cursor).toBeUndefined();
  });
});
