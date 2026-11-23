// A context decision whose answer is lost is sent again. A repeat refused as
// no longer pending is the first attempt's landing only when the feed shows
// the reader made the same decision; otherwise it names who decided and how.
import {
  ApiError,
  NetworkError,
  type ContextDecision,
  type FeedEntry,
} from "@neokapi/bowrain-client";

import { feedDecision, sendContextDecision } from "../lib/contextDeck";

const keep = {
  item: { kind: "op", id: "op1" },
  revision: "op1",
  action: "keep",
} as ContextDecision;

const noWait = async () => undefined;

function entry(over: Partial<FeedEntry>): FeedEntry {
  return {
    id: "e1",
    kind: "context_change",
    at: new Date().toISOString(),
    actor: { id: "u1", kind: "person", name: "Ada", viewer: true },
    counts: { keep: 1, attest: 1, term_rule: 2 },
    link: { screen: "context_deck", mode: "replay", items: ["op1"] },
    ...over,
  } as FeedEntry;
}

/** A client whose decideContext answers come from the list, in order, over a feed. */
function client(answers: (() => Promise<unknown>)[], entries: FeedEntry[] = []) {
  let calls = 0;
  return {
    get calls() {
      return calls;
    },
    decideContext: () => answers[calls++](),
    feed: async () => ({ entries }),
  } as never as {
    calls: number;
    decideContext: () => Promise<never>;
    feed: () => Promise<never>;
  };
}

const lost = () =>
  Promise.reject(new NetworkError("fetch failed: The network connection was lost."));
const notPending = () =>
  Promise.reject(new ApiError(409, "the item is not waiting for review", { code: "not_pending" }));
const kept = () => Promise.resolve({ outcome: "kept" });

describe("sendContextDecision", () => {
  it("counts a repeat as landed when the feed shows the reader made the same decision", async () => {
    const c = client([lost, notPending], [entry({})]);
    await expect(sendContextDecision(c, "acme", "p1", keep, noWait)).resolves.toEqual({
      outcome: "landed",
    });
    expect(c.calls).toBe(2);
  });

  it("names a teammate who decided the item first, with what they did", async () => {
    const theirs = entry({
      actor: { id: "u2", kind: "person", name: "Grace" },
      counts: { drop: 1, term_rule: 1 },
    });
    const c = client([lost, notPending], [theirs]);
    await expect(sendContextDecision(c, "acme", "p1", keep, noWait)).resolves.toEqual({
      outcome: "decided_by",
      name: "Grace",
      action: "reject",
    });
  });

  it("does not take the reader's own opposite decision as this one landing", async () => {
    const c = client([lost, notPending], [entry({ counts: { drop: 1 } })]);
    await expect(sendContextDecision(c, "acme", "p1", keep, noWait)).resolves.toMatchObject({
      outcome: "decided_by",
      action: "reject",
    });
  });

  it("says the item is gone when the feed does not name it", async () => {
    const c = client(
      [lost, notPending],
      [entry({ link: { screen: "context_deck", items: ["other"] } })],
    );
    await expect(sendContextDecision(c, "acme", "p1", keep, noWait)).resolves.toEqual({
      outcome: "gone",
    });
  });

  it("gives the server's answer when a repeat lands", async () => {
    const c = client([lost, kept]);
    await expect(sendContextDecision(c, "acme", "p1", keep, noWait)).resolves.toEqual({
      outcome: "kept",
    });
  });

  it("reports a no-longer-pending refusal that follows no lost answer", async () => {
    const c = client([notPending]);
    await expect(sendContextDecision(c, "acme", "p1", keep, noWait)).rejects.toBeInstanceOf(
      ApiError,
    );
    expect(c.calls).toBe(1);
  });

  it("gives up after three attempts with no answer", async () => {
    const c = client([lost, lost, lost]);
    await expect(sendContextDecision(c, "acme", "p1", keep, noWait)).rejects.toBeInstanceOf(
      NetworkError,
    );
    expect(c.calls).toBe(3);
  });
});

describe("feedDecision", () => {
  it("reads an agent's change made for the reader as the reader's", () => {
    const forReader = entry({
      actor: { id: "a1", kind: "agent", name: "codex" },
      on_behalf_of: { id: "u1", name: "Ada", viewer: true },
    });
    expect(feedDecision([forReader], "op1")).toEqual({ mine: true, name: "Ada", action: "keep" });
  });

  it("reads an entry with null counts, link or actor without throwing", () => {
    const nulls = [
      entry({ counts: null as never }),
      entry({ link: null as never }),
      entry({ actor: null as never, counts: { keep: 1 } }),
    ];
    expect(() => feedDecision(nulls, "op1")).not.toThrow();
    expect(feedDecision([entry({ counts: null as never })], "op1")).toBeUndefined();
    expect(feedDecision([entry({ actor: null as never, counts: { keep: 1 } })], "op1")).toEqual({
      mine: false,
      name: "",
      action: "keep",
    });
  });

  it("resends past a feed entry with null counts", async () => {
    const c = client([lost, notPending], [entry({ counts: null as never }), entry({})]);
    await expect(sendContextDecision(c, "acme", "p1", keep, noWait)).resolves.toEqual({
      outcome: "landed",
    });
  });
});
