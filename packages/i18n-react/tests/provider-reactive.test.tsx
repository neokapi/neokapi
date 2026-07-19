// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { NeokapiProvider, setTranslations, __t } from "../src/runtime/index.ts";

// React's act() needs this flag to drive effects/renders synchronously.
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// A component exactly as the plugin emits it: a non-reactive __t lookup
// (reads the dict at render time, no subscription of its own).
function Greeting() {
  return <p>{__t("h1", "Hello")}</p>;
}

describe("<NeokapiProvider> — reactive locale switch", () => {
  beforeEach(() => {
    setTranslations("", {}); // reset the module-global store between tests
  });

  it("re-renders plugin __t lookups when the locale switches at runtime", () => {
    const container = document.createElement("div");
    const root = createRoot(container);

    act(() => {
      root.render(
        <NeokapiProvider>
          <Greeting />
        </NeokapiProvider>,
      );
    });
    expect(container.textContent).toBe("Hello"); // source fallback before any dict loads

    act(() => {
      setTranslations("ja", { h1: "こんにちは" });
    });
    // The provider subscribed and re-keyed on the new locale, remounting the
    // subtree so the non-reactive __t lookup re-reads the active dictionary.
    expect(container.textContent).toBe("こんにちは");

    act(() => root.unmount());
  });

  it("a bare __t component without the provider does NOT react to a later switch", () => {
    const container = document.createElement("div");
    const root = createRoot(container);

    act(() => root.render(<Greeting />));
    expect(container.textContent).toBe("Hello");

    act(() => setTranslations("ja", { h1: "こんにちは" }));
    // No subscription → no re-render → still the first-render value. This is the
    // footgun NeokapiProvider exists to remove.
    expect(container.textContent).toBe("Hello");

    act(() => root.unmount());
  });
});
