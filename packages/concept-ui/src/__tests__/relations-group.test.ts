import { describe, expect, it } from "vitest";
import {
  DEFAULT_RELATIONS_COLLAPSE,
  RELATION_DISPLAY_ORDER,
  buildRelationView,
  neighbourIds,
} from "../relations-group";
import type { Relation } from "../types";

const rel = (id: string, sourceId: string, targetId: string, type: Relation["type"]): Relation => ({
  id,
  sourceId,
  targetId,
  type,
});

describe("buildRelationView", () => {
  it("orders lanes by reading order, not canonical type order", () => {
    const relations = [
      rel("a", "x", "rival", "COMPETITOR"),
      rel("b", "x", "y", "RELATED"),
      rel("c", "x", "z", "USE_INSTEAD"),
    ];
    const views = buildRelationView(relations, "x");
    expect(views.map((v) => v.type)).toEqual(["USE_INSTEAD", "RELATED", "COMPETITOR"]);
    expect(RELATION_DISPLAY_ORDER.indexOf("USE_INSTEAD")).toBeLessThan(
      RELATION_DISPLAY_ORDER.indexOf("COMPETITOR"),
    );
  });

  it("resolves the neighbour on the far end with direction", () => {
    const views = buildRelationView(
      [rel("a", "x", "y", "RELATED"), rel("b", "z", "x", "BROADER")],
      "x",
    );
    const broader = views.find((v) => v.type === "BROADER")!;
    expect(broader.items[0].otherId).toBe("z");
    expect(broader.items[0].outgoing).toBe(false);
    const related = views.find((v) => v.type === "RELATED")!;
    expect(related.items[0].otherId).toBe("y");
    expect(related.items[0].outgoing).toBe(true);
  });

  it("marks a lane collapsed only past the threshold", () => {
    const many = Array.from({ length: DEFAULT_RELATIONS_COLLAPSE + 1 }, (_, i) =>
      rel(`r${i}`, "x", `t${i}`, "RELATED"),
    );
    const [collapsed] = buildRelationView(many, "x");
    expect(collapsed.count).toBe(DEFAULT_RELATIONS_COLLAPSE + 1);
    expect(collapsed.collapsed).toBe(true);

    const [inline] = buildRelationView(many.slice(0, DEFAULT_RELATIONS_COLLAPSE), "x");
    expect(inline.collapsed).toBe(false);
  });

  it("honours an explicit threshold", () => {
    const three = [
      rel("a", "x", "p", "RELATED"),
      rel("b", "x", "q", "RELATED"),
      rel("c", "x", "r", "RELATED"),
    ];
    expect(buildRelationView(three, "x", 2)[0].collapsed).toBe(true);
    expect(buildRelationView(three, "x", 3)[0].collapsed).toBe(false);
  });
});

describe("neighbourIds", () => {
  it("collects distinct neighbour ids across lanes in first-seen order", () => {
    const views = buildRelationView(
      [
        rel("a", "x", "y", "RELATED"),
        rel("b", "x", "z", "USE_INSTEAD"),
        rel("c", "x", "y", "BROADER"), // y again, via another lane
      ],
      "x",
    );
    // Reading order puts USE_INSTEAD first (z), then BROADER (y), then RELATED (y).
    expect(neighbourIds(views)).toEqual(["z", "y"]);
  });
});
