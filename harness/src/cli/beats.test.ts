import test from "node:test";
import assert from "node:assert/strict";
import type { DemoManifest } from "../types.ts";
import { beatsFor } from "./beats.ts";

const manifest: DemoManifest = {
  id: "walk",
  title: "A claim",
  subtitle: "One line.",
  aspects: [],
  kind: "use-case",
  needsAi: false,
  terminal: "desktop",
  brand: "desktop",
  outro: { line: "Try it", pointer: "kapi up" },
  artifacts: [],
  narration: [
    { id: "title", kind: "title", text: "Hello." },
    { id: "a", kind: "desktop", beat: "a", text: "First.", caption: " The consequence ", hold: 5, crop: { selector: "[data-x]" }, zoom: 1.5, highlight: { selector: "[data-y]" } },
    { id: "b", kind: "desktop", beat: "b", text: "Second.", crop: { x: 0.1, y: 0.1, w: 0.5, h: 0.5 } },
    { id: "outro", kind: "outro", text: "Bye." },
  ],
  locales: {
    nb: { title: "En påstand", narration: [{ id: "a", text: "Først.", caption: "Konsekvensen" }] },
  },
};

test("beatsFor carries the cards and each scene's picture fields", () => {
  const b = beatsFor(manifest);
  assert.deepEqual(b.cards, { title: "A claim", subtitle: "One line.", outroLine: "Try it", outroPointer: "kapi up" });
  assert.equal(b.locale, undefined);
  const [, a, bScene] = b.scenes;
  assert.deepEqual(a, { id: "a", kind: "desktop", caption: "The consequence", beat: "a", hold: 5, zoom: 1.5, highlight: { selector: "[data-y]" } });
  assert.deepEqual(bScene, { id: "b", kind: "desktop", beat: "b", crop: { x: 0.1, y: 0.1, w: 0.5, h: 0.5 } });
});

test("beatsFor takes a locale's title and captions from its sidecar overlay", () => {
  const b = beatsFor(manifest, "nb");
  assert.equal(b.locale, "nb");
  assert.equal(b.cards.title, "En påstand");
  assert.equal(b.cards.subtitle, "One line.");
  assert.equal(b.scenes[1]!.caption, "Konsekvensen");
});

test("beatsFor defaults the outro to the title and the brand's site", () => {
  const b = beatsFor({ ...manifest, outro: undefined, brand: "bowrain" });
  assert.equal(b.cards.outroLine, "A claim");
  assert.equal(b.cards.outroPointer, "bowrain.cloud");
});
