import { describe, it, expect, beforeEach } from "vitest";
import { createElement, isValidElement, Fragment } from "react";
import { __t, __tx, setTranslations, t } from "../src/runtime/index.ts";

beforeEach(() => {
  setTranslations("", {});
});

// flattenText collects all string content from a __tx result (string, array, or
// React element tree) so a test can assert no internal {=mN} marker leaked.
function flattenText(node: unknown): string {
  if (typeof node === "string") return node;
  if (Array.isArray(node)) return node.map(flattenText).join("");
  if (isValidElement(node)) {
    const children = (node as { props?: { children?: unknown } }).props?.children;
    return children === undefined ? "" : flattenText(children);
  }
  return "";
}

describe("__t() — hash-based string lookup", () => {
  it("returns fallback when no translation", () => {
    expect(__t("hash1", "Hello")).toBe("Hello");
  });

  it("returns translation when available", () => {
    setTranslations("de", { hash1: "Hallo" });
    expect(__t("hash1", "Hello")).toBe("Hallo");
  });

  it("substitutes params", () => {
    setTranslations("de", { hash1: "Hallo, {name}!" });
    expect(__t("hash1", "Hello, {name}!", { name: "Alice" })).toBe("Hallo, Alice!");
  });

  it("resolves ICU plurals", () => {
    setTranslations("de", {
      hash1: "{count, plural, one {{count} Nachricht} other {{count} Nachrichten}}",
    });
    expect(__t("hash1", "{count} messages", { count: 1 })).toBe("1 Nachricht");
    expect(__t("hash1", "{count} messages", { count: 5 })).toBe("5 Nachrichten");
  });
});

