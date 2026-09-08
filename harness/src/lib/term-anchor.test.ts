import test from "node:test";
import assert from "node:assert/strict";
import {
  blocksCarrying,
  decidedMatches,
  describeMissingAnchor,
  describeTermAnchor,
  lookupCandidates,
  pickTermAnchor,
} from "./term-anchor.ts";
import type { SourceBlock, TermMatch } from "./term-anchor.ts";

// about-us.html as the editor's block list returns it, to the first few blocks
// the governance walk steps through. The heading carries a content-memory match
// and no term, which is the block the walk used to stop on (#2605).
const blocks: SourceBlock[] = [
  { id: "b1", source: "About Acme Inc." },
  { id: "b2", source: "Building the future of cloud infrastructure since 2018." },
  { id: "b3", source: "Our Mission" },
  { id: "b4", translatable: false, source: "About Us - Acme Inc." },
  { id: "b5", source: "   " },
];

const cloud: TermMatch = {
  source_term: "cloud infrastructure",
  target_terms: ["infrastructure cloud"],
  status: "approved",
};

test("only translatable blocks with text are worth a term lookup", () => {
  assert.deepEqual(
    lookupCandidates(blocks).map((b) => b.id),
    ["b1", "b2", "b3"],
  );
});

test("a match with no target wording is not a decision the sidebar can show", () => {
  const undecided: TermMatch[] = [
    { source_term: "uptime", target_terms: [] },
    { source_term: "encryption", target_terms: null },
    { source_term: "cloud infrastructure", target_terms: ["  "] },
    { source_term: "", target_terms: ["disponibilité"] },
  ];
  assert.deepEqual(decidedMatches(undecided), []);
  assert.deepEqual(decidedMatches([...undecided, cloud]), [cloud]);
});

test("the anchor is the first block the terms store decides for", () => {
  const anchor = pickTermAnchor(lookupCandidates(blocks), (id) => (id === "b2" ? [cloud] : []));
  assert.equal(anchor?.block.id, "b2");
  assert.equal(anchor?.term, "cloud infrastructure");
  assert.deepEqual(anchor?.targets, ["infrastructure cloud"]);
});

test("a block whose only match has no target wording is passed over", () => {
  const matches: Record<string, TermMatch[]> = {
    b1: [{ source_term: "Acme", target_terms: [] }],
    b2: [cloud],
  };
  const anchor = pickTermAnchor(lookupCandidates(blocks), (id) => matches[id] ?? []);
  assert.equal(anchor?.block.id, "b2");
});

test("no decided match anywhere leaves no anchor", () => {
  assert.equal(pickTermAnchor(lookupCandidates(blocks), () => []), null);
});

test("describeTermAnchor names the block, the term and the agreed wording", () => {
  const anchor = pickTermAnchor(lookupCandidates(blocks), (id) => (id === "b2" ? [cloud] : []))!;
  assert.equal(
    describeTermAnchor(anchor, "fr"),
    'block b2 carries "cloud infrastructure", agreed as infrastructure cloud in fr',
  );
});

test("blocksCarrying finds the words whatever their case", () => {
  assert.deepEqual(
    blocksCarrying(blocks, "Cloud Infrastructure").map((b) => b.id),
    ["b2"],
  );
  assert.deepEqual(blocksCarrying(blocks, "   "), []);
});

test("a file that lost the words is reported as the file drifting", () => {
  const drifted = [{ id: "b1", source: "About Acme Inc." }];
  assert.match(
    describeMissingAnchor("about-us.html", "cloud infrastructure", "fr", drifted, () => []),
    /has no block reading "cloud infrastructure"/,
  );
});

test("a file with the words and an empty terms store is reported as the store", () => {
  assert.match(
    describeMissingAnchor(
      "about-us.html",
      "cloud infrastructure",
      "fr",
      lookupCandidates(blocks),
      () => [],
    ),
    /carries "cloud infrastructure" on block b2, and the terms store answers no match/,
  );
});

test("matches without a target wording are reported as the missing wording", () => {
  assert.match(
    describeMissingAnchor("about-us.html", "cloud infrastructure", "fr", lookupCandidates(blocks), (id) =>
      id === "b2" ? [{ source_term: "cloud infrastructure", target_terms: [] }] : [],
    ),
    /none of those terms has a fr wording/,
  );
});
