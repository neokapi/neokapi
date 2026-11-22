import { afterEach, beforeEach, describe, it, expect, vi } from "vite-plus/test";
import {
  HeldEditError,
  discardAllPendingSaves,
  discardOwnPendingSaves,
  discardPendingEdit,
  flushPendingSaves,
  hasPendingSaves,
  pendingEdits,
  recordPendingEdit,
  savePendingEdit,
  normalizeServer,
  setPendingSavesServer,
  setPendingSavesUser,
  subscribePendingSaves,
  waitingEditCount,
} from "./pendingSaves";

beforeEach(() => {
  setPendingSavesServer("https://bowrain.test");
  setPendingSavesUser("alex");
});
afterEach(() => {
  discardAllPendingSaves();
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
    setPendingSavesUser(null);
    record("a", async () => {});
    expect(hasPendingSaves()).toBe(false);
  });
});

describe("pending saves across sessions", () => {
  it("sends an edit to its workspace when the person has switched to another", async () => {
    // The editor's unmount asks for the save and the layout switches
    // workspace before the send runs, as switching workspace does.
    const send = vi.fn().mockResolvedValue(undefined);
    record("acme/flow-1", send);
    const saved = savePendingEdit("acme/flow-1");
    setPendingSavesUser(null);
    setPendingSavesUser("alex");
    await saved;
    expect(send).toHaveBeenCalledTimes(1);
    expect(hasPendingSaves()).toBe(false);
  });

  it("the same user signing back in sees the edit and saves it", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    record("a", send);
    setPendingSavesUser(null);
    expect(pendingEdits()).toEqual([]);
    expect(waitingEditCount()).toBe(1);
    expect(hasPendingSaves()).toBe(true);
    setPendingSavesUser("alex");
    expect(pendingEdits().map((e) => e.key)).toEqual(["a"]);
    expect(await flushPendingSaves(1_000)).toEqual({ ok: true });
    expect(send).toHaveBeenCalledTimes(1);
  });

  it("a different user sees nothing of it and sends nothing", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    record("a", send);
    setPendingSavesUser("blake");
    expect(pendingEdits()).toEqual([]);
    expect(hasPendingSaves()).toBe(false);
    expect(waitingEditCount()).toBe(0);
    expect(await flushPendingSaves(1_000)).toEqual({ ok: true });
    await savePendingEdit("a");
    discardPendingEdit("a");
    discardOwnPendingSaves();
    expect(send).not.toHaveBeenCalled();
    // Kept for its own user.
    setPendingSavesUser("alex");
    expect(pendingEdits().map((e) => e.key)).toEqual(["a"]);
  });

  it("two users' edits to the same flow are kept apart", async () => {
    const alexSend = vi.fn().mockResolvedValue(undefined);
    record("a", alexSend);
    setPendingSavesUser("blake");
    const blakeSend = vi.fn().mockResolvedValue(undefined);
    record("a", blakeSend);
    await flushPendingSaves(1_000);
    expect(blakeSend).toHaveBeenCalledTimes(1);
    expect(alexSend).not.toHaveBeenCalled();
    setPendingSavesUser("alex");
    expect(pendingEdits().map((e) => e.key)).toEqual(["a"]);
  });

  it("another server sends nothing, and the address is compared normalized", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    record("a", send);
    setPendingSavesServer("https://elsewhere.test");
    expect(await flushPendingSaves(1_000)).toEqual({ ok: true });
    expect(send).not.toHaveBeenCalled();
    setPendingSavesServer("HTTPS://Bowrain.TEST/");
    expect(pendingEdits().map((e) => e.key)).toEqual(["a"]);
    expect(normalizeServer("HTTPS://Bowrain.TEST:8443/base/")).toBe(
      "https://bowrain.test:8443/base",
    );
  });

  it("without a session a flush sends nothing and counts what waits", async () => {
    const send = vi.fn().mockResolvedValue(undefined);
    record("a", send);
    setPendingSavesUser(null);
    expect(await flushPendingSaves(1_000)).toEqual({
      ok: false,
      timedOut: false,
      errors: [],
      held: 1,
    });
    expect(send).not.toHaveBeenCalled();
  });

  it("never drops an edit on its own, however long its user is away", () => {
    vi.useFakeTimers();
    for (let i = 0; i < 80; i++) record(`a${i}`, async () => {});
    setPendingSavesUser("blake");
    vi.advanceTimersByTime(7 * 24 * 60 * 60 * 1000);
    for (let i = 0; i < 80; i++) record(`b${i}`, async () => {});
    setPendingSavesUser("alex");
    expect(pendingEdits()).toHaveLength(80);
    vi.useRealTimers();
  });

  it("signing out discards the user's own edits in every workspace, and no one else's", () => {
    record("acme/a", async () => {});
    record("other/b", async () => {});
    setPendingSavesUser("blake");
    record("acme/c", async () => {});
    setPendingSavesUser("alex");
    discardOwnPendingSaves();
    expect(hasPendingSaves()).toBe(false);
    setPendingSavesUser("blake");
    expect(pendingEdits().map((e) => e.key)).toEqual(["acme/c"]);
  });
});
