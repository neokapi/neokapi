// The projection contract: a run sequence is answered for kind by kind, and a
// kind that has no answer is a compile error rather than a quiet omission.
import { describe, it, expect, afterEach } from "vitest";

import type { Run } from "./block.ts";
import {
  RUN_KINDS,
  projectRuns,
  projectRunsText,
  otherBranch,
  runKindOf,
  setRunProjectionReporter,
  type ModelRunSpec,
  type RunKind,
} from "./run-projection.ts";

const name: Run = { ph: { id: "1", type: "code:variable", data: "{{.Name}}", equiv: "{{.Name}}" } };
const bold = {
  open: { pcOpen: { id: "2", type: "fmt:bold", data: "<b>", equiv: "<b>" } } as Run,
  close: { pcClose: { id: "2", type: "fmt:bold", data: "</b>" } } as Run,
};

/** A reading that shows codes and reads one branch of a plural. */
const READING: ModelRunSpec<string> = {
  text: (r) => r.text,
  ph: (r) => `⟨${r.ph.equiv}⟩`,
  pcOpen: { dropped: "formatting is applied, not written out" },
  pcClose: { dropped: "formatting is applied, not written out" },
  sub: { unsupported: "a subblock is projected as its own node, not inline here" },
  plural: { expand: (r) => projectRuns(otherBranch(r.plural.forms), READING) },
  select: { expand: (r) => projectRuns(otherBranch(r.select.cases), READING) },
  fallback: (kind) => `⟨${kind}⟩`,
};

afterEach(() => setRunProjectionReporter(null));

/** Silence the default (throwing) reporter and record what it was told. */
function captureReports(): string[] {
  const seen: string[] = [];
  setRunProjectionReporter((kind, why) => seen.push(`${kind}: ${why}`));
  return seen;
}

describe("RUN_KINDS", () => {
  it("names every discriminator the model defines", () => {
    // The list is the exhaustiveness contract, so it is worth stating twice:
    // a kind added to the model and not to this list would let every existing
    // projection keep compiling while quietly never seeing it.
    expect([...RUN_KINDS]).toEqual(["text", "ph", "pcOpen", "pcClose", "sub", "plural", "select"]);
  });

  it("reads the kind off a run", () => {
    expect(runKindOf({ text: "" })).toBe("text");
    expect(runKindOf(name)).toBe("ph");
    expect(runKindOf({})).toBeNull();
  });
});

describe("projectRuns", () => {
  it("keeps a placeholder in its place in the reading", () => {
    const runs: Run[] = [{ text: "Hello " }, name, { text: "!" }];
    expect(projectRunsText(runs, READING)).toBe("Hello ⟨{{.Name}}⟩!");
  });

  it("omits a dropped kind, and says so at the declaration", () => {
    const runs: Run[] = [bold.open, { text: "now" }, bold.close];
    expect(projectRunsText(runs, READING)).toBe("now");
  });

  it("reads one branch of a plural rather than nothing", () => {
    const runs: Run[] = [
      {
        plural: {
          pivot: "count",
          forms: { one: [{ text: "1 file" }], other: [{ text: "n files" }] },
        },
      },
    ];
    expect(projectRunsText(runs, READING)).toBe("n files");
  });

  it("falls back to the first branch when there is no `other`", () => {
    expect(otherBranch({ one: [{ text: "1 file" }] })).toEqual([{ text: "1 file" }]);
  });

  it("reports an unsupported kind and shows the fallback in its place", () => {
    const seen = captureReports();
    const runs: Run[] = [{ text: "see " }, { sub: { id: "3", ref: "b2", equiv: "the note" } }];

    expect(projectRunsText(runs, READING)).toBe("see ⟨sub⟩");
    expect(seen).toEqual(["sub: a subblock is projected as its own node, not inline here"]);
  });

  it("reports a run whose kind this build does not know, and still shows it", () => {
    const seen = captureReports();
    // What a newer engine emitting a kind this build predates looks like here.
    const runs = [{ text: "a " }, { footnote: { id: "9" } }, { text: " b" }] as unknown as Run[];

    expect(projectRunsText(runs, READING)).toBe("a ⟨unknown⟩ b");
    expect(seen).toEqual(["unknown: the run carries no discriminator this build knows"]);
  });

  it("throws by default, so a projection that cannot render is fixed, not shipped", () => {
    const runs: Run[] = [{ sub: { id: "3", ref: "b2", equiv: "the note" } }];
    expect(() => projectRunsText(runs, READING)).toThrow(/cannot render a "sub" run/);
  });
});

describe("the spec type", () => {
  it("does not compile when a kind has no answer", () => {
    const incomplete = {
      text: (r: Extract<Run, { text: string }>) => r.text,
      ph: (r: Extract<Run, { ph: unknown }>) => r.ph.equiv,
      pcOpen: { dropped: "…" },
      pcClose: { dropped: "…" },
      sub: { dropped: "…" },
      select: { dropped: "…" },
      fallback: () => "",
      // `plural` is missing — which is exactly how a plural block came to
      // render as an empty line.
    };
    // @ts-expect-error -- a RunSpec must answer for every kind in RUN_KINDS
    const spec: ModelRunSpec<string> = incomplete;
    expect(spec).toBeDefined();
  });

  it("types a rule's run by its kind", () => {
    // Compile-time, and the reason this file's READING spec reads `r.ph.equiv`
    // with no narrowing of its own: a rule is typed by the kind it answers for,
    // so the `ph` rule could not have read `r.pcOpen`.
    const kinds: RunKind[] = [...RUN_KINDS];
    expect(kinds).toContain("ph");
  });
});
