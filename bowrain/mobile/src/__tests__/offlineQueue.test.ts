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
  NOTICES_KEY,
  planUndo,
  queuedDecisions,
  setDecisionClient,
  undoLast,
  type DecisionNotice,
  type DecisionVars,
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
    queue(before, vars(1));
    queue(before, vars(2, "reject"));
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

  it("undoes the last decision while it is still queued", async () => {
    const s = server(applied);
    setDecisionClient(s.client);
    onlineManager.setOnline(false);
    const qc = createQueryClient();
    queue(qc, vars(4));
    queue(qc, vars(5));
    await flush();
    expect(planUndo(qc)).toMatchObject({ kind: "unqueue", vars: { id: "d5" } });
    expect(undoLast(qc, () => undefined)?.key).toBe("b5:nb");
    onlineManager.setOnline(true);
    await qc.resumePausedMutations();
    await flush();
    expect(s.sent.map((set) => set.ops[0].at.block)).toEqual(["b4"]);
  });

  it("withdraws the last decision with a new one once it landed", async () => {
    const s = server(applied);
    setDecisionClient(s.client);
    const qc = createQueryClient();
    queue(qc, vars(6));
    await flush();
    await flush();
    const withdrawn: string[] = [];
    expect(planUndo(qc)).toMatchObject({ kind: "withdraw", revision: "r:1111111111111111" });
    undoLast(qc, (w) => withdrawn.push(w.revision));
    expect(withdrawn).toEqual(["r:1111111111111111"]);
  });
});
