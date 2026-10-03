import { describe, it, expect, vi, afterEach } from "vite-plus/test";
import type { ChangeResult } from "@neokapi/contract-types";
import type { Run } from "@neokapi/kapi-format";
import { RestApiAdapter } from "../api/rest-adapter";
import {
  addNote,
  appendText,
  contentChangeSet,
  decideTranslation,
  decisionOutcome,
  markEntity,
  placeholderText,
  readOutcome,
  removeNote,
  renderedRevision,
  setTranslation,
  textRangeAnchor,
  toChangeRuns,
  translationRuns,
} from "../api/contentChanges";
import type { BlockInfo } from "../types/api";

/**
 * The operations the editor surfaces send, and how they read the answer: each
 * names the revision the surface rendered, carries no native code data, and a
 * stale refusal is read with the translation as it stands.
 */

const block: BlockInfo = {
  id: "b1",
  source: "Read the guide",
  source_runs: [
    { text: "Read the " },
    { pcOpen: { id: "1", type: "link:hyperlink", data: '<a href="/g">', equiv: "a" } },
    { text: "guide" },
    { pcClose: { id: "1", type: "link:hyperlink", data: "</a>", equiv: "a" } },
  ],
  targets: { fr: { text: "Lire le guide", status: "translated" } },
  translatable: true,
  has_spans: true,
  properties: {},
  target_revisions: { fr: "r:00000000000000aa", de: "absent" },
};

describe("content change operations", () => {
  it("name the revision the surface rendered", () => {
    expect(renderedRevision(block, "fr")).toBe("r:00000000000000aa");
    expect(renderedRevision(block, "de")).toBe("absent");
    // A block read without revisions has nothing to guard a translation it holds.
    const unguarded = { ...block, target_revisions: undefined };
    expect(renderedRevision(unguarded, "es")).toBe("absent");
    expect(() => renderedRevision(unguarded, "fr")).toThrow(/without the revision/);
  });

  it("send runs without native code data", () => {
    const runs = block.source_runs as Run[];
    expect(toChangeRuns(runs)).toEqual([
      { text: "Read the " },
      { pcOpen: { id: "1", type: "link:hyperlink", equiv: "a" } },
      { text: "guide" },
      { pcClose: { id: "1", type: "link:hyperlink", equiv: "a" } },
    ]);
    const op = setTranslation("guide.md", block, "fr", { runs });
    expect(op).toMatchObject({
      op: "set_content",
      at: { doc: "guide.md", block: "b1", edition: "fr" },
      if_match: "r:00000000000000aa",
    });
    expect(JSON.stringify(op)).not.toContain('"data"');
    expect(placeholderText(runs)).toBe('Read the <x id="1"/>guide<x id="/1"/>');
  });

  it("map a review control to its decision", () => {
    expect(decisionOutcome(true)).toBe("establish");
    expect(decisionOutcome(false, "draft")).toBe("reject");
    expect(decisionOutcome(false)).toBe("withdraw");
    expect(decideTranslation("guide.md", block, "fr", "reject")).toEqual({
      op: "decide",
      at: { doc: "guide.md", block: "b1", edition: "fr" },
      if_match: "r:00000000000000aa",
      outcome: "reject",
    });
  });

  it("put notes on the source", () => {
    expect(addNote("guide.md", "b1", "Check the link")).toEqual({
      op: "annotate",
      at: { doc: "guide.md", block: "b1" },
      type: "note",
      value: { text: "Check the link" },
    });
    expect(removeNote("guide.md", "b1", "note-1")).toEqual({
      op: "unannotate",
      at: { doc: "guide.md", block: "b1" },
      type: "note",
      id: "note-1",
    });
  });

  it("anchor an entity to the run positions of the words marked", () => {
    // "guide" is the third run. A position at the end of a run's text is the
    // start of the run after it, as model.RangeAnchor places it: the start
    // falls on the link's opening code, the end on its closing code.
    expect(textRangeAnchor(block.source_runs as Run[], block.source, 9, 14)).toEqual({
      kind: "range",
      start: { run: 1, offset: 0 },
      end: { run: 3, offset: 0 },
    });
    // Offsets count code points, as the engine counts them.
    expect(textRangeAnchor([{ text: "Café 😀 ici" }], "Café 😀 ici", 8, 11)).toEqual({
      kind: "range",
      start: { run: 0, offset: 7 },
      end: { run: 1, offset: 0 },
    });
    const op = markEntity("guide.md", block, {
      text: "guide",
      type: "product",
      start: 9,
      end: 14,
      dnt: true,
    });
    expect(op.value).toEqual({
      Text: "guide",
      Type: "product",
      Locale: "",
      DNT: true,
      Source: "manual",
    });
  });

  it("read a stale refusal with the translation as it stands", () => {
    const stale: ChangeResult = {
      schema: "kapi.change-result/v1",
      status: "refused",
      record: null,
      docs: [],
      ops: [
        {
          i: 0,
          op: "set_content",
          status: "refused",
          error: { code: "stale", message: "edition fr moved" },
          current: { rev: "r:00000000000000bb", text: "Lisez le guide" },
        },
      ],
    };
    const outcome = readOutcome(stale);
    expect(outcome.status).toBe("stale");
    if (outcome.status === "stale") expect(outcome.current.text).toBe("Lisez le guide");

    const refused = readOutcome({
      ...stale,
      ops: [
        {
          i: 0,
          op: "set_content",
          status: "refused",
          error: { code: "not_permitted", message: "no" },
        },
      ],
    });
    expect(refused).toMatchObject({ status: "refused", error: { code: "not_permitted" } });

    // A save the checks alone refused carries the failing findings, which a
    // person may override; a set refused for that and for something else is
    // not theirs to override.
    const finding = { rule: "terms.vocabulary", message: "Use réglages", fails: true };
    const advisory = { rule: "voice.tone", message: "Reads formal", fails: false };
    const gated = {
      ...stale,
      ops: [
        {
          i: 0,
          op: "set_content" as const,
          status: "refused" as const,
          error: { code: "gate_failed" as const, message: "the edit introduces 1 failing finding" },
          findings: [finding, advisory],
        },
      ],
    };
    expect(readOutcome(gated)).toMatchObject({ status: "gate_failed", findings: [finding] });
    const mixed = readOutcome({
      ...gated,
      ops: [
        ...gated.ops,
        {
          i: 1,
          op: "decide",
          status: "refused",
          error: { code: "not_permitted", message: "no" },
        },
      ],
    });
    expect(mixed.status).toBe("refused");
  });

  it("send a person's override of a failing check as gate report", () => {
    const op = decideTranslation("guide.md", block, "fr", "establish");
    expect(contentChangeSet([op])).not.toHaveProperty("gate");
    expect(contentChangeSet([op], { gate: "report", note: "save anyway" })).toMatchObject({
      gate: "report",
      note: "save anyway",
    });
  });
});

