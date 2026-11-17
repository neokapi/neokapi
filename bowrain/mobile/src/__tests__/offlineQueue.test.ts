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
import { createQueryClient as create } from "../lib/queryClient";

const clients: QueryClient[] = [];
function createQueryClient(): QueryClient {
  const qc = create();
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
      qc.getMutationCache().clear();
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
