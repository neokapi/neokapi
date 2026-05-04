import { describe, expect, it } from "vitest";

import type { Run } from "../src/index.ts";
import { validatePairedMarkers } from "../src/runs-validate.ts";

function source(): Run[] {
  // "Click {=m0}here{/=m0} to read." with a standalone {=m1} icon.
  return [
    { text: "Click " },
    { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
    { text: "here" },
    { pcClose: { id: "0", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m0" } },
    { text: " " },
    {
      ph: {
        id: "2",
        type: "jsx:element",
        subType: "Icon",
        data: "<Icon/>",
        equiv: "=m1",
      },
    },
    { text: " to read." },
  ];
}

describe("validatePairedMarkers — well-formed targets", () => {
  it("returns no diagnostics for a target that mirrors the source structure", () => {
    const target: Run[] = [
      { text: "Klicken Sie " },
      { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
      { text: "hier" },
      { pcClose: { id: "0", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m0" } },
      { text: " " },
      {
        ph: {
          id: "2",
          type: "jsx:element",
          subType: "Icon",
          data: "<Icon/>",
          equiv: "=m1",
        },
      },
      { text: " zum Lesen." },
    ];
    expect(validatePairedMarkers(source(), target)).toEqual([]);
  });

  it("allows reordering paired pairs and standalones across the sentence", () => {
    const target: Run[] = [
      {
        ph: {
          id: "2",
          type: "jsx:element",
          subType: "Icon",
          data: "<Icon/>",
          equiv: "=m1",
        },
      },
      { text: " " },
      { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
      { text: "lies" },
      { pcClose: { id: "0", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m0" } },
      { text: " hier." },
    ];
    expect(validatePairedMarkers(source(), target)).toEqual([]);
  });
});

describe("validatePairedMarkers — drop / duplicate", () => {
  it("flags a paired pair dropped from the target", () => {
    const target: Run[] = [
      { text: "Klicken Sie hier " },
      {
        ph: {
          id: "2",
          type: "jsx:element",
          subType: "Icon",
          data: "<Icon/>",
          equiv: "=m1",
        },
      },
      { text: " zum Lesen." },
    ];
    const diags = validatePairedMarkers(source(), target);
    const dropped = diags.find((d) => d.kind === "dropped-pair");
    expect(dropped?.ref).toBe("0");
  });

  it("flags a duplicated paired marker", () => {
    const target: Run[] = [
      { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
      { text: "Klick" },
      { pcClose: { id: "0", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m0" } },
      { text: " " },
      { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
      { text: "hier" },
      { pcClose: { id: "0", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m0" } },
      {
        ph: {
          id: "2",
          type: "jsx:element",
          subType: "Icon",
          data: "<Icon/>",
          equiv: "=m1",
        },
      },
    ];
    const diags = validatePairedMarkers(source(), target);
    expect(diags.some((d) => d.kind === "duplicated-pair" && d.ref === "0")).toBe(true);
  });

  it("flags a dropped standalone placeholder", () => {
    const target: Run[] = [
      { text: "Klicken Sie " },
      { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
      { text: "hier" },
      { pcClose: { id: "0", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m0" } },
      { text: " zum Lesen." },
    ];
    const diags = validatePairedMarkers(source(), target);
    expect(diags.some((d) => d.kind === "dropped-standalone" && d.ref === "=m1")).toBe(true);
  });

  it("flags a duplicated standalone unless cloneable", () => {
    const cloneableSource: Run[] = [
      {
        ph: {
          id: "1",
          type: "jsx:var",
          data: "{name}",
          equiv: "name",
          constraints: { deletable: false, cloneable: true, reorderable: true },
        },
      },
    ];
    const target: Run[] = [
      { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
      { text: " — " },
      { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
    ];
    expect(validatePairedMarkers(cloneableSource, target)).toEqual([]);

    const nonCloneableSource: Run[] = [
      { ph: { id: "1", type: "jsx:var", data: "{name}", equiv: "name" } },
    ];
    const diags = validatePairedMarkers(nonCloneableSource, target);
    expect(diags.some((d) => d.kind === "duplicated-standalone" && d.ref === "name")).toBe(true);
  });
});

describe("validatePairedMarkers — well-formedness", () => {
  it("flags an unbalanced open with no close", () => {
    const target: Run[] = [
      { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
      { text: "hier" },
      {
        ph: {
          id: "2",
          type: "jsx:element",
          subType: "Icon",
          data: "<Icon/>",
          equiv: "=m1",
        },
      },
    ];
    const diags = validatePairedMarkers(source(), target);
    expect(diags.some((d) => d.kind === "unbalanced-open" && d.ref === "0")).toBe(true);
  });

  it("flags an unbalanced close with no open", () => {
    const target: Run[] = [
      { text: "hier" },
      { pcClose: { id: "9", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m9" } },
    ];
    const diags = validatePairedMarkers(source(), target);
    expect(diags.some((d) => d.kind === "unbalanced-close" && d.ref === "9")).toBe(true);
  });

  it("flags ill-nested closes", () => {
    const sourceNested: Run[] = [
      { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
      { pcOpen: { id: "1", type: "jsx:element", subType: "b", data: "<b>", equiv: "=m1" } },
      { text: "x" },
      { pcClose: { id: "1", type: "jsx:element", subType: "b", data: "</b>", equiv: "=m1" } },
      { pcClose: { id: "0", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m0" } },
    ];
    const target: Run[] = [
      { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
      { pcOpen: { id: "1", type: "jsx:element", subType: "b", data: "<b>", equiv: "=m1" } },
      { text: "x" },
      // Closes the outer before the inner → ill-nested.
      { pcClose: { id: "0", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m0" } },
      { pcClose: { id: "1", type: "jsx:element", subType: "b", data: "</b>", equiv: "=m1" } },
    ];
    const diags = validatePairedMarkers(sourceNested, target);
    expect(diags.some((d) => d.kind === "ill-nested")).toBe(true);
  });

  it("flags a paired pair whose inner content was removed when the source had inner content", () => {
    const target: Run[] = [
      { text: "Klicken Sie " },
      { pcOpen: { id: "0", type: "jsx:element", subType: "a", data: "<a>", equiv: "=m0" } },
      { pcClose: { id: "0", type: "jsx:element", subType: "a", data: "</a>", equiv: "=m0" } },
      { text: " " },
      {
        ph: {
          id: "2",
          type: "jsx:element",
          subType: "Icon",
          data: "<Icon/>",
          equiv: "=m1",
        },
      },
      { text: " zum Lesen." },
    ];
    const diags = validatePairedMarkers(source(), target);
    expect(diags.some((d) => d.kind === "empty-paired-inner" && d.ref === "0")).toBe(true);
  });
});
