import { describe, it, expect } from "vitest";
import { formatWhen, relativeTime } from "../lib/when";

// The expectations are ICU's own output rather than a table this repo
// maintains. They avoid naming a calendar day, because `formatWhen` renders in
// the reader's own zone and this suite runs in whatever zone the machine is in.
const INSTANT = "2026-08-30T09:12:00Z";

describe("formatWhen", () => {
  it("renders the date and the time in the reader's language", () => {
    const en = formatWhen(INSTANT, { uiLocale: "en-US" });
    expect(en.valid).toBe(true);
    expect(en.iso).toBe("2026-08-30T09:12:00.000Z");
    expect(en.text).toMatch(/Aug(ust)? \d+, 2026/);
  });

  it("names the month in the reader's own language", () => {
    expect(formatWhen(INSTANT, { uiLocale: "fr" }).text).toMatch(/août/);
    expect(formatWhen(INSTANT, { uiLocale: "nb" }).text).toMatch(/aug/);
  });

  it("carries the exact instant with its zone in the title", () => {
    const { title, text } = formatWhen(INSTANT, { uiLocale: "en-US" });
    expect(title).not.toBe(text);
    expect(title).toMatch(/2026/);
    // `timeStyle: "long"` is what puts the zone on it, whichever zone that is.
    expect(title).toMatch(/GMT|UTC|[A-Z]{2,5}T/);
  });

  it("drops the half a caller says `none` to", () => {
    const dateOnly = formatWhen(INSTANT, { uiLocale: "en-US", timeStyle: "none" });
    expect(dateOnly.text).toMatch(/2026/);
    expect(dateOnly.text).not.toMatch(/:/);
    const timeOnly = formatWhen(INSTANT, { uiLocale: "en-US", dateStyle: "none" });
    expect(timeOnly.text).toMatch(/:/);
    expect(timeOnly.text).not.toMatch(/2026/);
  });

  it("returns an unreadable value as it was given rather than as Invalid Date", () => {
    expect(formatWhen("whenever")).toEqual({
      text: "whenever",
      title: "whenever",
      iso: "whenever",
      valid: false,
    });
    expect(formatWhen(undefined).text).toBe("");
    expect(formatWhen("").valid).toBe(false);
  });

  it("takes a Date and a millisecond count as readily as a string", () => {
    const d = new Date(INSTANT);
    expect(formatWhen(d, { uiLocale: "en-US" }).iso).toBe(d.toISOString());
    expect(formatWhen(d.getTime(), { uiLocale: "en-US" }).iso).toBe(d.toISOString());
  });
});

describe("formatWhen relative", () => {
  const now = new Date("2026-08-30T12:00:00Z").getTime();
  const at = (ms: number) => new Date(now - ms).toISOString();
  const MINUTE = 60_000;
  const HOUR = 3_600_000;
  const DAY = 24 * HOUR;

  const cases: [string, number, string][] = [
    ["under a minute reads as now", 20_000, "now"],
    ["minutes", 3 * MINUTE, "3 minutes ago"],
    ["an hour, truncated rather than rounded up", 119 * MINUTE, "1 hour ago"],
    ["a day reads as yesterday", DAY, "yesterday"],
    ["weeks", 18 * DAY, "2 weeks ago"],
    ["months", 120 * DAY, "3 months ago"],
    ["years", 800 * DAY, "2 years ago"],
  ];

  for (const [name, ago, want] of cases) {
    it(name, () => {
      expect(formatWhen(at(ago), { uiLocale: "en-US", relative: true, now }).text).toBe(want);
    });
  }

  it("reads a future instant forwards", () => {
    const ahead = new Date(now + 2 * HOUR).toISOString();
    expect(formatWhen(ahead, { uiLocale: "en-US", relative: true, now }).text).toBe("in 2 hours");
  });

  it("keeps the exact instant in the title, so recency never hides the moment", () => {
    const { title } = formatWhen(at(3 * MINUTE), { uiLocale: "en-US", relative: true, now });
    expect(title).toMatch(/2026/);
  });

  it("speaks the distance in the reader's language", () => {
    expect(formatWhen(at(3 * MINUTE), { uiLocale: "fr", relative: true, now }).text).toBe(
      "il y a 3 minutes",
    );
  });

  it("measures against a Date as readily as a millisecond count", () => {
    expect(
      formatWhen(at(3 * MINUTE), { uiLocale: "en-US", relative: true, now: new Date(now) }).text,
    ).toBe("3 minutes ago");
  });
});

describe("relativeTime", () => {
  it("is formatWhen's relative form, and the only one", () => {
    const iso = new Date(Date.now() - 5 * 60_000).toISOString();
    expect(relativeTime(iso)).toBe(formatWhen(iso, { relative: true }).text);
  });

  it("returns an unreadable value unchanged", () => {
    expect(relativeTime("whenever")).toBe("whenever");
  });
});
