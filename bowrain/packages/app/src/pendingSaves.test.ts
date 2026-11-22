import { afterEach, beforeEach, describe, it, expect, vi } from "vite-plus/test";
import {
  HELD_EDIT_LIMIT,
  HELD_EDIT_TTL_MS,
  HeldEditError,
  discardAllPendingSaves,
  discardPendingEdit,
  flushPendingSaves,
  hasPendingSaves,
  pendingEdits,
  recordPendingEdit,
  savePendingEdit,
  setPendingSavesServer,
  setPendingSavesSession,
  subscribePendingSaves,
  waitingEditCount,
} from "./pendingSaves";

const alex = { userId: "alex", workspace: "acme" };

beforeEach(() => {
  setPendingSavesServer("https://bowrain.test");
  setPendingSavesSession(alex);
});
afterEach(() => {
  discardAllPendingSaves();
  vi.useRealTimers();
});

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

  it("records nothing without a session", () => {
    setPendingSavesSession(null);
    record("a", async () => {});
    expect(hasPendingSaves()).toBe(false);
  });
});

describe("pending saves across sessions", () => {
  it("the same user back in the same workspace sees the edit and saves it", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    record("a", send);
    setPendingSavesSession(null);
    expect(pendingEdits()).toEqual([]);
    expect(waitingEditCount()).toBe(1);
    setPendingSavesSession(alex);
    expect(pendingEdits().map((e) => e.key)).toEqual(["a"]);
    expect(await flushPendingSaves(1_000)).toEqual({ ok: true });
    expect(send).toHaveBeenCalledTimes(1);
  });

  it("a different user sees nothing of it and sends nothing", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    record("a", send);
    setPendingSavesSession({ userId: "blake", workspace: "acme" });
    expect(pendingEdits()).toEqual([]);
    expect(hasPendingSaves()).toBe(false);
    expect(waitingEditCount()).toBe(0);
    expect(await flushPendingSaves(1_000)).toEqual({ ok: true });
    await expect(savePendingEdit("a")).rejects.toBeInstanceOf(HeldEditError);
    discardPendingEdit("a");
    expect(send).not.toHaveBeenCalled();
    // Kept for its own user.
    setPendingSavesSession(alex);
    expect(pendingEdits().map((e) => e.key)).toEqual(["a"]);
  });

  it("the same user in another workspace, or on another server, sends nothing", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    record("a", send);
    setPendingSavesSession({ userId: "alex", workspace: "other" });
    expect(await flushPendingSaves(1_000)).toEqual({ ok: true });
    setPendingSavesSession(alex);
    setPendingSavesServer("https://elsewhere.test");
    expect(await flushPendingSaves(1_000)).toEqual({ ok: true });
    expect(send).not.toHaveBeenCalled();
  });

  it("without a session a flush sends nothing and counts what waits", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    record("a", send);
    setPendingSavesSession(null);
    expect(hasPendingSaves()).toBe(true);
    expect(await flushPendingSaves(1_000)).toEqual({
      ok: false,
      timedOut: false,
      errors: [],
      held: 1,
    });
    expect(send).not.toHaveBeenCalled();
  });

  it("keeps another session's edit for a bounded time", () => {
    vi.useFakeTimers();
    record("a", async () => {});
    setPendingSavesSession({ userId: "blake", workspace: "acme" });
    vi.advanceTimersByTime(HELD_EDIT_TTL_MS + 1);
    record("b", async () => {});
    setPendingSavesSession(alex);
    expect(pendingEdits()).toEqual([]);
  });

  it("drops the oldest of another session's edits past the limit", () => {
    for (let i = 0; i < HELD_EDIT_LIMIT; i++) record(`a${i}`, async () => {});
    setPendingSavesSession({ userId: "blake", workspace: "acme" });
    record("b", async () => {});
    setPendingSavesSession(alex);
    const keys = pendingEdits().map((e) => e.key);
    expect(keys).toHaveLength(HELD_EDIT_LIMIT - 1);
    expect(keys).not.toContain("a0");
  });
});
