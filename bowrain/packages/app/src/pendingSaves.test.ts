import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import {
  discardAllPendingSaves,
  discardPendingEdit,
  flushPendingSaves,
  hasPendingSaves,
  pendingEdits,
  recordPendingEdit,
  savePendingEdit,
  subscribePendingSaves,
} from "./pendingSaves";

afterEach(() => discardAllPendingSaves());

function record(key: string, send: () => Promise<void>) {
  recordPendingEdit({ key, label: key, scope: "test", send });
}

describe("pending saves", () => {
  it("has nothing to flush when nothing is recorded", async () => {
    expect(hasPendingSaves()).toBe(false);
    expect(await flushPendingSaves(1_000)).toEqual({ ok: true });
  });

  it("flushes every recorded edit and reports each failure by name", async () => {
    const saved = vi.fn().mockResolvedValue(undefined);
    record("a", saved);
    record("b", () => Promise.reject(new Error("conflict")));
    const result = await flushPendingSaves(1_000);
    expect(saved).toHaveBeenCalledTimes(1);
    expect(result).toMatchObject({ ok: false, timedOut: false });
    const errors = (result as { errors: { key: string; error: unknown }[] }).errors;
    expect(errors.map((e) => e.key)).toEqual(["b"]);
    expect(String(errors[0].error)).toContain("conflict");
    expect(pendingEdits().map((e) => [e.key, e.state])).toEqual([["b", "failed"]]);
  });

  it("stops waiting after the bound and says so", async () => {
    record("a", () => new Promise<void>(() => {}));
    expect(await flushPendingSaves(20)).toEqual({ ok: false, timedOut: true, errors: [] });
    expect(hasPendingSaves()).toBe(true);
  });

  it("keeps a failed edit until a save of the same key succeeds", async () => {
    const send = vi.fn().mockRejectedValueOnce(new Error("down")).mockResolvedValue(undefined);
    record("a", send);
    await expect(savePendingEdit("a")).rejects.toThrow("down");
    record("b", async () => {});
    await savePendingEdit("b");
    expect(pendingEdits().map((e) => e.key)).toEqual(["a"]);
    await savePendingEdit("a");
    expect(hasPendingSaves()).toBe(false);
  });

  it("keeps an edit made while its save was in flight", async () => {
    let finish!: () => void;
    record("a", () => new Promise<void>((r) => (finish = r)));
    const first = savePendingEdit("a");
    await Promise.resolve();
    const second = vi.fn().mockResolvedValue(undefined);
    record("a", second);
    finish();
    await first;
    expect(hasPendingSaves()).toBe(true);
    await savePendingEdit("a");
    expect(second).toHaveBeenCalledTimes(1);
    expect(hasPendingSaves()).toBe(false);
  });

  it("drops a discarded edit and tells subscribers", () => {
    const listener = vi.fn();
    const unsubscribe = subscribePendingSaves(listener);
    record("a", async () => {});
    expect(listener).toHaveBeenCalledTimes(1);
    discardPendingEdit("a");
    expect(listener).toHaveBeenCalledTimes(2);
    expect(hasPendingSaves()).toBe(false);
    unsubscribe();
  });
});
