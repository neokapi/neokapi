import { describe, it, expect } from "vite-plus/test";
import type { ChangeSetOp, ChangeSetOpPayload, OpType } from "../../types/brand-graph";
import {
  isGovernedTermTransition,
  isGovernedOp,
  opDiffRow,
  opSummary,
  opCategory,
  groupOps,
  governedOpCount,
  CATEGORY_LABEL,
} from "./ops";

let seq = 0;
function mkOp(op: OpType, payload: ChangeSetOpPayload): ChangeSetOp {
  return {
    workspace_id: "ws-1",
    changeset_id: "cs-1",
    seq: ++seq,
    op,
    payload,
    base_rev: 0,
    created_by: "tester",
    created_at: "2026-06-13T10:00:00Z",
  };
}

describe("isGovernedTermTransition", () => {
  it("is governed when targeting forbidden or preferred", () => {
    expect(isGovernedTermTransition("proposed", "forbidden")).toBe(true);
    expect(isGovernedTermTransition("approved", "preferred")).toBe(true);
  });
  it("is governed when moving away from forbidden", () => {
    expect(isGovernedTermTransition("forbidden", "deprecated")).toBe(true);
  });
  it("is ordinary otherwise", () => {
    expect(isGovernedTermTransition("approved", "admitted")).toBe(false);
    expect(isGovernedTermTransition("preferred", "admitted")).toBe(false);
  });
  it("never governs a no-op transition", () => {
    expect(isGovernedTermTransition("forbidden", "forbidden")).toBe(false);
    expect(isGovernedTermTransition("preferred", "preferred")).toBe(false);
  });
});

describe("isGovernedOp", () => {
  it("flags a ban (term.status → forbidden)", () => {
    const op = mkOp("term.status", {
      concept_id: "c-1",
      locale: "en-US",
      text: "utilize",
      from: "approved",
      to: "forbidden",
    });
    expect(isGovernedOp(op)).toBe(true);
  });
  it("flags a REPLACED_BY relation but not an ordinary one", () => {
    const replaced = mkOp("relation.add", {
      relation: {
        id: "r-1",
        source_id: "c-1",
        target_id: "c-2",
        relation_type: "REPLACED_BY",
        created_at: "2026-06-13T10:00:00Z",
      },
    });
    const related = mkOp("relation.add", {
      relation: {
        id: "r-2",
        source_id: "c-1",
        target_id: "c-2",
        relation_type: "RELATED",
        created_at: "2026-06-13T10:00:00Z",
      },
    });
    expect(isGovernedOp(replaced)).toBe(true);
    expect(isGovernedOp(related)).toBe(false);
  });
  it("always flags concept.delete and voice-rule ops", () => {
    expect(isGovernedOp(mkOp("concept.delete", { concept_id: "c-1" }))).toBe(true);
    expect(
      isGovernedOp(
        mkOp("voice.rule.add", { profile_id: "p-1", list: "forbidden", rule: { term: "x" } }),
      ),
    ).toBe(true);
    expect(
      isGovernedOp(mkOp("voice.rule.remove", { profile_id: "p-1", list: "preferred", term: "x" })),
    ).toBe(true);
  });
  it("treats ordinary term/relation/concept edits as ungoverned", () => {
    expect(
      isGovernedOp(
        mkOp("term.add", {
          concept_id: "c-1",
          term: { text: "x", locale: "en-US", status: "proposed" },
        }),
      ),
    ).toBe(false);
    expect(isGovernedOp(mkOp("relation.remove", { relation_id: "r-1" }))).toBe(false);
    expect(isGovernedOp(mkOp("concept.update", { concept_id: "c-1", definition: "d" }))).toBe(
      false,
    );
  });
});

describe("opDiffRow / opSummary", () => {
  it("bans a term with a destructive tone", () => {
    const row = opDiffRow(
      mkOp("term.status", {
        concept_id: "c-1",
        locale: "en-US",
        text: "utilize",
        from: "approved",
        to: "forbidden",
      }),
    );
    expect(row.verb).toBe("Ban");
    expect(row.summary).toBe("Ban “utilize” (en-US)");
    expect(row.tone).toBe("destructive");
    expect(row.governed).toBe(true);
    expect(row.category).toBe("term");
  });
  it("prefers a term with a success tone", () => {
    const row = opDiffRow(
      mkOp("term.status", {
        concept_id: "c-1",
        locale: "fr-FR",
        text: "Paiement",
        from: "proposed",
        to: "preferred",
      }),
    );
    expect(row.verb).toBe("Prefer");
    expect(row.summary).toBe("Prefer “Paiement” (fr-FR)");
    expect(row.tone).toBe("success");
  });
  it("summarises a voice rule with its replacement", () => {
    const row = opDiffRow(
      mkOp("voice.rule.add", {
        profile_id: "p-1",
        list: "preferred",
        rule: { term: "utilize", replacement: "use" },
      }),
    );
    expect(row.summary).toBe("Add preferred rule “utilize” → prefer “use”");
    expect(row.verb).toBe("Add preferred rule");
    expect(row.tone).toBe("success");
    expect(row.governed).toBe(true);
  });
  it("renders a relation as a readable phrase", () => {
    expect(
      opSummary(
        mkOp("relation.add", {
          relation: {
            id: "r-1",
            source_id: "cloud",
            target_id: "cloud-services",
            relation_type: "USE_INSTEAD",
            created_at: "2026-06-13T10:00:00Z",
          },
        }),
      ),
    ).toBe("cloud use instead cloud-services");
  });
  it("summarises a concept update by the field it touches", () => {
    expect(opSummary(mkOp("concept.update", { concept_id: "c-1", definition: "x" }))).toBe(
      "Edit concept c-1 (definition)",
    );
  });
});

describe("opCategory", () => {
  it("maps each op to its part of the graph", () => {
    expect(opCategory("term.status")).toBe("term");
    expect(opCategory("voice.rule.add")).toBe("voice");
    expect(opCategory("relation.remove")).toBe("relation");
    expect(opCategory("concept.create")).toBe("concept");
  });
});

describe("groupOps + governedOpCount", () => {
  it("groups ops by category in a stable order and counts governed ops", () => {
    seq = 0;
    const ops = [
      mkOp("concept.update", { concept_id: "c-1", definition: "d" }),
      mkOp("term.status", {
        concept_id: "c-1",
        locale: "en-US",
        text: "utilize",
        from: "approved",
        to: "forbidden",
      }),
      mkOp("voice.rule.add", { profile_id: "p-1", list: "forbidden", rule: { term: "utilize" } }),
      mkOp("relation.remove", { relation_id: "r-1" }),
    ];
    const groups = groupOps(ops);
    expect(groups.map((g) => g.category)).toEqual(["term", "voice", "relation", "concept"]);
    expect(groups[0].rows).toHaveLength(1);
    expect(CATEGORY_LABEL.term).toBe("Terms");
    expect(governedOpCount(ops)).toBe(2);
  });
});
