import { describe, expect, it } from "vitest";

import type { CodeRead } from "@neokapi/contract-types";

import {
  codedToEditText,
  editTextToCoded,
  editTextToSegments,
} from "../components/editor/editText";
import { codedToRuns, runsToCoded } from "../components/editor/runsCodedBridge";

// The paragraph of the change contract's examples, as a read shows it.
const guide =
  'Read the <x id="1"/>shop guide<x id="/1"/> before you <x id="2"/>order<x id="/2"/>.';
const guideCodes: Record<string, CodeRead> = {
  "1": { kind: "paired", type: "link:hyperlink", attrs: { href: "https://old.example/guide" } },
  "2": { kind: "paired", type: "fmt:bold" },
};

describe("editTextToSegments", () => {
  it("makes a tag of each token, typed from the read's codes", () => {
    const segs = editTextToSegments(guide, guideCodes);
    expect(segs.map((s) => (s.type === "text" ? s.value : `[${s.spanInfo.span_type}:${s.spanInfo.id}:${s.spanInfo.type}]`))).toEqual([
      "Read the ",
      "[opening:1:link:hyperlink]",
      "shop guide",
      "[closing:1:link:hyperlink]",
      " before you ",
      "[opening:2:fmt:bold]",
      "order",
      "[closing:2:fmt:bold]",
      ".",
    ]);
  });

  it("names a placeholder by what it stands for", () => {
    const codes: Record<string, CodeRead> = {
      "n/": { kind: "placeholder", type: "jsx:var", equiv: "count", disp: "count" },
    };
    const [tag] = editTextToSegments('<x id="n/"/> items', codes);
    expect(tag).toEqual({
      type: "tag",
      spanInfo: {
        span_type: "placeholder",
        type: "jsx:var",
        id: "n",
        data: "",
        equiv_text: "count",
        display_text: "count",
      },
    });
  });

  it("keeps a token the codes do not list, for the service to judge", () => {
    const segs = editTextToSegments('a <x id="9/"/> b', {});
    expect(segs[1]).toMatchObject({ type: "tag", spanInfo: { span_type: "placeholder", id: "9" } });
  });

  it("reads text with no token as text alone", () => {
    expect(editTextToSegments("Plain words", {})).toEqual([{ type: "text", value: "Plain words" }]);
    expect(editTextToSegments("", {})).toEqual([]);
  });
});

describe("the round trip", () => {
  const cases: Array<{ name: string; text: string; codes: Record<string, CodeRead> }> = [
    { name: "paired codes", text: guide, codes: guideCodes },
    {
      name: "a placeholder",
      text: 'You have <x id="1/"/> new messages',
      codes: { "1/": { kind: "placeholder", type: "code:variable", equiv: "count" } },
    },
    {
      name: "a subblock reference",
      text: 'See <x id="sub:3"/> below',
      codes: { "sub:3": { kind: "subblock", attrs: { ref: "tu3" } } },
    },
    { name: "plain text", text: "Nothing coded here.", codes: {} },
    { name: "codes and nothing else", text: '<x id="1"/><x id="/1"/>', codes: guideCodes },
  ];
  for (const c of cases) {
    it(`gives back the edit text it read: ${c.name}`, () => {
      const { codedText, spans } = editTextToCoded(c.text, c.codes);
      expect(codedToEditText(codedText, spans)).toBe(c.text);
    });
    it(`survives the coded-text bridge to runs and back: ${c.name}`, () => {
      const { codedText, spans } = editTextToCoded(c.text, c.codes);
      const back = runsToCoded(codedToRuns(codedText, spans));
      expect(codedToEditText(back.codedText, back.spans)).toBe(c.text);
    });
  }

  it("carries an edit made between the codes", () => {
    const { codedText, spans } = editTextToCoded(guide, guideCodes);
    const edited = codedText.replace("shop guide", "handbook");
    expect(codedToEditText(edited, spans)).toBe(
      'Read the <x id="1"/>handbook<x id="/1"/> before you <x id="2"/>order<x id="/2"/>.',
    );
  });
});
