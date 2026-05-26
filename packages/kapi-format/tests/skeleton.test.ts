import { describe, expect, it } from "vitest";

import type { Document, File, Skeleton } from "../src/index.ts";
import { Kind, marshalFile, SchemaVersion } from "../src/index.ts";

// The KLF Skeleton shape is canonicalized in Go (core/klf.Skeleton =
// { ref, inline }). The TypeScript mirror must agree on the field
// names — issue #717 fixed a drift where TS carried { ref, digest? }.
// These tests pin the canonical shape so the drift can't recur.

function fileWithSkeleton(skeleton: Skeleton | undefined): File {
  const doc: Document = {
    id: "doc1",
    documentType: "jsx",
    path: "src/App.tsx",
    ...(skeleton ? { skeleton } : {}),
    blocks: [
      {
        id: "b1",
        hash: "abc",
        translatable: true,
        type: "jsx:element",
        source: [{ text: "Hello" }],
        placeholders: [],
        properties: { file: "src/App.tsx", line: 1, component: "App", jsxPath: "App", element: "h1" },
      },
    ],
  };
  return {
    schemaVersion: SchemaVersion,
    kind: Kind,
    generator: { id: "test", version: "1.0" },
    project: { id: "p", sourceLocale: "en" },
    documents: [doc],
  };
}

function marshalText(file: File): string {
  return new TextDecoder().decode(marshalFile(file));
}

describe("Skeleton canonical shape (ref + inline, matching Go)", () => {
  it("serializes both ref and inline, in that order", () => {
    const text = marshalText(fileWithSkeleton({ ref: "skel://1", inline: "<root>{0}</root>" }));
    const parsed = JSON.parse(text);
    const skel = parsed.documents[0].skeleton;
    expect(skel).toEqual({ ref: "skel://1", inline: "<root>{0}</root>" });
    // Field order is normative: ref before inline.
    expect(Object.keys(skel)).toEqual(["ref", "inline"]);
    // The retired field name must never appear.
    expect(text).not.toContain("digest");
  });

  it("emits only ref when inline is absent", () => {
    const parsed = JSON.parse(marshalText(fileWithSkeleton({ ref: "skel://only" })));
    expect(parsed.documents[0].skeleton).toEqual({ ref: "skel://only" });
  });

  it("emits only inline when ref is absent", () => {
    const parsed = JSON.parse(marshalText(fileWithSkeleton({ inline: "payload" })));
    expect(parsed.documents[0].skeleton).toEqual({ inline: "payload" });
  });

  it("omits the skeleton entirely when the document has none", () => {
    const parsed = JSON.parse(marshalText(fileWithSkeleton(undefined)));
    expect(parsed.documents[0]).not.toHaveProperty("skeleton");
  });

  it("round-trips a skeleton-carrying file through marshal/parse losslessly", () => {
    const input = fileWithSkeleton({ ref: "skel://rt", inline: "inline-body" });
    const a = marshalText(input);
    // Re-marshal a structurally-equivalent file rebuilt from the parse.
    const parsed = JSON.parse(a) as File;
    const b = marshalText(parsed);
    expect(b).toEqual(a);
  });
});
