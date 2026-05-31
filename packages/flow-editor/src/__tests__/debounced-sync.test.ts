import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { createDebouncedSync } from "../debouncedSync";

/**
 * Debounced config sync for the StepConfigPanel (finding #51).
 *
 * The panel debounces config edits (300ms). The bug: when the panel unmounts or
 * closes before the timer fires, the last sub-300ms edit was DROPPED — the
 * cleanup only cleared the timer. The fix flushes the pending edit on cleanup.
 * The panel wires `flush()` into its React unmount cleanup, so these tests pin
 * the controller behavior that the panel relies on.
 */
describe("createDebouncedSync", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("emits once after the delay elapses", () => {
    const emit = vi.fn();
    const sync = createDebouncedSync(emit, 300);
    sync.schedule({ a: 1 });
    expect(emit).not.toHaveBeenCalled();
    vi.advanceTimersByTime(299);
    expect(emit).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(emit).toHaveBeenCalledExactlyOnceWith({ a: 1 });
  });

  it("coalesces rapid edits into a single trailing emit", () => {
    const emit = vi.fn();
    const sync = createDebouncedSync(emit, 300);
    sync.schedule({ n: 1 });
    vi.advanceTimersByTime(100);
    sync.schedule({ n: 2 });
    vi.advanceTimersByTime(100);
    sync.schedule({ n: 3 });
    vi.advanceTimersByTime(300);
    expect(emit).toHaveBeenCalledExactlyOnceWith({ n: 3 });
  });

  it("flush() emits the last pending edit immediately (the unmount case)", () => {
    const emit = vi.fn();
    const sync = createDebouncedSync(emit, 300);
    sync.schedule({ targetLang: "fr" });
    // Unmount before the 300ms timer fires.
    expect(sync.hasPending()).toBe(true);
    sync.flush();
    expect(emit).toHaveBeenCalledExactlyOnceWith({ targetLang: "fr" });
    expect(sync.hasPending()).toBe(false);
    // The original timer must not double-fire afterwards.
    vi.advanceTimersByTime(300);
    expect(emit).toHaveBeenCalledTimes(1);
  });

  it("flush() is a no-op when nothing is pending", () => {
    const emit = vi.fn();
    const sync = createDebouncedSync(emit, 300);
    sync.flush();
    expect(emit).not.toHaveBeenCalled();
  });

  it("flush() after a normal emit does not re-emit", () => {
    const emit = vi.fn();
    const sync = createDebouncedSync(emit, 300);
    sync.schedule({ a: 1 });
    vi.advanceTimersByTime(300);
    expect(emit).toHaveBeenCalledTimes(1);
    sync.flush();
    expect(emit).toHaveBeenCalledTimes(1);
  });

  it("cancel() drops the pending edit without emitting", () => {
    const emit = vi.fn();
    const sync = createDebouncedSync(emit, 300);
    sync.schedule({ a: 1 });
    sync.cancel();
    expect(sync.hasPending()).toBe(false);
    vi.advanceTimersByTime(300);
    expect(emit).not.toHaveBeenCalled();
  });
});
