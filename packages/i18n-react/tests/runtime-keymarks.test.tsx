import { afterEach, beforeEach, describe, expect, it } from "vite-plus/test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import {
  __t,
  __tx,
  readKeyMarks,
  setKeyMarking,
  setTranslations,
  stripKeyMarks,
} from "../src/runtime/index.ts";

beforeEach(() =>
  setTranslations("nb", { k1: "Får ikke kontakt med serveren", k2: "Prøv {=m0}igjen{/=m0}" }),
);
afterEach(() => setKeyMarking(false));

describe("key marks", () => {
  it("leave every message unmarked unless a host turns them on", () => {
    expect(__t("k1", "Can't reach your server")).toBe("Får ikke kontakt med serveren");
  });

  it("bracket a string message with its key, invisibly", () => {
    setKeyMarking(true);
    const out = __t("k1", "Can't reach your server");
    expect(stripKeyMarks(out)).toBe("Får ikke kontakt med serveren");
    expect(readKeyMarks(out)).toEqual([
      { key: "k1", start: out.indexOf("F"), end: out.length - 1 },
    ]);
  });

  it("bracket a message with elements, and nest inside another", () => {
    setKeyMarking(true);
    const inner = __tx("k2", "Try {=m0}again{/=m0}", { "=m0": createElement("b") });
    const html = renderToStaticMarkup(createElement("p", null, __t("k1", "x"), " ", inner));
    const text = html.replace(/<[^>]+>/g, "");
    expect(
      readKeyMarks(text).map((m) => [m.key, stripKeyMarks(text.slice(m.start, m.end))]),
    ).toEqual([
      ["k1", "Får ikke kontakt med serveren"],
      ["k2", "Prøv igjen"],
    ]);
  });

  it("mark inside a message that renders as one element, so it stays the only child", () => {
    setKeyMarking(true);
    setTranslations("nb", { k3: "{=m0}Lagre{/=m0}" });
    const node = __tx("k3", "{=m0}Save{/=m0}", { "=m0": createElement("button") });
    expect(renderToStaticMarkup(node as never)).toMatch(/^<button>.*Lagre.*<\/button>$/u);
    expect(readKeyMarks(renderToStaticMarkup(node as never).replace(/<[^>]+>/g, ""))[0].key).toBe(
      "k3",
    );
  });
});
