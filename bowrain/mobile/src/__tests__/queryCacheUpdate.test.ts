import type { PersistedClient } from "@tanstack/react-query-persist-client";

import { forRunningUpdate } from "../lib/queryClient";

const client = (): PersistedClient => ({
  timestamp: 1,
  buster: "1",
  clientState: {
    queries: [{ queryKey: ["review-home"], queryHash: "h", state: {} } as never],
    mutations: [{ mutationKey: ["decide"], state: {} } as never],
  },
});

describe("persisted cache across updates", () => {
  it("keeps everything when the running update wrote it", () => {
    const c = client();
    expect(forRunningUpdate(c, "u1", "u1")).toBe(c);
  });

  it("drops the queries and keeps queued decisions after an update", () => {
    const out = forRunningUpdate(client(), "u1", "u2");
    expect(out?.clientState.queries).toEqual([]);
    expect(out?.clientState.mutations).toHaveLength(1);
  });

  it("treats a cache with no recorded update as written by another", () => {
    const out = forRunningUpdate(client(), null, "embedded");
    expect(out?.clientState.queries).toEqual([]);
    expect(out?.clientState.mutations).toHaveLength(1);
  });

  it("passes through an empty store", () => {
    expect(forRunningUpdate(undefined, null, "u1")).toBeUndefined();
  });
});
