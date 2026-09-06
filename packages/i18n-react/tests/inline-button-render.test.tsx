// @vitest-environment jsdom
/**
 * The rendered half of a paragraph that holds a button.
 *
 * Extraction puts the whole sentence in one message and the button in it as a
 * paired code, so a translation carries `{=m0}…{/=m0}` around the label. `__tx`
 * clones the button with the translated label as its children, which is what
 * keeps the handler, the type and the classes the author wrote: the reader gets
 * a working button with translated copy on both sides of it.
 */
import { describe, it, expect, beforeEach, vi } from "vitest";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { setTranslations, __tx } from "../src/runtime/index.ts";
import { hashKey } from "../src/plugin/hash.ts";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const SOURCE = "Press {=m0}Go{/=m0} to start.";
const HASH = hashKey(SOURCE, "p");
const TRANSLATED = "Þŕéšš {=m0}Ĝö{/=m0} ţö šţàŕţ.";

/** The call site the transform emits for `<p>Press <button…>Go</button> to start.</p>`. */
function Paragraph({ onRun }: { onRun: () => void }) {
  return (
    <p>
      {__tx(HASH, SOURCE, {
        "=m0": (
          <button type="button" className="button button--primary" onClick={onRun}>
            Go
          </button>
        ),
      })}
    </p>
  );
}

function render(onRun: () => void): { container: HTMLDivElement; unmount: () => void } {
  const container = document.createElement("div");
  const root = createRoot(container);
  act(() => {
    root.render(<Paragraph onRun={onRun} />);
  });
  return { container, unmount: () => act(() => root.unmount()) };
}

describe("a translated paragraph holding a button", () => {
  beforeEach(() => {
    setTranslations("", {});
  });

  it("renders the source sentence with a real button when no catalog is loaded", () => {
    const { container, unmount } = render(() => {});
    const button = container.querySelector("button");
    expect(button?.textContent).toBe("Go");
    expect(container.querySelector("p")?.textContent).toBe("Press Go to start.");
    unmount();
  });

  it("renders the translated prose and the translated label", () => {
    setTranslations("qps", { [HASH]: TRANSLATED });
    const { container, unmount } = render(() => {});
    const paragraph = container.querySelector("p");
    expect(paragraph?.textContent).toBe("Þŕéšš Ĝö ţö šţàŕţ.");
    expect(container.querySelector("button")?.textContent).toBe("Ĝö");
    unmount();
  });

  it("keeps the button's attributes through the substitution", () => {
    setTranslations("qps", { [HASH]: TRANSLATED });
    const { container, unmount } = render(() => {});
    const button = container.querySelector("button");
    expect(button?.getAttribute("type")).toBe("button");
    expect(button?.className).toBe("button button--primary");
    unmount();
  });

  it("still calls the handler when the label is translated", () => {
    setTranslations("qps", { [HASH]: TRANSLATED });
    const onRun = vi.fn();
    const { container, unmount } = render(onRun);
    act(() => {
      container.querySelector("button")?.click();
    });
    expect(onRun).toHaveBeenCalledTimes(1);
    unmount();
  });

  it("renders one working button when the translation moves it in the sentence", () => {
    setTranslations("qps", { [HASH]: "Ţö šţàŕţ, þŕéšš {=m0}Ĝö{/=m0}." });
    const onRun = vi.fn();
    const { container, unmount } = render(onRun);
    expect(container.querySelectorAll("button")).toHaveLength(1);
    expect(container.querySelector("p")?.textContent).toBe("Ţö šţàŕţ, þŕéšš Ĝö.");
    act(() => {
      container.querySelector("button")?.click();
    });
    expect(onRun).toHaveBeenCalledTimes(1);
    unmount();
  });
});
