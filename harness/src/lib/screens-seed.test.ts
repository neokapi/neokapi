import test from "node:test";
import assert from "node:assert/strict";
import { keptBySeed, observationOf, queueShortfall, recordsNote, resetPoint } from "./screens-seed.ts";
import type { LoggedOp, SeedIdentity, SeedNote } from "./screens-seed.ts";

const who: SeedIdentity = { session: "harness-seed", person: "Alex Rivera" };
const agent = { kind: "agent", name: "claude-code", session: "harness-seed" };
const seedPerson = { kind: "person", name: "Alex Rivera" };
const webPerson = { kind: "person", name: "Demo User" };

const term: SeedNote = { term: "Help Center", insteadOf: "Help Centre", seenIn: "docs/a.md", quote: "q" };
const note: SeedNote = { text: "the guides address the reader as you", seenIn: "docs/a.md", quote: "q" };
const kept: SeedNote = { term: "dashboard", insteadOf: "home screen", seenIn: "docs/a.md", quote: "q" };

// The log of a seeded project, as `kapi context log --json` gives it after a sync.
const seeded: LoggedOp[] = [
  { id: "t1", kind: "observe", actor: agent, subject: { kind: "term", term: { term: "Help Centre", replacement: "Help Center" } }, at: "2026-11-02T20:10:37.4Z", status: "suggested" },
  { id: "n1", kind: "observe", actor: agent, subject: { kind: "note", text: note.text }, at: "2026-11-02T20:10:37.5Z", status: "suggested" },
  { id: "k1", kind: "observe", actor: agent, subject: { kind: "term", term: { term: "home screen", replacement: "dashboard" } }, at: "2026-11-02T20:10:37.6Z", status: "established" },
  { id: "keep1", kind: "keep", actor: seedPerson, target: "k1", at: "2026-11-02T20:10:49Z", status: "established" },
];

test("a term note matches on the wording it says to use, a prose note on its text", () => {
  assert.equal(recordsNote(seeded[0], term), true);
  assert.equal(recordsNote(seeded[0], kept), false);
  assert.equal(recordsNote(seeded[1], note), true);
  assert.equal(observationOf(seeded, who, kept)?.id, "k1");
  assert.equal(observationOf(seeded, who, { ...note, text: "another" }), undefined);
});

test("the seed's keep counts until a reset sets it aside", () => {
  assert.equal(keptBySeed(seeded, who, "k1"), true);
  assert.equal(keptBySeed(seeded, who, "t1"), false);
  const undone = seeded.map((op) => (op.id === "keep1" ? { ...op, status: "reset" } : op));
  assert.equal(keptBySeed(undone, who, "k1"), false);
});

test("a freshly seeded project has nothing to reset", () => {
  assert.equal(resetPoint(seeded, who, kept), undefined);
  assert.equal(resetPoint([], who, kept), undefined);
});

test("the take's keeps are reset from the first one on", () => {
  const taken: LoggedOp[] = [
    ...seeded,
    { id: "w1", kind: "keep", actor: webPerson, target: "t1", at: "2026-11-02T20:20:00Z", status: "established" },
    { id: "w2", kind: "keep", actor: webPerson, target: "n1", at: "2026-11-02T20:21:00Z", status: "established" },
  ];
  assert.equal(resetPoint(taken, who, kept)?.id, "w1");
});

test("a reset an earlier seed made stays in force, and only the newer take is reset", () => {
  const again: LoggedOp[] = [
    ...seeded,
    { id: "w1", kind: "keep", actor: webPerson, target: "t1", at: "2026-11-02T20:20:00Z", status: "reset" },
    { id: "r1", kind: "reset", actor: seedPerson, at: "2026-11-02T20:30:00Z", status: "established" },
    { id: "w3", kind: "keep", actor: webPerson, target: "n1", at: "2026-11-02T20:40:00Z", status: "established" },
  ];
  assert.equal(resetPoint(again, who, kept)?.id, "w3");
  const settled = again.map((op) => (op.id === "w3" ? { ...op, status: "reset" } : op));
  assert.equal(resetPoint(settled, who, kept), undefined);
});

test("a keep by the seed's person of a suggestion the seed did not keep is the take's", () => {
  const taken: LoggedOp[] = [
    ...seeded,
    { id: "w1", kind: "keep", actor: seedPerson, target: "t1", at: "2026-11-02T20:20:00Z", status: "established" },
  ];
  assert.equal(resetPoint(taken, who, kept)?.id, "w1");
});

test("the queue carries a take with a name, a note and a kept rule", () => {
  const two = [
    { id: "t1", kind: "name" },
    { id: "n1", kind: "note" },
  ];
  assert.equal(queueShortfall(two, 1), "");
  assert.match(queueShortfall([two[0]], 1), /1 suggestion/);
  assert.match(queueShortfall([two[0], { id: "t2", kind: "word" }], 1), /no note waiting/);
  assert.match(queueShortfall(two, 0), /Kept by starts empty/);
});

test("stamps in one second compare as instants, not as text", () => {
  // "…37Z" is the whole second, and as text it sorts after "…37.4Z".
  const sameSecond: LoggedOp[] = [
    {
      id: "o1",
      kind: "observe",
      actor: agent,
      subject: { kind: "term", term: { term: "home screen", replacement: "dashboard" } },
      at: "2026-11-02T20:10:37Z",
      status: "established",
    },
    { id: "w1", kind: "keep", actor: webPerson, target: "x", at: "2026-11-02T20:10:37.4Z", status: "established" },
  ];
  assert.equal(resetPoint(sameSecond, who, kept)?.id, "w1");
});

test("every keep the seed made of its kept rule stays, even after something set one aside", () => {
  const rekept: LoggedOp[] = [
    ...seeded.map((op) => (op.id === "keep1" ? { ...op, status: "reset" } : op)),
    { id: "r0", kind: "reset", actor: webPerson, at: "2026-11-02T20:15:00Z", status: "established" },
    { id: "keep2", kind: "keep", actor: seedPerson, target: "k1", at: "2026-11-02T20:16:00Z", status: "established" },
  ];
  assert.equal(resetPoint(rekept, who, kept), undefined);
  const taken: LoggedOp[] = [
    ...rekept,
    { id: "w1", kind: "keep", actor: webPerson, target: "t1", at: "2026-11-02T20:20:00Z", status: "established" },
  ];
  assert.equal(resetPoint(taken, who, kept)?.id, "w1");
});
