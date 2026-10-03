import { describe, expect, it } from "vitest";

import { decideOp, wordsInPlainText } from "../lib/changes";

describe("wordsInPlainText", () => {
  it.each([
    {
      name: "words in plain text",
      text: "Please utilize the dashboard",
      words: "utilize",
      want: true,
    },
    {
      name: "words beside a code",
      text: 'Please utilize <x id="1"/>the dashboard<x id="/1"/>',
      words: "utilize",
      want: true,
    },
    {
      name: "words inside a code's span",
      text: 'Please <x id="1"/>utilize the<x id="/1"/> dashboard',
      words: "utilize the",
      want: true,
    },
    {
      name: "words with a link among them",
      text: 'First read the <x id="1"/>setup guide<x id="/1"/> carefully.',
      words: "read the setup guide",
      want: false,
    },
    {
      name: "words with a placeholder among them",
      text: 'You have <x id="1/"/> items',
      words: "have  items",
      want: false,
    },
    {
      name: "words once in plain text and once across a code",
      text: 'use the gadget, use <x id="1"/>the gadget<x id="/1"/>',
      words: "use the gadget",
      want: false,
    },
    {
      name: "words that are gone",
      text: "Please use the dashboard",
      words: "utilize",
      want: false,
    },
    { name: "no words", text: "Please utilize the dashboard", words: "", want: false },
  ])("$name", ({ text, words, want }) => {
    expect(wordsInPlainText(text, words)).toBe(want);
  });
});

describe("decideOp", () => {
  // A parked draft reads as absent; the decision names it so, never "*", so a
  // file written meanwhile refuses it rather than taking it.
  it.each([
    { rev: "r:0123456789abcdef", want: "r:0123456789abcdef" },
    { rev: "absent", want: "absent" },
  ])("names the revision $rev read", ({ rev, want }) => {
    const op = decideOp({ doc: "src/en.json", block: "title", edition: "nl" }, rev, "establish");
    expect(op).toEqual({
      op: "decide",
      at: { doc: "src/en.json", block: "title", edition: "nl" },
      if_match: want,
      outcome: "establish",
    });
  });
});
