// A context decision whose answer is lost is sent again, and a repeat refused
// as no longer pending is the first attempt's landing, not a failure.
import { ApiError, NetworkError, type ContextDecision } from "@neokapi/bowrain-client";

import { sendContextDecision } from "../lib/contextDeck";

const decision = {
  item: { kind: "concept", id: "c1" },
  revision: "r1",
  action: "keep",
} as unknown as ContextDecision;

const noWait = async () => undefined;

/** A client whose answers to decideContext come from the list, in order. */
function client(answers: (() => Promise<unknown>)[]) {
  let calls = 0;
  return {
    get calls() {
      return calls;
    },
    decideContext: () => answers[calls++](),
  } as never as { calls: number; decideContext: () => Promise<never> };
}

const lost = () =>
  Promise.reject(new NetworkError("fetch failed: The network connection was lost."));
const notPending = () =>
  Promise.reject(new ApiError(409, "it waits no longer", { code: "not_pending" }));
const kept = () => Promise.resolve({ outcome: "kept" });

describe("sendContextDecision", () => {
  it("takes a repeat refused as no longer pending, after a lost answer, as landed", async () => {
    const c = client([lost, notPending]);
    await expect(sendContextDecision(c, "acme", "p1", decision, noWait)).resolves.toEqual({
      outcome: "landed",
    });
    expect(c.calls).toBe(2);
  });

  it("gives the server's answer when a repeat lands", async () => {
    const c = client([lost, kept]);
    await expect(sendContextDecision(c, "acme", "p1", decision, noWait)).resolves.toEqual({
      outcome: "kept",
    });
  });

  it("reports a no-longer-pending refusal that follows no lost answer", async () => {
    const c = client([notPending]);
    await expect(sendContextDecision(c, "acme", "p1", decision, noWait)).rejects.toBeInstanceOf(
      ApiError,
    );
    expect(c.calls).toBe(1);
  });

  it("gives up after three attempts with no answer", async () => {
    const c = client([lost, lost, lost]);
    await expect(sendContextDecision(c, "acme", "p1", decision, noWait)).rejects.toBeInstanceOf(
      NetworkError,
    );
    expect(c.calls).toBe(3);
  });
});
