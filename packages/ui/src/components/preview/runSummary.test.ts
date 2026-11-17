import { describe, expect, it } from "vite-plus/test";
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
    ).toBe("⟨count: one: One item; other: ⟨value⟩ items for ⟨audience: other: ⟨value⟩⟩⟩");
  });

  it("reads a paired code's content without tags, and names a sub-block reference", () => {
    expect(
      runSummary([
        { pcOpen: { id: "b", data: "<b>" } },
        { text: "Read " },
        { sub: { id: "s", ref: "notice" } },
        { pcClose: { id: "b", data: "</b>" } },
      ]),
    ).toBe("Read ⟨notice⟩");
  });
});

describe("coded text in summaries", () => {
  it("never shows a code's native form or a synthesized identifier", () => {
    const summary = runSummary([
      { text: "Reach " },
      {
        pcOpen: {
          id: "0",
          type: "jsx:element",
          subType: "span",
          data: '<span className="x">',
          equiv: "=m0",
        },
      },
      { ph: { id: "1", type: "jsx:var", data: "{serverURL}", equiv: "serverURL" } },
      { pcClose: { id: "0", type: "jsx:element", subType: "span", data: "</span>", equiv: "=m0" } },
      { ph: { id: "2", type: "jsx:node", data: "{open && <Link/>}", equiv: "=m2" } },
    ]);
    expect(summary).toBe("Reach ⟨serverURL⟩⟨content⟩");
  });
});
