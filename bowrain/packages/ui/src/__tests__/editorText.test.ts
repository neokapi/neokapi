import { describe, it, expect } from "vite-plus/test";
import { RUN_KINDS, type Run } from "@neokapi/kapi-format";

import { codeSpans, fromEditorText, toEditorText } from "../components/editor/editorText";

// The private-use markers the inline-code editor's text carries, one per chip.
const OPEN = "";
const CLOSE = "";
const PH = "";

/**
 * One run of every kind in RUN_KINDS. The seam must answer for each: a kind
 * added to the model fails this list's type until a sample is added here,
 * and the round-trip test below then says what the editor does with it.
 */
const SAMPLE: { [K in (typeof RUN_KINDS)[number]]: Run } = {
  text: { text: "Hello " },
  ph: { ph: { id: "0", type: "jsx:var", subType: "number", data: "{count}", equiv: "count" } },
  pcOpen: { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b", disp: "B" } },
  pcClose: { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
  sub: { sub: { id: "2", ref: "blk-9", equiv: "note" } },
  plural: {
    plural: { pivot: "n", forms: { one: [{ text: "one item" }], other: [{ text: "items" }] } },
  },
  select: {
    select: { pivot: "g", cases: { female: [{ text: "her" }], other: [{ text: "them" }] } },
  },
};

describe("toEditorText — runs into the editor's text and chips", () => {
  it("answers for every run kind", () => {
    for (const kind of RUN_KINDS) {
      // No kind is dropped: each contributes either text or a chip.
      const { codedText, spans } = toEditorText([SAMPLE[kind]]);
      expect(codedText.length + spans.length, kind).toBeGreaterThan(0);
    }
  });

  it("writes text as text and each inline code as one marker with its chip", () => {
    const { codedText, spans } = toEditorText([
      SAMPLE.text,
      SAMPLE.pcOpen,
      SAMPLE.ph,
      SAMPLE.pcClose,
      SAMPLE.sub,
    ]);
    expect(codedText).toBe(`Hello ${OPEN}${PH}${CLOSE}${PH}`);
    expect(spans.map((s) => s.span_type)).toEqual([
      "opening",
      "placeholder",
      "closing",
      "placeholder",
    ]);
    expect(spans[0]).toMatchObject({ type: "fmt:bold", id: "1", data: "<b>", equiv_text: "b" });
    expect(spans[1]).toMatchObject({ type: "jsx:var", sub_type: "number", equiv_text: "count" });
    // A subblock reference is a chip that names the block it refers to.
    expect(spans[3]).toMatchObject({ type: "sub", sub_type: "blk-9", equiv_text: "note" });
  });

  it("spells a plural or select inside a flat sequence in ICU, as text", () => {
    const { codedText, spans } = toEditorText([{ text: "You have " }, SAMPLE.plural]);
    expect(codedText).toBe("You have {n, plural, one {one item} other {items}}");
    expect(spans).toEqual([]);
    expect(toEditorText([SAMPLE.select]).codedText).toBe("{g, select, female {her} other {them}}");
  });

  it("reads an absent or empty sequence as an empty editor", () => {
    expect(toEditorText(undefined)).toEqual({ codedText: "", spans: [] });
    expect(toEditorText([])).toEqual({ codedText: "", spans: [] });
  });
});

describe("fromEditorText — an edit back into runs", () => {
  it("round-trips text, a placeholder, a paired code and a subblock reference", () => {
    const runs: Run[] = [SAMPLE.text, SAMPLE.pcOpen, SAMPLE.ph, SAMPLE.pcClose, SAMPLE.sub];
    const { codedText, spans } = toEditorText(runs);
    expect(fromEditorText(codedText, spans)).toEqual(runs);
  });

  it("reads a moved chip as the code it is, in its new place", () => {
    const { spans } = toEditorText([SAMPLE.text, SAMPLE.pcOpen, SAMPLE.pcClose]);
    // The person dragged the bold pair around "Hello".
    const runs = fromEditorText(`${OPEN}Hello${CLOSE} world`, spans);
    expect(runs).toEqual([SAMPLE.pcOpen, { text: "Hello" }, SAMPLE.pcClose, { text: " world" }]);
  });

  it("reads a plural edited as ICU text back as one text run", () => {
    // A flat editor has no per-form affordance: the ICU spelling it showed is
    // the text it gives back. A translation that is one plural never comes
    // through here; UnifiedTargetEditor opens one editor per form.
    const { codedText, spans } = toEditorText([SAMPLE.plural]);
    expect(fromEditorText(codedText, spans)).toEqual([
      { text: "{n, plural, one {one item} other {items}}" },
    ]);
  });
});

describe("codeSpans — the chips a sequence offers", () => {
  it("lists each inline code in order and no text", () => {
    const spans = codeSpans([SAMPLE.text, SAMPLE.pcOpen, SAMPLE.ph, SAMPLE.pcClose]);
    expect(spans.map((s) => s.id)).toEqual(["1", "0", "1"]);
  });

  it("is empty for text alone", () => {
    expect(codeSpans([SAMPLE.text])).toEqual([]);
    expect(codeSpans(undefined)).toEqual([]);
  });
});
