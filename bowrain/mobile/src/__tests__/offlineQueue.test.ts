import { createBowrainClient, type DeckCard } from "@neokapi/bowrain-client";
import {
  dehydrate,
  hydrate,
  MutationObserver,
  onlineManager,
  type QueryClient,
} from "@tanstack/react-query";

import {
  DECISION_KEY,
  GRACE_MS,
  holdDecision,
  LOG_KEY,
  NOTICES_KEY,
  planUndo,
  queuedDecisions,
  readLog,
  releaseDue,
  resendLost,
  setDecisionClient,
  undoDecision,
  type DecisionNotice,
  type DecisionVars,
  type LoggedDecision,
  type WithdrawVars,
} from "../lib/decisions";
import { createDecisionTestClient } from "./testProviders";

const clients: QueryClient[] = [];
function createQueryClient(): QueryClient {
  const qc = createDecisionTestClient();
  clients.push(qc);
  return qc;
}

const card = (n: number): DeckCard => ({
  key: `b${n}:nb`,
  blockId: `b${n}`,
  item: "i18n/en.json",
  locale: "nb",
  collectionId: "",
  source: [{ text: `Source ${n}` }],
  target: [{ text: `Mål ${n}` }],
  targetRevision: `r:000000000000000${n}`,
  sourceRevision: "r:aaaaaaaaaaaaaaaa",
  group: "fast",
  notes: [],
  findingsCount: 0,
  failingFindingsCount: 0,
  ownWork: false,
});

const scope = { workspace: "ws", project: "p" };

function vars(n: number, kind: DecisionVars["kind"] = "approve"): DecisionVars {
  return { id: `d${n}`, scope, card: card(n), kind, madeAt: n };
}

/** A server double at the HTTP boundary: records each change set, answers with a result. */
function server(
  answer: (set: { ops: { if_match: string }[] }) => { status: number; body: unknown },
) {
  const sent: { ops: { op: string; at: { block: string }; if_match: string }[] }[] = [];
  const fetch = (async (_url: string, init: RequestInit) => {
    const set = JSON.parse(String(init.body));
    sent.push(set);
    const { status, body } = answer(set);
    return new Response(JSON.stringify(body), { status });
  }) as typeof globalThis.fetch;
  return {
    sent,
    client: createBowrainClient({ baseUrl: "http://server", getAccessToken: () => "t", fetch }),
  };
}

const applied = () => ({
  status: 200,
  body: {
    schema: "kapi.change-result/v1",
    status: "applied",
    record: "rec",
    docs: [],
    ops: [{ i: 0, op: "decide", status: "applied", after: "r:1111111111111111" }],
  },
});

function queue(qc: QueryClient, v: DecisionVars) {
  void new MutationObserver<unknown, unknown, DecisionVars>(qc, { mutationKey: DECISION_KEY })
    .mutate(v)
    .catch(() => undefined);
}

const flush = () => new Promise((r) => setTimeout(r, 0));

