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
  revisionsOf,
  withdrawSet,
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
  const copied: DeckCard = {
    ...card,
    copies: [
      {
        blockId: "b2",
        item: "web/en.json",
        collectionId: "c2",
        targetRevision: "r:2222222222222222",
        sourceRevision: "r:aaaaaaaaaaaaaaaa",
      },
    ],
  };

  it("decides each copy a card stands for in the same set", () => {
    const set = approveSet(copied);
    expect(set.ops.map((op) => ("at" in op ? op.at.block : ""))).toEqual(["b1", "b2"]);
    expect(set.ops[1]).toMatchObject({
      op: "decide",
      at: { doc: "web/en.json", block: "b2", edition: "nb" },
      if_match: "r:2222222222222222",
      outcome: "establish",
    });
    const fix = fixSet(copied, [{ text: "Hallo " }, ph], []);
    expect(fix.ops.map((op) => op.op)).toEqual(["set_content", "decide", "set_content", "decide"]);
    expect(rejectSet(copied, ["meaning"]).ops).toHaveLength(2);
  });

  it("withdraws each copy on the revision the decision left there", () => {
    const set = approveSet(copied);
    const result = {
      status: "applied",
      ops: [
        { i: 0, op: "decide", status: "applied", after: "r:a1" },
        { i: 1, op: "decide", status: "applied", after: "r:b2" },
      ],
    } as unknown as ChangeResult;
    expect(revisionsOf(set, result)).toEqual({ b1: "r:a1", b2: "r:b2" });
    const undo = withdrawSet(copied, "r:a1", { b2: "r:b2" });
    expect(undo.ops).toEqual([
      {
        op: "decide",
        at: { doc: "i18n/en.json", block: "b1", edition: "nb" },
        if_match: "r:a1",
        outcome: "withdraw",
      },
      {
        op: "decide",
        at: { doc: "web/en.json", block: "b2", edition: "nb" },
        if_match: "r:b2",
        outcome: "withdraw",
      },
    ]);
  });

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
    if (outcome.status === "stale") expect(outcome.current?.rev).toBe("r:fedcba9876543210");
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

  it("narrows the deck to a collection and lists a language's collections", async () => {
    const collections = {
      locale: "nb",
      stream: "main",
      all: {
        collection_id: "",
        name: "",
        waiting: 3,
        fast: 2,
        closer: 1,
        desk: 0,
        unclassified: 0,
        translations: 5,
        decided: 2,
      },
      collections: [
        {
          collection_id: "c1",
          name: "web",
          waiting: 3,
          fast: 2,
          closer: 1,
          desk: 0,
          unclassified: 0,
          translations: 5,
          decided: 2,
        },
      ],
    };
    const urls: string[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (url: string) => {
        urls.push(url);
        if (url.includes("/collections"))
          return new Response(JSON.stringify(collections), { status: 200 });
        return new Response(
          JSON.stringify({
            locale: "nb",
            cards: [],
            counts: { fast: 0, closer: 0, desk: 0, total: 0 },
          }),
          {
            status: 200,
          },
        );
      }) as typeof fetch,
    });
    const scope = { workspace: "ws", project: "p" };
    expect(await client.deckCollections(scope, "nb")).toEqual(collections);
    await client.deck(scope, "nb", { limit: 50, collection: "c1" });
    await client.deck(scope, "nb", { collection: "" });
    expect(urls).toEqual([
      "http://server/api/v1/ws/p/review-deck/main/collections?locale=nb",
      "http://server/api/v1/ws/p/review-deck/main?locale=nb&limit=50&collection=c1",
      "http://server/api/v1/ws/p/review-deck/main?locale=nb&collection=",
    ]);
  });

  it("answers null for a language's collections from a server without the route", async () => {
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async () => new Response("{}", { status: 404 })) as typeof fetch,
    });
    expect(await client.deckCollections({ workspace: "ws", project: "p" }, "nb")).toBeNull();
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
  it("reads the review inbox, and null from a server without it", async () => {
    const language = {
      workspace: "ws",
      workspace_name: "WS",
      project: "p",
      project_name: "Site",
      stream: "main",
      locale: "nb",
      waiting: 12,
      fast: 8,
      closer: 2,
      desk: 1,
      unclassified: 1,
    };
    const urls: string[] = [];
    const answering = (status: number) =>
      createBowrainClient({
        baseUrl: "http://server",
        getAccessToken: () => "t",
        fetch: (async (url: string) => {
          urls.push(url);
          return new Response(JSON.stringify(status === 200 ? { languages: [language] } : {}), {
            status,
          });
        }) as typeof fetch,
      });
    expect(await answering(200).reviewInbox()).toEqual([language]);
    expect(urls[0]).toBe("http://server/api/v1/auth/me/review");
    expect(await answering(404).reviewInbox()).toBeNull();
    await expect(answering(500).reviewInbox()).rejects.toThrow();
  });

  it("reads the context waiting in the inbox, and an older server's answer as none", async () => {
    const entry = {
      workspace: "ws",
      workspace_name: "WS",
      project: "p",
      project_name: "Site",
      waiting: 3,
      by_kind: { term_rule: 2, note: 1 },
    };
    const answering = (body: object) =>
      createBowrainClient({
        baseUrl: "http://server",
        getAccessToken: () => "t",
        fetch: (async () => new Response(JSON.stringify(body), { status: 200 })) as typeof fetch,
      });
    expect(await answering({ context: [entry], languages: [] }).reviewHome()).toEqual({
      context: [entry],
      languages: [],
    });
    expect(await answering({ languages: [] }).reviewHome()).toEqual({ context: [], languages: [] });
  });

  it("pages the context deck and sends a decision", async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (url: string, init?: RequestInit) => {
        calls.push({ url, init });
        const body = url.endsWith("/decide")
          ? { outcome: "attested" }
          : {
              mode: "pending",
              bound: true,
              counts: { total: 0, by_kind: {}, agent: 0, person: 0 },
              cards: [],
              next_cursor: "",
            };
        return new Response(JSON.stringify(body), { status: 200 });
      }) as typeof fetch,
    });
    const deck = await client.contextDeck("ws", "p", { limit: 20, cursor: "40" });
    expect(calls[0]?.url).toBe(
      "http://server/api/v1/ws/p/context/deck?mode=pending&limit=20&cursor=40",
    );
    expect(deck.next_cursor).toBeUndefined();
    const result = await client.decideContext("ws", "p", {
      item: { kind: "op", id: "0abc" },
      revision: "0abc",
      action: "keep",
    });
    expect(result.outcome).toBe("attested");
    expect(calls[1]?.url).toBe("http://server/api/v1/ws/p/context/deck/decide");
    expect(calls[1]?.init?.method).toBe("POST");
  });

  it("maps a candidate term's reach and fills what an older server leaves out", async () => {
    const card = (extra: object) => ({
      item: { kind: "concept", id: "c1" },
      kind: "concept",
      revision: "u1",
      where: { level: "project", coordinates: { product: "bowrain" } },
      author: { kind: "tool", name: "context-scan", agent_authored: true },
      own_work: false,
      actions: ["keep", "reject", "later"],
      ...extra,
    });
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async () =>
        new Response(
          JSON.stringify({
            mode: "pending",
            bound: true,
            counts: { total: 3, by_kind: { concept: 3 }, agent: 3, person: 0 },
            cards: [
              card({
                proposed: true,
                reach: { blocks: 6, collections: [{ name: "bowrain-app", blocks: 6 }] },
              }),
              card({ proposed: true, reach: { blocks: 0 } }),
              card({}),
            ],
          }),
          { status: 200 },
        )) as typeof fetch,
    });
    const deck = await client.contextDeck("ws", "p");
    expect(deck.counts.proposed).toBe(0);
    expect(deck.cards[0]?.proposed).toBe(true);
    expect(deck.cards[0]?.reach).toEqual({
      blocks: 6,
      collections: [{ name: "bowrain-app", blocks: 6 }],
    });
    expect(deck.cards[0]?.where.coordinates).toEqual({ product: "bowrain" });
    expect(deck.cards[1]?.reach).toEqual({ blocks: 0, collections: [] });
    expect(deck.cards[2]?.proposed).toBe(false);
    expect(deck.cards[2]?.reach).toBeUndefined();
  });

  it("asks for proposed target terms and sends a reviewer's wording with a keep", async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (url: string, init?: RequestInit) => {
        calls.push({ url, init });
        const body = url.endsWith("/term-proposals")
          ? { waiting: 3, job_id: "j1", status: "queued" }
          : { outcome: "kept" };
        return new Response(JSON.stringify(body), { status: url.endsWith("/decide") ? 200 : 202 });
      }) as typeof fetch,
    });
    expect(await client.proposeTermTranslations("ws", "p")).toEqual({
      waiting: 3,
      job_id: "j1",
      status: "queued",
    });
    expect(calls[0]?.url).toBe("http://server/api/v1/ws/p/context/term-proposals");
    expect(calls[0]?.init?.method).toBe("POST");
    await client.decideContext("ws", "p", {
      item: { kind: "term_proposal", id: "c1/nb" },
      revision: "p1",
      action: "keep",
      text: "arbeidsområde",
    });
    expect(JSON.parse(String(calls[1]?.init?.body))).toMatchObject({ text: "arbeidsområde" });
  });
});

