// @vitest-environment jsdom
/**
 * The rendered half of a block that holds a React-element parameter.
 *
 * The platform's translation editor draws its progress bar as coloured
 * segments under a label. Both sit in one `<div>`, the label carries the text,
 * and the segments arrive as `{progressSegments}`. Promotion made the `<div>`
 * one message and the segments one of its parameters, and the reader got
 * `[object Object]` painted across the bar (#2561).
 *
 * `__tx` now asks each parameter what it holds. React content keeps its token
 * through substitution and renders as a node wherever the translator put it;
 * a string, a number and a `null` behave as they always have.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";

import { setTranslations, setStringTransform, __t, __tx } from "../src/runtime/index.ts";
import { hashKey } from "../src/plugin/hash.ts";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function render(node: ReactNode): { container: HTMLDivElement; unmount: () => void } {
  const container = document.createElement("div");
  const root = createRoot(container);
  act(() => {
    root.render(node);
  });
  return { container, unmount: () => act(() => root.unmount()) };
}

const SAVE = "{icon} Save changes";
const SAVE_HASH = hashKey(SAVE, "div");
const Icon = () => <b data-testid="icon">*</b>;

describe("an element-valued parameter beside a sentence", () => {
  beforeEach(() => {
    setTranslations("", {});
  });

  it("renders the element, not its String() form", () => {
    const { container, unmount } = render(
      <div>{__tx(SAVE_HASH, SAVE, {}, { icon: <Icon /> })}</div>,
    );
    expect(container.querySelector("[data-testid=icon]")).not.toBeNull();
    expect(container.textContent).toBe("* Save changes");
    expect(container.textContent).not.toContain("[object Object]");
    unmount();
  });

  it("follows the translator when the token moves to the end", () => {
    setTranslations("qps", { [SAVE_HASH]: "Ŝàvé çĥàñĝéš {icon}" });
    const { container, unmount } = render(
      <div>{__tx(SAVE_HASH, SAVE, {}, { icon: <Icon /> })}</div>,
    );
    expect(container.textContent).toBe("Ŝàvé çĥàñĝéš *");
    unmount();
  });

  it("drops the element when the translation drops the token", () => {
    setTranslations("qps", { [SAVE_HASH]: "Ŝàvé çĥàñĝéš" });
    const { container, unmount } = render(
      <div>{__tx(SAVE_HASH, SAVE, {}, { icon: <Icon /> })}</div>,
    );
    expect(container.textContent).toBe("Ŝàvé çĥàñĝéš");
    expect(container.querySelector("[data-testid=icon]")).toBeNull();
    unmount();
  });

  it("keeps the element's props and its handler", () => {
    const onClick = vi.fn();
    const { container, unmount } = render(
      <div>
        {__tx(
          SAVE_HASH,
          SAVE,
          {},
          {
            icon: (
              <button type="button" onClick={onClick} className="ghost">
                go
              </button>
            ),
          },
        )}
      </div>,
    );
    const button = container.querySelector("button");
    expect(button?.className).toBe("ghost");
    expect(button?.getAttribute("type")).toBe("button");
    act(() => button?.dispatchEvent(new MouseEvent("click", { bubbles: true })));
    expect(onClick).toHaveBeenCalledOnce();
    unmount();
  });

  it("renders every element of a mapped list", () => {
    const rows = [<i key="a">A</i>, <i key="b">B</i>];
    const { container, unmount } = render(<div>{__tx(SAVE_HASH, SAVE, {}, { icon: rows })}</div>);
    expect(container.querySelectorAll("i")).toHaveLength(2);
    expect(container.textContent).toBe("AB Save changes");
    unmount();
  });

  it("renders each appearance when the translation repeats the token", () => {
    setTranslations("qps", { [SAVE_HASH]: "{icon} Ŝàvé {icon}" });
    const { container, unmount } = render(
      <div>{__tx(SAVE_HASH, SAVE, {}, { icon: <Icon /> })}</div>,
    );
    expect(container.querySelectorAll("[data-testid=icon]")).toHaveLength(2);
    unmount();
  });
});

describe("parameters that are not React content", () => {
  beforeEach(() => {
    setTranslations("", {});
  });

  it("still substitutes a string", () => {
    const text = "{who} saved it";
    const hash = hashKey(text, "div");
    const { container, unmount } = render(<div>{__tx(hash, text, {}, { who: "Ada" })}</div>);
    expect(container.textContent).toBe("Ada saved it");
    unmount();
  });

  it("still renders a null parameter as nothing", () => {
    const text = "Words{indicator}";
    const hash = hashKey(text, "div");
    const { container, unmount } = render(<div>{__tx(hash, text, {}, { indicator: null })}</div>);
    expect(container.textContent).toBe("Words");
    unmount();
  });

  it("still joins an array of strings the way it always did", () => {
    const text = "Tags: {tags}";
    const hash = hashKey(text, "div");
    const { container, unmount } = render(
      <div>{__tx(hash, text, {}, { tags: ["a", "b"] as unknown as string })}</div>,
    );
    expect(container.textContent).toBe("Tags: a,b");
    unmount();
  });

  it("still formats a number through ICU", () => {
    const text = "{n, number} saved";
    const hash = hashKey(text, "div");
    const { container, unmount } = render(<div>{__tx(hash, text, {}, { n: 1200 })}</div>);
    expect(container.textContent).toBe("1,200 saved");
    unmount();
  });
});

describe("an element-valued parameter among other tokens", () => {
  beforeEach(() => {
    setTranslations("", {});
  });

  it("renders inside a paired element's range", () => {
    const text = "Saved {=m0}by {who}{/=m0}";
    const hash = hashKey(text, "p");
    const { container, unmount } = render(
      <p>{__tx(hash, text, { "=m0": <em /> }, { who: <b data-testid="who">Ada</b> })}</p>,
    );
    expect(container.querySelector("em [data-testid=who]")).not.toBeNull();
    expect(container.textContent).toBe("Saved by Ada");
    unmount();
  });

  it("survives an ICU branch", () => {
    const text = "{count, plural, one {1 file {icon}} other {# files {icon}}}";
    const hash = hashKey(text, "div");
    const { container, unmount } = render(
      <div>{__tx(hash, text, {}, { count: 3, icon: <Icon /> })}</div>,
    );
    expect(container.textContent).toBe("3 files *");
    expect(container.querySelector("[data-testid=icon]")).not.toBeNull();
    unmount();
  });

  it("survives a string transform that leaves braces alone", () => {
    setStringTransform((t) => t.replace(/Save/g, "Ŝàvé"));
    const { container, unmount } = render(
      <div>{__tx(SAVE_HASH, SAVE, {}, { icon: <Icon /> })}</div>,
    );
    expect(container.textContent).toBe("* Ŝàvé changes");
    expect(container.querySelector("[data-testid=icon]")).not.toBeNull();
    unmount();
    setStringTransform(null);
  });

  it("returns the element alone when it is the whole message", () => {
    const text = "{icon}";
    const hash = hashKey(text, "div");
    const { container, unmount } = render(<div>{__tx(hash, text, {}, { icon: <Icon /> })}</div>);
    expect(container.innerHTML).toBe('<div><b data-testid="icon">*</b></div>');
    unmount();
  });
});

/**
 * The walkthrough case from the issue: the editor's progress bar, with the
 * coloured segments beside the label that carries the text.
 */