describe("__tx() — hash-based JSX lookup", () => {
  it("returns string when no element tokens", () => {
    const result = __tx("hash1", "Hello", {});
    expect(result).toBe("Hello");
  });

  it("returns string when translation has no element tokens", () => {
    setTranslations("de", { hash1: "Hallo" });
    const result = __tx("hash1", "Hello", {});
    expect(result).toBe("Hallo");
  });

  it("interleaves elements with text", () => {
    const link = "<a>here</a>";
    const result = __tx("hash1", "Click {=m0} to continue.", { "=m0": link });
    expect(result).not.toBe("Click {=m0} to continue.");
    expect(typeof result).toBe("object");
  });

  it("uses translated text with elements", () => {
    setTranslations("de", { hash1: "Klicken Sie {=m0}, um fortzufahren." });
    const link = "<a>hier</a>";
    const result = __tx("hash1", "Click {=m0} to continue.", { "=m0": link });
    expect(typeof result).toBe("object");
  });

  it("returns string when elements are not in the translation", () => {
    setTranslations("de", { hash1: "Einfacher Text" });
    const result = __tx("hash1", "Simple text", { "=m0": "<b>bold</b>" });
    expect(result).toBe("Einfacher Text");
  });

  it("falls back to source — never a raw token — when a translation has an unbound marker", () => {
    // Stale catalog: the translation references {=m1}, but the current call site
    // only binds {=m0}. The translation must be rejected in favour of the source.
    const icon = createElement("svg", { key: "i" });
    setTranslations("de", { hash1: "Klick {=m0} und {=m1} jetzt." });
    const result = __tx("hash1", "Click {=m0} now.", { "=m0": icon });
    const text = flattenText(result);
    expect(text).not.toContain("{=m"); // no internal marker leaks to the UI
    expect(text).toContain("now"); // used the source, not the stale translation
    expect(text).not.toContain("jetzt");
  });

  it("drops an unbound standalone token rather than rendering it literally", () => {
    setTranslations("", {});
    const result = __tx("hashX", "A {=m0} B", {});
    expect(result).toBe("A  B"); // {=m0} dropped, not emitted as text
  });

  it("substitutes string params alongside elements", () => {
    setTranslations("de", { hash1: "{name} klickt {=m0}" });
    const link = "<a>hier</a>";
    const result = __tx("hash1", "{name} clicks {=m0}", { "=m0": link }, { name: "Alice" });
    expect(typeof result).toBe("object");
  });

  it("returns a transparent Fragment, not a wrapping <span>, for element children", () => {
    // Regression: shadcn <Button> uses `inline-flex items-center gap-2`
    // and relies on the icon + text being *direct* children. A
    // wrapping <span> collapses them into a single flex item, which
    // loses the gap and can wrap to two lines.
    const icon = createElement("svg", { key: "icon" });
    const result = __tx("hash1", "{=m0} Run", { "=m0": icon });
    // Must be a React element; must be a Fragment (type === Fragment symbol).
    expect(isValidElement(result)).toBe(true);
    expect((result as { type: unknown }).type).toBe(Fragment);
  });

  it("clones a paired element with the inner translated content as its children", () => {
    // The bound element is the original `<a href="/docs">here</a>`;
    // the translation moves the inner word but keeps the wrapping.
    // cloneElement replaces the children with the translated inner.
    setTranslations("de", { hash1: "Klicken Sie {=m0}hier{/=m0}, um fortzufahren." });
    const link = createElement("a", { href: "/docs" }, "here");
    const result = __tx("hash1", "Click {=m0}here{/=m0} to continue.", { "=m0": link });
    expect(isValidElement(result)).toBe(true);
    // The Fragment's children include the cloned <a> with "hier" inside.
    const children = (result as { props: { children: unknown[] } }).props.children;
    const cloned = children.find(
      (c) => isValidElement(c) && (c as { type: unknown }).type === "a",
    ) as { props: { href: string; children: unknown } } | undefined;
    expect(cloned).toBeTruthy();
    expect(cloned?.props.href).toBe("/docs");
    expect(cloned?.props.children).toBe("hier");
  });

  it("substitutes a variable inside a paired pair", () => {
    setTranslations("de", { hash1: "Klicken Sie {=m0}{userName}{/=m0}, danke." });
    const link = createElement("a", { href: "/u" }, "USER");
    const result = __tx(
      "hash1",
      "Click {=m0}{userName}{/=m0} thanks.",
      { "=m0": link },
      { userName: "Alice" },
    );
    const children = (result as { props: { children: unknown[] } }).props.children;
    const cloned = children.find(
      (c) => isValidElement(c) && (c as { type: unknown }).type === "a",
    ) as { props: { children: unknown } } | undefined;
    expect(cloned?.props.children).toBe("Alice");
  });

  it("renders nested paired pairs (LIFO well-formed)", () => {
    setTranslations("de", { hash1: "{=m0}lies {=m1}das{/=m1} doc{/=m0}" });
    const link = createElement("a", { href: "/d" }, "_");
    const strong = createElement("strong", null, "_");
    const result = __tx("hash1", "{=m0}read {=m1}the{/=m1} doc{/=m0}", {
      "=m0": link,
      "=m1": strong,
    });
    const rawChildren = (result as { props: { children: unknown } }).props.children;
    const children = Array.isArray(rawChildren) ? rawChildren : [rawChildren];
    const outerLink = children.find(
      (c) => isValidElement(c) && (c as { type: unknown }).type === "a",
    ) as { props: { children: unknown } } | undefined;
    expect(outerLink).toBeTruthy();
    const innerRaw = outerLink?.props.children;
    const innerArr = Array.isArray(innerRaw) ? innerRaw : [innerRaw];
    const inner = innerArr.find(
      (c) => isValidElement(c) && (c as { type: unknown }).type === "strong",
    ) as { props: { children: unknown } } | undefined;
    expect(inner).toBeTruthy();
    expect(inner?.props.children).toBe("das");
  });

  it("treats {=mN} as standalone when no matching {/=mN} exists in the same scope", () => {
    setTranslations("de", { hash1: "Klick {=m0} jetzt." });
    const icon = createElement("svg", { key: "icon" });
    const result = __tx("hash1", "Click {=m0} now.", { "=m0": icon });
    const children = (result as { props: { children: unknown[] } }).props.children;
    const found = children.find(
      (c) => isValidElement(c) && (c as { type: unknown }).type === "svg",
    );
    expect(found).toBeTruthy();
  });
});

describe("t() — user-facing escape hatch (dev-mode fallback)", () => {
  it("returns the source text verbatim", () => {
    expect(t("Hello")).toBe("Hello");
  });

  it("substitutes params in the source text", () => {
    expect(t("Hello, {name}!", { name: "Alice" })).toBe("Hello, Alice!");
  });

  it("accepts an optional positional context (ignored at runtime)", () => {
    // Context enters the hash at build time via the plugin; the
    // runtime fallback has no dict lookup so it just returns the
    // source text.
    expect(t("English", "UI Language")).toBe("English");
  });

  it("accepts context + params together", () => {
    expect(t("Hello, {name}!", "greeting", { name: "Alice" })).toBe("Hello, Alice!");
  });

  it("ignores the runtime dict (plugin rewrites to __t)", () => {
    // Source text returns verbatim even when a hash-keyed entry
    // exists for the same text — the plugin is the only thing
    // that knows the hash to look up.
    setTranslations("de", { someHash: "Hallo" });
    expect(t("Hello")).toBe("Hello");
  });
});