describe("a translation's runs", () => {
  it("are the served runs, else the coded text, else the text", () => {
    expect(translationRuns({ ...block, targets_runs: { fr: [{ text: "Lire" }] } }, "fr")).toEqual([
      { text: "Lire" },
    ]);
    expect(translationRuns(block, "fr")).toEqual([{ text: "Lire le guide" }]);
    expect(translationRuns(block, "de")).toEqual([]);
  });

  it("take appended words after their codes, joining a trailing text run", () => {
    const close = { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } };
    expect(appendText([], "guide")).toEqual([{ text: "guide" }]);
    expect(appendText([{ text: "Lire le" }], "guide")).toEqual([{ text: "Lire le guide" }]);
    expect(appendText([{ text: "Lire" }, close], "guide")).toEqual([
      { text: "Lire" },
      close,
      { text: " guide" },
    ]);
    // Words a person adds are translatable, so a do-not-translate run stays as it is.
    expect(appendText([{ text: "kapi", noTranslate: true }], "CLI")).toEqual([
      { text: "kapi", noTranslate: true },
      { text: " CLI" },
    ]);
  });
});

describe("RestApiAdapter.applyChanges", () => {
  afterEach(() => vi.unstubAllGlobals());

  function answer(status: number, body: unknown) {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: status >= 200 && status < 300,
      status,
      json: async () => body,
      text: async () => JSON.stringify(body),
      headers: { get: () => null },
    });
    vi.stubGlobal("fetch", fetchMock);
    return fetchMock;
  }

  const set = contentChangeSet([decideTranslation("guide.md", block, "fr", "establish")]);

  it("posts the change set to the stream's changes route", async () => {
    const fetchMock = answer(200, {
      schema: "kapi.change-result/v1",
      status: "applied",
      record: "c1",
      docs: [],
      ops: [],
    });
    const res = await new RestApiAdapter("http://server").applyChanges("acme", "p 1", set, "dev");
    expect(res.status).toBe("applied");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://server/api/v1/acme/projects/p%201/streams/dev/changes");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual(set);
  });

  it("answers a refusal with its result rather than an error", async () => {
    const refusal = {
      schema: "kapi.change-result/v1",
      status: "refused",
      record: null,
      docs: [],
      ops: [
        {
          i: 0,
          op: "decide",
          status: "refused",
          error: { code: "stale", message: "moved" },
          current: { rev: "r:00000000000000bb", text: "Lisez le guide" },
        },
      ],
    };
    answer(409, refusal);
    const res = await new RestApiAdapter("http://server").applyChanges("acme", "p1", set);
    expect(res).toEqual(refusal);
  });

  it("rejects an answer that is no change result", async () => {
    answer(403, { error: "forbidden", message: "no access to the project" });
    await expect(
      new RestApiAdapter("http://server").applyChanges("acme", "p1", set),
    ).rejects.toThrow(/no access to the project/);
  });
});
