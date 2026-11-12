import { describe, expect, it } from "vite-plus/test";
import type { CodeRead, Run } from "@neokapi/contract-types";

import { labelCodesFromRuns } from "../components/editor/codeLabels";
import { editTextToSegments } from "../components/editor/editText";
import { semanticLabel } from "../components/editor/tagSemantics";

const name = { id: "c1", type: "code", data: "{{name}}", equiv: "" };

describe("labelCodesFromRuns", () => {
  it("labels a code finder's placeholder with its native form", () => {
    const codes: Record<string, CodeRead> = { "c1/": { kind: "placeholder", type: "code" } };
    const runs: Run[] = [{ text: "Failed to cordon node " }, { ph: name }];
    const labelled = labelCodesFromRuns(codes, runs);
    expect(labelled?.["c1/"].disp).toBe("{{name}}");

    // The editor's chip reads the label rather than the type.
    const [, tag] = editTextToSegments('Failed to cordon node <x id="c1/"/>', labelled);
    expect(tag.type).toBe("tag");
    if (tag.type === "tag") expect(semanticLabel(tag.spanInfo)).toBe("{{name}}");
  });

  it("reads placeholders inside a plural's branches", () => {
    const codes: Record<string, CodeRead> = { "c1/": { kind: "placeholder", type: "code" } };
    const runs: Run[] = [
      {
        plural: {
          pivot: "count",
          forms: { one: [{ text: "one node" }], other: [{ ph: name }, { text: " nodes" }] },
        },
      },
    ];
    expect(labelCodesFromRuns(codes, runs)?.["c1/"].disp).toBe("{{name}}");
  });

  it("leaves a code the read already labels, or one its vocabulary names", () => {
    const codes: Record<string, CodeRead> = {
      "c1/": { kind: "placeholder", type: "code", equiv: "user" },
      "2/": { kind: "placeholder", type: "struct:break" },
    };
    const runs: Run[] = [
      { ph: name },
      { ph: { id: "2", type: "struct:break", data: "<br/>", equiv: "" } },
    ];
    expect(labelCodesFromRuns(codes, runs)).toBe(codes);
  });
});
