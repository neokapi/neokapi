import { describe, expect, it } from "vitest";
import { marshalFile, parseFile } from "@neokapi/kapi-format";

import { ANATOMY_FILE, ANATOMY_LINES, ANATOMY_TEXT, TERMS } from "./kbfAnatomyDoc";

// The anatomy page explains a .kbf.json line by line, so the text it shows has
// to be the file the serializer writes, field order and all.
describe("the KBF anatomy example", () => {
  it("is the canonical form of its document", () => {
    expect(ANATOMY_TEXT).toBe(new TextDecoder().decode(marshalFile(ANATOMY_FILE)));
  });

  it("reads back as the document it shows", () => {
    expect(parseFile(ANATOMY_TEXT)).toEqual(
      parseFile(new TextDecoder().decode(marshalFile(ANATOMY_FILE))),
    );
  });

  it("tags every line with a term the page explains", () => {
    const terms = new Set(TERMS.map((term) => term.id));
    for (const line of ANATOMY_LINES) expect(terms.has(line.term), line.text).toBe(true);
  });
});