describe("the offline decision queue", () => {
  afterEach(() => {
    for (const qc of clients.splice(0)) {
      qc.clear();
    }
    onlineManager.setOnline(true);
    setDecisionClient(null);
  });

  it("holds decisions offline, survives a restart, and sends them in order", async () => {
    const s = server(applied);
    setDecisionClient(s.client);
    onlineManager.setOnline(false);

    const before = createQueryClient();
    holdDecision(before, made(1));
    holdDecision(before, made(2, "reject"));
    releaseDue(before, now + 10 * GRACE_MS);
    await flush();
    expect(queuedDecisions(before).map((v) => v.id)).toEqual(["d1", "d2"]);
    expect(s.sent).toHaveLength(0);

    // A restart: the dehydrated cache is what the persister writes on the device.
    const persisted = JSON.parse(JSON.stringify(dehydrate(before)));
    const after = createQueryClient();
    hydrate(after, persisted);
    onlineManager.setOnline(true);
    await after.resumePausedMutations();
    await flush();

    expect(s.sent.map((set) => set.ops[0].at.block)).toEqual(["b1", "b2"]);
    expect(s.sent[0].ops[0].if_match).toBe("r:0000000000000001");
  });

  it("reports a stale decision so the deck offers the new version", async () => {
    const s = server(() => ({
      status: 409,
      body: {
        schema: "kapi.change-result/v1",
        status: "refused",
        record: null,
        docs: [],
        ops: [
          {
            i: 0,
            op: "decide",
            status: "refused",
            error: { code: "stale", field: "if_match", message: "the translation moved" },
            current: { rev: "r:2222222222222222", text: "Nytt mål" },
          },
        ],
      },
    }));
    setDecisionClient(s.client);
    const qc = createQueryClient();
    queue(qc, vars(3));
    await flush();
    await flush();
    const notices = qc.getQueryData<DecisionNotice[]>(NOTICES_KEY) ?? [];
    expect(notices.map((n) => [n.kind, n.card.key])).toEqual([["stale", "b3:nb"]]);
  });

  /**
   * A server that applies the first change set and loses its answer on the
   * way back, as Expo's fetch reports a dropped connection, then answers
   * every later one with `then`.
   */
  function losesFirstAnswer(then: () => { status: number; body: unknown }) {
    const sent: unknown[] = [];
    const fetch = (async (_url: string, init: RequestInit) => {
      sent.push(JSON.parse(String(init.body)));
      if (sent.length === 1) throw new Error("fetch failed: The network connection was lost.");
      const { status, body } = then();
      return new Response(JSON.stringify(body), { status });
    }) as typeof globalThis.fetch;
    return {
      sent,
      client: createBowrainClient({ baseUrl: "http://server", getAccessToken: () => "t", fetch }),
    };
  }

  async function settled(qc: QueryClient, id: string) {
    for (let i = 0; i < 40; i++) {
      if (readLog(qc).find((x) => x.vars.id === id)?.phase === "done") return;
      await new Promise((r) => setTimeout(r, 100));
    }
    throw new Error(`decision ${id} did not settle`);
  }

  it("sends a decision again when its answer is lost, and shows it done once", async () => {
    // The repeat of a decision that landed answers "unchanged" within an applied set.
    const s = losesFirstAnswer(() => ({
      status: 200,
      body: {
        schema: "kapi.change-result/v1",
        status: "applied",
        record: null,
        docs: [],
        ops: [{ i: 0, op: "decide", status: "unchanged", after: "r:0000000000000004" }],
      },
    }));
    setDecisionClient(s.client);
    const qc = createQueryClient();
    holdDecision(qc, made(4));
    queue(qc, made(4));
    await settled(qc, "d4");
    expect(s.sent).toHaveLength(2);
    expect(readLog(qc).find((x) => x.vars.id === "d4")?.result?.outcome).toBe("applied");
    expect(qc.getQueryData<DecisionNotice[]>(NOTICES_KEY) ?? []).toEqual([]);
  });

  // A correction refused as stale: the edition moved. The app reads each block
  // the correction covers again and approves, with the person's reasons and
  // note, only the blocks whose translation is the correction run for run.
  const ph = (id: string, type = "x-var") =>
    ({ ph: { id, type, data: "{{n}}", equiv: "{{n}}" } }) as unknown as DeckCard["target"][number];
  const pc = (type: string) => [
    { pcOpen: { id: "1", type, data: "<b>" } },
    { text: "Lagre" },
    { pcClose: { id: "1", type, data: "</b>" } },
  ] as unknown as DeckCard["target"];
  const plural = (one: string) =>
    [{ plural: { pivot: "n", forms: { one: [{ text: one }], other: [{ text: "filer" }] } } }] as unknown as DeckCard["target"];
  const staleAnswer = {
    status: 409,
    body: {
      schema: "kapi.change-result/v1",
      status: "refused",
      record: null,
      docs: [],
      ops: [
        { i: 0, op: "set_content", status: "not_applied", blocked_by: 1 },
        {
          i: 1,
          op: "decide",
          status: "refused",
          error: { code: "stale", field: "if_match", message: "the edition moved" },
          current: { rev: "r:3333333333333333", text: "Rettet tekst" },
        },
      ],
    },
  };
  type Sent = {
    ops: { op: string; at: { block: string }; if_match: string; outcome?: string; reasons?: string[] }[];
    note?: string;
  };

  /**
   * A server double that answers a block's container with the translation it
   * holds now, and change sets in turn: stale first, then the given status
   * for each confirming decision. lose drops the first answer.
   */
  function staleServer(
    held: Record<string, DeckCard["target"]>,
    opts: { lose?: boolean; confirm?: "applied" | "unchanged" } = {},
  ) {
    const sent: Sent[] = [];
    let posts = 0;
    const fetch = (async (url: string, init: RequestInit) => {
      const container = /\/blocks\/[^/]+\/([^/]+)\/container/.exec(url);
      if (container) {
        const blockId = container[1];
        return new Response(
          JSON.stringify({
            kind: "window",
            title: "i18n/en.json",
            item_name: "i18n/en.json",
            locale: "nb",
            focus: blockId,
            units: [
              {
                block_id: blockId,
                item_name: "i18n/en.json",
                key: blockId,
                role: "text",
                source_runs: [{ text: "Source" }],
                target_runs: held[blockId] ?? [{ text: "Annen tekst" }],
                source_revision: "r:aaaaaaaaaaaaaaaa",
                target_revision: `r:held-${blockId}`,
              },
            ],
          }),
          { status: 200 },
        );
      }
      const set = JSON.parse(String(init.body)) as Sent;
      sent.push(set);
      posts++;
      if (opts.lose && posts === 1) throw new Error("fetch failed: The network connection was lost.");
      if (posts <= (opts.lose ? 2 : 1)) {
        return new Response(JSON.stringify(staleAnswer.body), { status: staleAnswer.status });
      }
      const ops = set.ops.map((op, i) => ({
        i,
        op: "decide",
        status: opts.confirm ?? "applied",
        at: { doc: "i18n/en.json", block: op.at.block, edition: "nb" },
        before: op.if_match,
        after: op.if_match,
      }));
      return new Response(
        JSON.stringify({ schema: "kapi.change-result/v1", status: "applied", record: null, docs: [], ops }),
        { status: 200 },
      );
    }) as typeof globalThis.fetch;
    return {
      sent,
      client: createBowrainClient({ baseUrl: "http://server", getAccessToken: () => "t", fetch }),
    };
  }

  const withCopy = (n: number): DeckCard => ({
    ...card(n),
    copies: [
      {
        item: "i18n/other.json",
        collectionId: "",
        blockId: `c${n}`,
        targetRevision: `r:c00000000000000${n}`,
        sourceRevision: "r:aaaaaaaaaaaaaaaa",
      },
    ],
  });

  async function correct(
    n: number,
    runs: DeckCard["target"],
    s: ReturnType<typeof staleServer>,
    c: DeckCard = card(n),
  ) {
    setDecisionClient(s.client);
    const qc = createQueryClient();
    const fixed: DecisionVars = {
      ...made(n, "fix"),
      card: c,
      runs,
      reasons: ["wrong_term"],
      note: "The product says lagre.",
    };
    holdDecision(qc, fixed);
    queue(qc, fixed);
    await settled(qc, `d${n}`);
    return readLog(qc).find((x) => x.vars.id === `d${n}`)?.result;
  }

  it("confirms once, with the person's reasons and note, when the content is the correction's", async () => {
    const runs = [{ text: "Rettet " }, ph("1")];
    const s = staleServer({ b5: runs }, { lose: true, confirm: "unchanged" });
    const result = await correct(5, runs, s);
    expect(result).toMatchObject({ outcome: "applied", revision: "r:held-b5" });
    const confirms = s.sent.filter((set) => set.ops.every((op) => op.op === "decide"));
    expect(confirms).toHaveLength(1);
    expect(confirms[0].ops).toEqual([
      expect.objectContaining({ outcome: "establish", if_match: "r:held-b5", reasons: ["wrong_term"] }),
    ]);
    expect(confirms[0].note).toBe("The product says lagre.");
  });

  it("does not confirm the same words with a placeholder that differs", async () => {
    const s = staleServer({ b6: [{ text: "Rettet " }, ph("1", "x-date")] });
    const result = await correct(6, [{ text: "Rettet " }, ph("1")], s);
    expect(result?.outcome).toBe("stale");
    expect(s.sent).toHaveLength(1);
  });

  it("does not confirm the same words with a paired code that differs", async () => {
    const s = staleServer({ b7: pc("link") });
    const result = await correct(7, pc("fmt"), s);
    expect(result?.outcome).toBe("stale");
    expect(s.sent).toHaveLength(1);
  });

  it("compares a plural form by form", async () => {
    const differs = staleServer({ b8: plural("fil") });
    expect((await correct(8, plural("fila"), differs))?.outcome).toBe("stale");
    const same = staleServer({ b9: plural("fil") });
    expect((await correct(9, plural("fil"), same))?.outcome).toBe("applied");
  });

  it("confirms each copy against its own revision, and says partial when one differs", async () => {
    const runs = [{ text: "Rettet tekst" }];
    const both = staleServer({ b10: runs, c10: runs });
    const all = await correct(10, runs, both, withCopy(10));
    expect(all).toMatchObject({ outcome: "applied", revision: "r:held-b10", copyRevisions: { c10: "r:held-c10" } });
    expect(both.sent[1].ops.map((op) => [op.at.block, op.if_match])).toEqual([
      ["b10", "r:held-b10"],
      ["c10", "r:held-c10"],
    ]);

    const one = staleServer({ b11: runs, c11: [{ text: "Noe annet" }] });
    const some = await correct(11, runs, one, withCopy(11));
    expect(some?.outcome).toBe("partial");
    expect(one.sent[1].ops.map((op) => op.at.block)).toEqual(["b11"]);
  });

  const entry = (qc: QueryClient, id: string): LoggedDecision => {
    const e = readLog(qc).find((x) => x.vars.id === id);
    if (!e) throw new Error(`no decision ${id}`);
    return e;
  };
  const now = Date.now();
  const made = (n: number, kind: DecisionVars["kind"] = "approve"): DecisionVars => ({
    ...vars(n, kind),
    madeAt: now + n,
  });

  it("holds a new decision on the device for the grace period", async () => {
    const s = server(applied);
    setDecisionClient(s.client);
    const qc = createQueryClient();
    holdDecision(qc, made(1));
    expect(releaseDue(qc, now + 1 + GRACE_MS - 1)).toBe(0);
    expect(queuedDecisions(qc).map((v) => v.id)).toEqual(["d1"]);
    expect(releaseDue(qc, now + 1 + GRACE_MS)).toBe(1);
    await flush();
    await flush();
    expect(s.sent.map((set) => set.ops[0].at.block)).toEqual(["b1"]);
    expect(entry(qc, "d1")).toMatchObject({
      phase: "done",
      result: { outcome: "applied", revision: "r:1111111111111111" },
    });
  });

  it("drops a held decision on undo, so nothing reaches the server", async () => {
    const s = server(applied);
    setDecisionClient(s.client);
    const qc = createQueryClient();
    holdDecision(qc, made(2));
    holdDecision(qc, made(3));
    expect(planUndo(qc, entry(qc, "d2"))).toEqual({ kind: "drop" });
    expect(undoDecision(qc, "d2", () => undefined)?.key).toBe("b2:nb");
    releaseDue(qc, now + 10 * GRACE_MS);
    await flush();
    await flush();
    expect(s.sent.map((set) => set.ops[0].at.block)).toEqual(["b3"]);
    expect(entry(qc, "d2").phase).toBe("undone");
  });

  it("sends every held decision when the app leaves the foreground", async () => {
    const s = server(applied);
    setDecisionClient(s.client);
    const qc = createQueryClient();
    holdDecision(qc, made(4));
    expect(releaseDue(qc, now, true)).toBe(1);
    await flush();
    expect(s.sent).toHaveLength(1);
  });

  it("drops a decision that waits offline in the queue", async () => {
    const s = server(applied);
    setDecisionClient(s.client);
    onlineManager.setOnline(false);
    const qc = createQueryClient();
    holdDecision(qc, made(5));
    holdDecision(qc, made(6));
    releaseDue(qc, now + 10 * GRACE_MS);
    await flush();
    expect(planUndo(qc, entry(qc, "d5"))).toEqual({ kind: "unqueue" });
    undoDecision(qc, "d5", () => undefined);
    onlineManager.setOnline(true);
    await qc.resumePausedMutations();
    await flush();
    expect(s.sent.map((set) => set.ops[0].at.block)).toEqual(["b6"]);
  });

  it("withdraws a sent decision with a new one, and keeps a landed fix", async () => {
    const s = server(applied);
    setDecisionClient(s.client);
    const qc = createQueryClient();
    holdDecision(qc, made(7));
    holdDecision(qc, { ...made(8, "fix"), runs: [{ text: "Rettet" }] });
    releaseDue(qc, now + 10 * GRACE_MS);
    await flush();
    await flush();
    await flush();
    expect(planUndo(qc, entry(qc, "d8"))).toBeNull();
    const withdrawn: WithdrawVars[] = [];
    undoDecision(qc, "d7", (w) => withdrawn.push(w));
    expect(withdrawn.map((w) => [w.id, w.revision])).toEqual([["d7", "r:1111111111111111"]]);
    expect(entry(qc, "d7").phase).toBe("undoing");
  });

  it("keeps held decisions across a restart and sends one that was in flight", async () => {
    const s = server(applied);
    setDecisionClient(s.client);
    const before = createQueryClient();
    holdDecision(before, made(9));
    // A decision handed to the queue whose mutation was not persisted.
    before.setQueryData<LoggedDecision[]>(LOG_KEY, (log = []) => [
      ...log,
      { vars: made(10), phase: "queued" },
    ]);
    const persisted = JSON.parse(JSON.stringify(dehydrate(before)));
    const after = createQueryClient();
    hydrate(after, persisted);
    expect(readLog(after).map((e) => [e.vars.id, e.phase])).toEqual([
      ["d9", "held"],
      ["d10", "queued"],
    ]);
    resendLost(after);
    releaseDue(after, now + 10 * GRACE_MS);
    await flush();
    await flush();
    expect(s.sent.map((set) => set.ops[0].at.block).sort()).toEqual(["b10", "b9"]);
  });
});
