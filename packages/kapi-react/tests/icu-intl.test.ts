/**
 * ICU number/date/time formatting via Intl, locale-formatted `#`,
 * and the ICU-syntax detector.
 */

import { describe, expect, it } from "vitest";

import { hasICUSyntax, resolveICU } from "../src/runtime/icu.ts";

describe("hasICUSyntax", () => {
  it("detects the argument types the resolver handles", () => {
    expect(hasICUSyntax("{n, plural, one {x} other {y}}")).toBe(true);
    expect(hasICUSyntax("{g, select, male {a} other {b}}")).toBe(true);
    expect(hasICUSyntax("{n, selectordinal, other {#th}}")).toBe(true);
    expect(hasICUSyntax("Total: {price, number}")).toBe(true);
    expect(hasICUSyntax("Due {when, date, long}")).toBe(true);
    expect(hasICUSyntax("At {when, time}")).toBe(true);
  });

  it("ignores plain params and element markers", () => {
    expect(hasICUSyntax("Hello, {name}!")).toBe(false);
    expect(hasICUSyntax("Click {=m0}here{/=m0}")).toBe(false);
    expect(hasICUSyntax("no braces at all")).toBe(false);
  });
});

describe("number formatting", () => {
  it("formats with locale separators", () => {
    expect(resolveICU("{n, number}", { n: 1234567.89 }, "en")).toBe("1,234,567.89");
    expect(resolveICU("{n, number}", { n: 1234567.89 }, "de")).toBe("1.234.567,89");
  });

  it("supports integer and percent styles", () => {
    expect(resolveICU("{n, number, integer}", { n: 1234.56 }, "en")).toBe("1,235");
    expect(resolveICU("{p, number, percent}", { p: 0.42 }, "en")).toBe("42%");
  });

  it("supports currency", () => {
    expect(resolveICU("{total, number, currency/EUR}", { total: 9.5 }, "de")).toContain("€");
    expect(resolveICU("{total, number, currency/USD}", { total: 9.5 }, "en")).toBe("$9.50");
  });

  it("leaves missing params as tokens", () => {
    expect(resolveICU("{n, number}", { other: 1 }, "en")).toBe("{n}");
  });
});

describe("date/time formatting", () => {
  const when = new Date(Date.UTC(2026, 6, 11, 12, 0, 0));

  it("formats dates per locale and style", () => {
    const en = resolveICU("{when, date, long}", { when }, "en");
    expect(en).toContain("July");
    expect(en).toContain("2026");
    const de = resolveICU("{when, date, long}", { when }, "de");
    expect(de).toContain("Juli");
  });

  it("accepts ISO strings and epoch numbers", () => {
    expect(resolveICU("{when, date, medium}", { when: "2026-07-11T00:00:00Z" }, "en")).toContain(
      "2026",
    );
    expect(resolveICU("{when, date, medium}", { when: when.getTime() }, "en")).toContain("2026");
  });

  it("formats times", () => {
    const out = resolveICU("{when, time, short}", { when }, "en");
    expect(out).toMatch(/\d{1,2}:\d{2}/);
  });

  it("falls back to the raw value on unparseable dates", () => {
    expect(resolveICU("{when, date}", { when: "not-a-date" }, "en")).toBe("not-a-date");
  });
});

describe("# inside plural branches", () => {
  it("is locale-number-formatted", () => {
    const msg = "{n, plural, other {# items}}";
    expect(resolveICU(msg, { n: 1234567 }, "en")).toBe("1,234,567 items");
    expect(resolveICU(msg, { n: 1234567 }, "de")).toBe("1.234.567 items");
  });
});

describe("composition", () => {
  it("nests formatting inside plural branches", () => {
    const msg =
      "{n, plural, one {{total, number, currency/USD} for one} other {{total, number, currency/USD} for #}}";
    expect(resolveICU(msg, { n: 3, total: 12 }, "en")).toBe("$12.00 for 3");
  });
});
