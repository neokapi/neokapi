// @vitest-environment jsdom
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTextTransition } from "@neokapi/ui-primitives/preview";

// useTextTransition drives the source→target swap. We use fake timers to assert
// the typewriter reveals progressively, that reduced motion shows the full text
// instantly, and that the crossfade cycle counter bumps on each change.

describe("useTextTransition", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("reveals text word-by-word for the typewriter effect", () => {
    const { result, rerender } = renderHook(
      ({ text }) => useTextTransition(text, { effect: "typewriter", speed: 10 }),
      { initialProps: { text: "alpha" } },
    );
    // First render of an unchanged value shows it instantly.
    expect(result.current.visible).toBe("alpha");

    // Changing the text starts a reveal from empty.
    rerender({ text: "one two three" });
    expect(result.current.visible).toBe("");
    expect(result.current.done).toBe(false);

    act(() => vi.advanceTimersByTime(10));
    expect(result.current.visible).toBe("one ");

    act(() => vi.advanceTimersByTime(10));
    expect(result.current.visible).toBe("one two ");

    act(() => vi.advanceTimersByTime(10));
    expect(result.current.visible).toBe("one two three");
    expect(result.current.done).toBe(true);
  });

  it("shows full text instantly under reduced motion", () => {
    const { result, rerender } = renderHook(
      ({ text }) => useTextTransition(text, { effect: "typewriter", reducedMotion: true }),
      { initialProps: { text: "a" } },
    );
    rerender({ text: "hello world" });
    expect(result.current.visible).toBe("hello world");
    expect(result.current.done).toBe(true);
  });

  it("bumps the crossfade cycle counter on each text change", () => {
    const { result, rerender } = renderHook(
      ({ text }) => useTextTransition(text, { effect: "crossfade" }),
      { initialProps: { text: "a" } },
    );
    const start = result.current.cycle;
    rerender({ text: "b" });
    expect(result.current.cycle).toBe(start + 1);
    expect(result.current.visible).toBe("b"); // crossfade is instant; CSS owns fade
    rerender({ text: "c" });
    expect(result.current.cycle).toBe(start + 2);
  });
});