describe("deckPages", () => {
  it("reads the deck by page, and null from a server without the route", async () => {
    const calls: string[] = [];
    let present = true;
    const client = createBowrainClient({
      baseUrl: "http://server/",
      getAccessToken: () => "t",
      fetch: (async (url: string) => {
        calls.push(url);
        if (!present) return new Response("{}", { status: 404 });
        return new Response(
          JSON.stringify({
            locale: "nb",
            pages: [
              {
                kind: "component",
                title: "OfflineLaunch",
                item_name: "app.kbf.json",
                collection_id: "c1",
                component: "OfflineLaunch",
                waiting: 3,
                first_block_id: "b1",
              },
              {
                kind: "document",
                title: "guide.md",
                item_name: "docs/guide.md",
                waiting: 2,
                first_block_id: "b9",
              },
            ],
            waiting: 5,
            more: false,
          }),
          { status: 200 },
        );
      }) as typeof fetch,
    });
    const scope = { workspace: "ws", project: "p" };
    const got = await client.deckPages(scope, "nb", { collection: "c1" });
    expect(calls[0]).toBe(
      "http://server/api/v1/ws/p/review-deck/main/pages?locale=nb&collection=c1",
    );
    expect(got?.waiting).toBe(5);
    expect(got?.pages[0]).toEqual({
      kind: "component",
      title: "OfflineLaunch",
      item: "app.kbf.json",
      collectionId: "c1",
      component: "OfflineLaunch",
      waiting: 3,
      findings: 0,
      firstBlockId: "b1",
    });
    expect(got?.pages[1]).toMatchObject({
      kind: "document",
      collectionId: "",
      component: undefined,
    });
    present = false;
    expect(await client.deckPages(scope, "nb")).toBeNull();
  });
});

