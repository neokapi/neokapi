import { describe, expect, it } from "vitest";
import { runSummary } from "./runSummary";

describe("collapsed run summaries", () => {
  it("keeps a name placeholder visible between text runs", () => {
    expect(
      runSummary([{ text: "Hello, " }, { ph: { id: "name", data: "name" } }, { text: "!" }]),
    ).toBe("Hello, ⟨name⟩!");
  });

  it("keeps plural counters and all nested select branches", () => {
    expect(
      runSummary([
        {
          plural: {
            pivot: "count",
            forms: {
              one: [{ text: "One item" }],
              other: [
                { ph: { id: "count", data: "#" } },
                { text: " items for " },
                {
                  select: {
                    pivot: "audience",
                    cases: {
                      other: [{ ph: { id: "name" } }],
                    },
                  },
                },
              ],
            },
          },
        },
      ]),
    ).toBe(
      "{count, plural, one {One item} other {# items for {audience, select, other {⟨name⟩}}}}",
    );
  });

  it("preserves paired codes and sub-block references", () => {
    expect(
      runSummary([
        { pcOpen: { id: "b", data: "<b>" } },
        { text: "Read " },
        { sub: { id: "s", ref: "notice" } },
        { pcClose: { id: "b", data: "</b>" } },
      ]),
    ).toBe("<b>Read [notice]</b>");
  });
});
