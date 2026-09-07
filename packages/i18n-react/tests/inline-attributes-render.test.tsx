// @vitest-environment jsdom
/**
 * The rendered half of a sentence holding an element with a translatable
 * attribute.
 *
 * The transform hands `__tx` the element with its attribute already resolved
 * through its own lookup, so the alt text and the prose around it come from
 * two catalog entries and land on one line of output. `cloneElement` keeps the
 * props the call site built, which is what makes the two independent.
 */
import { beforeEach, describe, expect, it } from "vitest";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { hashKey } from "../src/plugin/hash.ts";
import { setTranslations, __t, __tx } from "../src/runtime/index.ts";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const SENTENCE = "See {=m0} above.";
const BLOCK = hashKey(SENTENCE, "p");
const ALT = hashKey("the chart", "img[alt]");

/** The call site the transform emits for `<p>See <img alt="the chart" /> above.</p>`. */
function Paragraph() {
  return (
    <p>{__tx(BLOCK, SENTENCE, { "=m0": <img src="/x.png" alt={__t(ALT, "the chart")} /> })}</p>
  );
}

function render(): { container: HTMLDivElement; unmount: () => void } {
  const container = document.createElement("div");
  const root = createRoot(container);
  act(() => {
    root.render(<Paragraph />);
  });
  return { container, unmount: () => act(() => root.unmount()) };
}

describe("a translated sentence holding an image", () => {
  beforeEach(() => {
    setTranslations("", {});
  });

  it("renders the source sentence and the source alt with no catalog", () => {
    const { container, unmount } = render();
    expect(container.querySelector("p")?.textContent).toBe("See  above.");
    expect(container.querySelector("img")?.getAttribute("alt")).toBe("the chart");
    unmount();
  });

  it("renders the translated prose and the translated alt", () => {
    setTranslations("qps", { [BLOCK]: "Šéé {=m0} àbövé.", [ALT]: "ţhé çhàŕţ" });
    const { container, unmount } = render();
    expect(container.querySelector("p")?.textContent).toBe("Šéé  àbövé.");
    expect(container.querySelector("img")?.getAttribute("alt")).toBe("ţhé çhàŕţ");
    unmount();
  });

  it("translates the alt even when the sentence has no translation", () => {
    setTranslations("qps", { [ALT]: "ţhé çhàŕţ" });
    const { container, unmount } = render();
    expect(container.querySelector("p")?.textContent).toBe("See  above.");
    expect(container.querySelector("img")?.getAttribute("alt")).toBe("ţhé çhàŕţ");
    unmount();
  });

  it("keeps the other props through the substitution", () => {
    setTranslations("qps", { [BLOCK]: "Šéé {=m0} àbövé.", [ALT]: "ţhé çhàŕţ" });
    const { container, unmount } = render();
    expect(container.querySelector("img")?.getAttribute("src")).toBe("/x.png");
    unmount();
  });

  it("renders one image when the translation moves it in the sentence", () => {
    setTranslations("qps", { [BLOCK]: "Àbövé, šéé {=m0}.", [ALT]: "ţhé çhàŕţ" });
    const { container, unmount } = render();
    expect(container.querySelectorAll("img")).toHaveLength(1);
    expect(container.querySelector("img")?.getAttribute("alt")).toBe("ţhé çhàŕţ");
    unmount();
  });
});
