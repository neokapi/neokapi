import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import {
  flushPendingSaves,
  hasPendingSaves,
  registerPendingSave,
  subscribePendingSaves,
} from "./pendingSaves";

const cleanups: Array<() => void> = [];
afterEach(() => {
  while (cleanups.length) cleanups.pop()!();
});

function register(pending: boolean, flush: () => Promise<void>) {
  cleanups.push(registerPendingSave({ pending: () => pending, flush }));
}

describe("pending saves", () => {
  it("has nothing to flush when no surface waits", async () => {
    register(false, () => Promise.reject(new Error("never called")));
    expect(hasPendingSaves()).toBe(false);
    expect(await flushPendingSaves(1_000)).toEqual({ ok: true });
  });

  it("flushes every waiting surface and reports each failure", async () => {
    const saved = vi.fn().mockResolvedValue(undefined);
    register(true, saved);
    register(true, () => Promise.reject(new Error("conflict")));
    expect(hasPendingSaves()).toBe(true);
    const result = await flushPendingSaves(1_000);
    expect(saved).toHaveBeenCalledTimes(1);
    expect(result).toMatchObject({ ok: false, timedOut: false });
    expect(String((result as { errors: unknown[] }).errors[0])).toContain("conflict");
  });

  it("stops waiting after the bound and says so", async () => {
    register(true, () => new Promise<void>(() => {}));
    expect(await flushPendingSaves(20)).toEqual({ ok: false, timedOut: true, errors: [] });
  });

  it("tells subscribers when a surface registers or leaves", () => {
    const listener = vi.fn();
    cleanups.push(subscribePendingSaves(listener));
    const unregister = registerPendingSave({ pending: () => true, flush: async () => {} });
    expect(listener).toHaveBeenCalledTimes(1);
    unregister();
    expect(listener).toHaveBeenCalledTimes(2);
  });
});