describe("the progress bar", () => {
  const TEXT =
    "{progressSegments}{=m1}{progress}% ({translatedCount}/{translatableCount} translated){/=m1}";
  const HASH = hashKey(TEXT, "div");

  const segments = (
    <div className="segments">
      <div data-testid="progress-reviewed" style={{ width: "40%" }} />
      <div data-testid="progress-draft" style={{ width: "10%" }} />
    </div>
  );

  const label = <span data-testid="progress-text" className="label" />;

  function Bar() {
    return (
      <div className="relative" data-testid="progress-bar">
        {__tx(
          HASH,
          TEXT,
          { "=m1": label },
          {
            progressSegments: segments,
            progress: 50,
            translatedCount: 5,
            translatableCount: 10,
          },
        )}
      </div>
    );
  }

  beforeEach(() => {
    setTranslations("", {});
  });

  it("draws the segments as elements and the label as text", () => {
    const { container, unmount } = render(<Bar />);
    expect(container.querySelector("[data-testid=progress-reviewed]")).not.toBeNull();
    expect(container.querySelector("[data-testid=progress-draft]")).not.toBeNull();
    expect(container.querySelector("[data-testid=progress-text]")?.textContent).toBe(
      "50% (5/10 translated)",
    );
    unmount();
  });

  it("puts no [object Object] on the screen", () => {
    const { container, unmount } = render(<Bar />);
    expect(container.textContent).toBe("50% (5/10 translated)");
    unmount();
  });

  it("keeps both when the label is translated", () => {
    setTranslations("qps", {
      [HASH]:
        "{progressSegments}{=m1}{progress}% ({translatedCount}/{translatableCount} ţŕàñšļàţéð){/=m1}",
    });
    const { container, unmount } = render(<Bar />);
    expect(container.querySelector("[data-testid=progress-reviewed]")).not.toBeNull();
    expect(container.querySelector("[data-testid=progress-text]")?.textContent).toBe(
      "50% (5/10 ţŕàñšļàţéð)",
    );
    unmount();
  });
});

describe("a React element handed to a string message", () => {
  const text = "Close {icon}";
  const hash = hashKey(text, "button[aria-label]");
  let warn: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    setTranslations("", {});
    warn = vi.spyOn(console, "warn").mockImplementation(() => {});
  });

  afterEach(() => {
    warn.mockRestore();
  });

  it("says so, because __t answers with a string", () => {
    __t(hash, text, { icon: <Icon /> } as unknown as Record<string, string>);
    expect(warn).toHaveBeenCalledOnce();
    expect(warn.mock.calls[0][0]).toContain('parameter "icon"');
  });

  it("says nothing for an ordinary string parameter", () => {
    __t(hash, text, { icon: "x" });
    expect(warn).not.toHaveBeenCalled();
  });
});
