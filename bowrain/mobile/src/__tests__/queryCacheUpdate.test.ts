import type { PersistedClient } from "@tanstack/react-query-persist-client";

import { forRunningUpdate } from "../lib/queryClient";

const query = (key: string) => ({ queryKey: [key], queryHash: key, state: {} }) as never;

const client = (updateId?: string): PersistedClient & { updateId?: string } => ({
  timestamp: 1,
  buster: "1",
  updateId,
  clientState: {
    queries: [query("review-home"), query("decision-log"), query("decision-notices")],
    mutations: [{ mutationKey: ["decide"], state: {} } as never],
  },
});

const keys = (c: PersistedClient | undefined) =>
  c?.clientState.queries.map((q) => String((q.queryKey as unknown[])[0]));

describe("persisted cache across updates", () => {
  it("keeps everything when the running update wrote it", () => {
    const out = forRunningUpdate(client("u1"), "u1");
    expect(keys(out)).toEqual(["review-home", "decision-log", "decision-notices"]);
    expect(out).not.toHaveProperty("updateId");
  });

  it("drops server responses but keeps the decision log, notices and queue after an update", () => {
    const out = forRunningUpdate(client("u1"), "u2");
    expect(keys(out)).toEqual(["decision-log", "decision-notices"]);
    expect(out?.clientState.mutations).toHaveLength(1);
  });

  it("treats a cache with no recorded update as written by another", () => {
    expect(keys(forRunningUpdate(client(), "embedded"))).toEqual([
      "decision-log",
      "decision-notices",
    ]);
  });

  it("passes through an empty store", () => {
    expect(forRunningUpdate(undefined, "u1")).toBeUndefined();
  });
});
