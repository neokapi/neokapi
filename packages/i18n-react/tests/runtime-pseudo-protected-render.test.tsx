// @vitest-environment jsdom
/**
 * The rendered half of runtime pseudo mode over prose holding a code span.
 *
 * `__tx` clones the `<code>` with the message's inner content as its children,
 * so what reaches the DOM is the accented sentence around a span still reading
 * as the command the author wrote. A `<kbd>` behaves the same way, which is
 * what keeps a keyboard hint pressable while the page is in pseudo mode.
 */
import { describe, it, expect, beforeEach } from "vitest";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { setTranslations, __tx } from "../src/runtime/index.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { setPseudoMode } from "../src/runtime/pseudo.ts";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const CODE_SOURCE = "Run {=m0}kapi check --ship{/=m0} before release.";
const CODE_HASH = hashKey(CODE_SOURCE, "p");

const KBD_SOURCE = "Press {=m0}Ctrl{/=m0} to stop.";
const KBD_HASH = hashKey(KBD_SOURCE, "p");

/** The call site the transform emits for a paragraph holding a code span. */
function CodeParagraph() {
  return (
    <p>
      {__tx(CODE_HASH, CODE_SOURCE, { "=m0": <code>kapi check --ship</code> }, undefined, {
        "=m0": "no",
      })}
    </p>
  );
}

function KbdParagraph() {
  return (
    <p>{__tx(KBD_HASH, KBD_SOURCE, { "=m0": <kbd>Ctrl</kbd> }, undefined, { "=m0": "no" })}</p>
  );
}

/** The same code paragraph as a build that predates the marker answers. */
function UnmarkedParagraph() {
  return <p>{__tx(CODE_HASH, CODE_SOURCE, { "=m0": <code>kapi check --ship</code> })}</p>;
}

function render(node: React.ReactElement): { container: HTMLDivElement; unmount: () => void } {
  const container = document.createElement("div");
  const root = createRoot(container);
  act(() => {
    root.render(node);
  });
  return { container, unmount: () => act(() => root.unmount()) };
}

describe("pseudo mode over a paragraph holding a code span", () => {
  beforeEach(() => {
    setPseudoMode(null);
    setTranslations("", {});
  });

  it("renders the source sentence untouched when pseudo mode is off", () => {
    const { container, unmount } = render(<CodeParagraph />);
    expect(container.querySelector("p")?.textContent).toBe("Run kapi check --ship before release.");
    unmount();
  });

  it("keeps the command runnable and accents the prose around it", () => {
    setPseudoMode({});
    const { container, unmount } = render(<CodeParagraph />);
    expect(container.querySelector("code")?.textContent).toBe("kapi check --ship");
    expect(container.querySelector("p")?.textContent).toContain("Ŕüñ"); // Ŕüñ
    unmount();
    setPseudoMode(null);
  });

  it("keeps a keyboard hint readable", () => {
    setPseudoMode({});
    const { container, unmount } = render(<KbdParagraph />);
    expect(container.querySelector("kbd")?.textContent).toBe("Ctrl");
    unmount();
    setPseudoMode(null);
  });

  it("accents the span when the call site carries no answers", () => {
    setPseudoMode({});
    const { container, unmount } = render(<UnmarkedParagraph />);
    expect(container.querySelector("code")?.textContent).not.toBe("kapi check --ship");
    unmount();
    setPseudoMode(null);
  });

  it("protects the span in a translated catalog too", () => {
    setTranslations("nb", { [CODE_HASH]: "Kjor {=m0}kapi check --ship{/=m0} for utgivelse." });
    setPseudoMode({});
    const { container, unmount } = render(<CodeParagraph />);
    expect(container.querySelector("code")?.textContent).toBe("kapi check --ship");
    unmount();
    setPseudoMode(null);
  });
});
