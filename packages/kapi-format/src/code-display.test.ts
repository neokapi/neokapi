import { describe, expect, it } from "vite-plus/test";

import type { Run } from "./block.ts";
import { codeDisplays, codeKind, humanCodeName, type CodeDisplay } from "./code-display.ts";
import { projectRuns, type ModelRunSpec } from "./run-projection.ts";

/** Every code in a sequence as a surface would show it. */
function displays(runs: Run[]): CodeDisplay[] {
  const d = codeDisplays(runs);
  const spec: ModelRunSpec<CodeDisplay | null> = {
    text: () => null,
    ph: ({ ph }) => d.ph(ph),
    pcOpen: ({ pcOpen }) => d.pcOpen(pcOpen),
    pcClose: ({ pcClose }) => d.pcClose(pcClose),
    sub: ({ sub }) => d.sub(sub),
    plural: ({ plural }) => d.choice(plural.pivot),
    select: ({ select }) => d.choice(select.pivot),
    fallback: () => d.unknown(),
  };
  return projectRuns(runs, spec).filter((x): x is CodeDisplay => x !== null);
}

describe("codeDisplays", () => {
  it("never shows a synthesized identifier or native data", () => {
    // The React extractor's shape for <span>{serverURL}</span>.
    const runs: Run[] = [
      { text: "Bowrain needs to reach " },
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
    ];
    const got = displays(runs).map(({ role, kind, name }) => ({ role, kind, name }));
    expect(got).toEqual([
      { role: "open", kind: "element", name: undefined },
      { role: "placeholder", kind: "variable", name: "serverURL" },
      { role: "close", kind: "element", name: undefined },
    ]);
  });

  it("names a code only with a human name, whatever the extractor wrote", () => {
    const hostile = ["=m0", "<=m0>", "</b>", '<a href="x">', "%1$s", "[/B]", "a".repeat(80), ""];
    for (const s of hostile) {
      const runs: Run[] = [
        { pcOpen: { id: "1", type: "x-bold", data: s, equiv: s, disp: s } },
        { ph: { id: "2", type: "x-var", data: s, equiv: s, disp: s } },
        { pcClose: { id: "1", type: "x-bold", data: s, equiv: s } },
        { sub: { id: "3", ref: "r", equiv: s } } as Run,
      ];
      for (const d of displays(runs)) {
        if (d.name !== undefined) expect(d.name).toMatch(/^\p{L}[\p{L}\p{N} _.-]*$/u);
      }
    }
  });

  it("keeps a vocabulary's display label and drops its brackets", () => {
    expect(humanCodeName("[B]")).toBe("B");
    expect(humanCodeName("=m3", "label")).toBe("label");
    expect(humanCodeName("{=m0}")).toBeUndefined();
    expect(humanCodeName("<b>")).toBeUndefined();
  });

  it("keeps a variable's name from inside its delimiters", () => {
    expect(humanCodeName("{{name}}")).toBe("name");
    expect(humanCodeName("{{.Name}}")).toBe("Name");
    expect(humanCodeName("{count}")).toBe("count");
    expect(humanCodeName("${value}")).toBe("value");
    expect(humanCodeName("%(user)s")).toBe("user");
    expect(humanCodeName("%1$s")).toBeUndefined();
  });

  it("names the closing half after its opening half", () => {
    const runs: Run[] = [
      { pcOpen: { id: "1", type: "html:a", subType: "a", data: "<a>", equiv: "docsLink" } },
      { text: "the guide" },
      { pcClose: { id: "1", type: "html:a", subType: "a", data: "</a>" } },
    ];
    const [open, close] = displays(runs);
    expect(open).toMatchObject({ role: "open", kind: "link", name: "docsLink" });
    expect(close).toMatchObject({ role: "close", kind: "link", name: "docsLink" });
  });

  it("reads a tag as the kind, not as a name", () => {
    const [br] = displays([
      { ph: { id: "1", type: "html:br", subType: "br", data: "<br/>", equiv: "br" } },
    ]);
    expect(br.kind).toBe("break");
    expect(br.name).toBeUndefined();
  });

  it("kinds codes by type and subtype", () => {
    expect(codeKind("jsx:element", "strong")).toBe("bold");
    expect(codeKind("jsx:node")).toBe("content");
    expect(codeKind("jsx:var")).toBe("variable");
    expect(codeKind("x-link")).toBe("element");
    expect(codeKind("md:link")).toBe("link");
  });
});
