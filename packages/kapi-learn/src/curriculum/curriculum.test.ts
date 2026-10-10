import { describe, expect, it } from "vitest";
import {
  LABS,
  SERIES,
  SAMPLES,
  isExplorerLab,
  isPlaygroundLab,
  labById,
  labsInSeries,
  nextLab,
  previousLab,
} from "./index.ts";
import { SAMPLE_TREES } from "../samples.gen.ts";

describe("curriculum", () => {
  it("names every lab and chapter once, with URL-safe ids", () => {
    const ids = new Set<string>();
    for (const lab of LABS) {
      expect(lab.id).toMatch(/^[a-z0-9-]+$/);
      expect(ids.has(lab.id)).toBe(false);
      ids.add(lab.id);
      const chapterIds = new Set<string>();
      for (const ch of lab.chapters) {
        expect(ch.id).toMatch(/^[a-z0-9-]+$/);
        expect(chapterIds.has(ch.id), `${lab.id}: duplicate chapter ${ch.id}`).toBe(false);
        chapterIds.add(ch.id);
      }
    }
  });

  it("numbers each series contiguously from 1", () => {
    for (const s of SERIES) {
      const labs = labsInSeries(s.id);
      expect(labs.length).toBeGreaterThan(0);
      expect(labs.map((l) => l.position)).toEqual(labs.map((_, i) => i + 1));
      // The explorer series runs on the engine's own fixtures, except the free
      // terminal, which opens a sample project to type into.
      if (s.id !== "explore") for (const lab of labs) expect(lab.sample).toBe(s.sample);
    }
  });

  it("runs every terminal lab in a sample the generator carries", () => {
    for (const lab of LABS) {
      expect(SAMPLES[lab.sample]).toBeDefined();
      if (isExplorerLab(lab)) continue;
      const tree = (SAMPLE_TREES as Record<string, readonly unknown[]>)[lab.sample];
      expect(tree?.length, `${lab.id}: no sample tree for ${lab.sample}`).toBeGreaterThan(0);
    }
  });

  it("gives every chapter something to run or show, and an expectation where a command runs", () => {
    for (const lab of LABS) {
      for (const ch of lab.chapters) {
        const has =
          ch.command || ch.look?.file || ch.files?.length || (lab.kind === "explorer" && ch.stage);
        expect(has, `${lab.id}/${ch.id}`).toBeTruthy();
        expect(ch.narration.length).toBeGreaterThan(20);
        if (ch.exit !== undefined) expect(ch.command).toBeTruthy();
        if (isExplorerLab(lab)) expect(ch.command, `${lab.id}/${ch.id}`).toBeUndefined();
      }
      if (lab.kind === "explorer") expect(lab.explorer, `${lab.id}: no explorer`).toBeTruthy();
      if (!isPlaygroundLab(lab)) expect(lab.chapters.length, `${lab.id}`).toBeGreaterThan(0);
    }
  });

  it("gives a playground no chapters and something to try", () => {
    const playgrounds = LABS.filter(isPlaygroundLab);
    expect(playgrounds.map((l) => l.id)).toEqual(["structure", "vision", "free-terminal"]);
    for (const lab of playgrounds) {
      expect(lab.chapters).toEqual([]);
      expect(lab.tryNext?.length ?? 0).toBeGreaterThan(1);
      // An explorer playground states what its stage shows; a terminal one
      // opens the sample and offers commands.
      if (lab.explorer) expect(lab.stage).toBeDefined();
      else for (const t of lab.tryNext ?? []) expect(t.startsWith("kapi ")).toBe(true);
    }
  });

  it("links labs in order, across series", () => {
    expect(previousLab(LABS[0].id)).toBeUndefined();
    expect(nextLab(LABS[LABS.length - 1].id)).toBeUndefined();
    expect(nextLab("northsea-edit")?.id).toBe("compass-axis");
    expect(labById("compass-review")?.series).toBe("add-languages");
  });

  it("keeps the prose free of em dashes", () => {
    for (const lab of LABS) {
      const text = [
        lab.title,
        lab.tagline,
        lab.summary,
        ...lab.chapters.flatMap((c) => [c.title, c.narration, c.note ?? ""]),
      ].join("\n");
      expect(text.includes("—"), `${lab.id} uses an em dash`).toBe(false);
    }
  });
});