describe("updateMe", () => {
  it("patches the signed-in person's name", async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (url: string, init?: RequestInit) => {
        calls.push({ url, init });
        return new Response(
          JSON.stringify({ id: "u1", email: "a@b.c", name: "Ada", shown_name: "Ada" }),
          {
            status: 200,
          },
        );
      }) as typeof fetch,
    });
    expect((await client.updateMe({ name: "Ada" })).shown_name).toBe("Ada");
    expect(calls[0].url).toBe("http://server/api/v1/auth/me");
    expect(calls[0].init?.method).toBe("PATCH");
    expect(JSON.parse(calls[0].init?.body as string)).toEqual({ name: "Ada" });
  });
});

describe("contextRuleExamples", () => {
  it("asks for a voice rule's examples by profile and id", async () => {
    const calls: string[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (url: string) => {
        calls.push(url);
        return new Response(JSON.stringify({ examples: null, scanned: 12, complete: true }), {
          status: 200,
        });
      }) as typeof fetch,
    });
    const out = await client.contextRuleExamples("ws", "p", { profile: "docs", id: "ab12" });
    expect(calls[0]).toBe("http://server/api/v1/ws/p/context/deck/examples?profile=docs&id=ab12");
    expect(out).toEqual({ examples: [], scanned: 12, complete: true });
  });
});

describe("feedDecisions", () => {
  it("reads a review entry's decisions as deck cards", async () => {
    const calls: string[] = [];
    const client = createBowrainClient({
      baseUrl: "http://server",
      getAccessToken: () => "t",
      fetch: (async (url: string) => {
        calls.push(url);
        return new Response(
          JSON.stringify({
            locale: "nb",
            bulk_approved: 0,
            decisions: [
              {
                card: {
                  block_id: "b1",
                  item_name: "i18n/en.json",
                  key: "greeting",
                  collection_id: "",
                  locale: "nb",
                  source_runs: [{ text: "Hello" }],
                  target_runs: [{ text: "Hei" }],
                  source_revision: "r:s",
                  target_revision: "r:t",
                  notes: [],
                  findings_count: 0,
                  failing_findings_count: 0,
                  own_work: false,
                  group: "fast",
                  reason: "",
                },
                decision: "approved",
                at: "2026-11-17T10:00:00Z",
                current: true,
                can_undo: true,
              },
            ],
          }),
          { status: 200 },
        );
      }) as typeof fetch,
    });
    const out = await client.feedDecisions("ws", "p", "rvw_1");
    expect(calls[0]).toBe("http://server/api/v1/ws/p/feed/rvw_1/decisions");
    expect(out.decisions[0]).toMatchObject({
      decision: "approved",
      canUndo: true,
      card: { blockId: "b1", targetRevision: "r:t", target: [{ text: "Hei" }] },
    });
  });
});
