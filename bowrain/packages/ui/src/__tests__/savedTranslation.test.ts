import { describe, it, expect } from "vite-plus/test";
import type { Run } from "@neokapi/kapi-format";

import {
  savedRuns,
  savedTranslation,
  statusAfterEdit,
  withSavedTranslation,
} from "../components/editor/savedTranslation";
import type { BlockInfo, TargetEntry } from "../types/api";

const open: Run = { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } };
const close: Run = { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } };
const count: Run = { ph: { id: "0", type: "jsx:var", data: "{count}", equiv: "count" } };

function makeBlock(targets: Record<string, TargetEntry>, runs?: Record<string, Run[]>): BlockInfo {
  return {
    id: "b1",
    source: "Hello",
    targets,
    targets_runs: runs as BlockInfo["targets_runs"],
    translatable: true,
    properties: {},
  };
}

describe("savedTranslation — what a save carries", () => {
  it("keeps the runs and shows their text without the codes", () => {
    const saved = savedTranslation({
      runs: [open, { text: "Bonjour" }, close, { text: " monde" }],
    });
    expect(saved.runs).toEqual([open, { text: "Bonjour" }, close, { text: " monde" }]);
    expect(saved.text).toBe("Bonjour monde");
    // The stale prompt shows each code as a placeholder, as the server spells it.
    expect(saved.mine).toBe('<x id="1"/>Bonjour<x id="/1"/> monde');
  });

  it("shows a plural as its ICU spelling until the block is read back", () => {
    const plural: Run = {
      plural: {
        pivot: "count",
        forms: { one: [count, { text: " message" }], other: [count, { text: " messages" }] },
      },
    };
    const saved = savedRuns([plural]);
    expect(saved.runs).toEqual([plural]);
    expect(saved.text).toBe("{count, plural, one {{count} message} other {{count} messages}}");
  });

  it("writes the translation's runs and text onto the block as a reload would serve them", () => {
    const block = makeBlock({ fr: { text: "Bonjour", status: "established" } });
    const next = withSavedTranslation(block, "fr", savedRuns([{ text: "Salut" }]), "r:2");
    expect(next.targets.fr).toEqual({ text: "Salut", status: "translated" });
    expect(next.targets_runs?.fr).toEqual([{ text: "Salut" }]);
    expect(next.target_revisions?.fr).toBe("r:2");
  });
});

describe("statusAfterEdit — an edit invalidates a stale review decision", () => {
  const saved = (text: string) => savedRuns([{ text }]);

  it("demotes established to translated when the text changes", () => {
    const established = makeBlock({ fr: { text: "Bonjour", status: "established" } });
    expect(statusAfterEdit(established, "fr", saved("Salut"))).toBe("translated");
  });

  it("keeps the status when re-saving identical content", () => {
    const established = makeBlock({ fr: { text: "Bonjour", status: "established" } });
    expect(statusAfterEdit(established, "fr", saved("Bonjour"))).toBe("established");
  });

  it("makes a draft a person rewrote translated, as the change service does", () => {
    expect(
      statusAfterEdit(
        makeBlock({ fr: { text: "Bonjour", status: "draft" } }),
        "fr",
        saved("Salut"),
      ),
    ).toBe("translated");
    expect(
      statusAfterEdit(
        makeBlock({ fr: { text: "Bonjour", status: "translated" } }),
        "fr",
        saved("Salut"),
      ),
    ).toBe("translated");
    expect(statusAfterEdit(makeBlock({ fr: "Bonjour" }), "fr", saved("Salut"))).toBe("translated");
    // Saving the same wording is no edit: the rung stays where it was.
    expect(
      statusAfterEdit(
        makeBlock({ fr: { text: "Bonjour", status: "draft" } }),
        "fr",
        saved("Bonjour"),
      ),
    ).toBe("draft");
  });

  it("compares the runs when the block serves them", () => {
    // Identical once flattened, a different run sequence to the server: the
    // inline code moved. The server demotes on the runs, so the optimistic copy
    // must too — comparing plain text alone would keep a stale "established".
    const before: Run[] = [open, { text: "Bonjour" }, close, { text: " monde" }];
    const moved: Run[] = [{ text: "Bonjour " }, open, { text: "monde" }, close];
    const established = makeBlock(
      { fr: { text: "Bonjour monde", status: "established" } },
      { fr: before },
    );
    expect(statusAfterEdit(established, "fr", savedRuns(moved))).toBe("translated");
    expect(statusAfterEdit(established, "fr", savedRuns(before))).toBe("established");
  });

  it("compares the text when the block serves no runs", () => {
    const established = makeBlock({ fr: { text: "Bonjour", status: "established" } });
    expect(statusAfterEdit(established, "fr", saved("Bonjour"))).toBe("established");
    expect(statusAfterEdit(established, "fr", saved("Salut"))).toBe("translated");
  });
});
