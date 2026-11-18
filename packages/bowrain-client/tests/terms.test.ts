import { describe, expect, it } from "vite-plus/test";

import { containerFromServer, conceptFromServer, unitTerms } from "../src/index.ts";

// The overlays as the server writes them (core/editor.OverlayView), for
// "Can't reach your server" / "Impossible de joindre le serveur" and a second
// rule the translation misses.
const container = containerFromServer({
  kind: "component",
  title: "OfflineLaunch",
  item_name: "OfflineLaunch.kbf.json",
  locale: "fr",
  focus: "o1",
  units: [
    {
      block_id: "o1",
      item_name: "OfflineLaunch.kbf.json",
      key: "k",
      role: "heading",
      source_runs: [{ text: "Can't reach your server, try again" }],
      target_runs: [{ text: "Impossible de joindre le serveur, réessayez" }],
      source_revision: "s",
      target_revision: "t",
      overlays: [
        {
          type: "term",
          side: "source",
          spans: [
            {
              id: "term-check:0",
              range: { kind: "range", start: { run: 0, offset: 17 }, end: { run: 0, offset: 23 } },
              text: "server",
              props: {
                satisfied: "true",
                term: "server",
                replacement: "serveur",
                concept_id: "c-server",
              },
            },
            {
              id: "term-check:1",
              range: { kind: "range", start: { run: 0, offset: 25 }, end: { run: 0, offset: 34 } },
              text: "try again",
              props: {
                satisfied: "false",
                term: "try again",
                replacement: "recommencer",
                advisory: "true",
              },
            },
          ],
        },
        {
          type: "term",
          side: "fr",
          spans: [
            {
              id: "term-check:0",
              range: { kind: "range", start: { run: 0, offset: 25 }, end: { run: 0, offset: 32 } },
              text: "serveur",
              props: {
                satisfied: "true",
                term: "server",
                replacement: "serveur",
                concept_id: "c-server",
              },
            },
          ],
        },
        { type: "qa", side: "source", spans: [{ id: "x", range: { kind: "block" } }] },
      ],
    },
  ],
});

describe("unitTerms", () => {
  it("pairs each term in the source with its renderings in the translation", () => {
    const [server, retry] = unitTerms(container.units[0]);
    expect(server).toMatchObject({
      id: "term-check:0",
      term: "server",
      replacement: "serveur",
      conceptId: "c-server",
      satisfied: true,
      advisory: false,
    });
    expect(server.source).toHaveLength(1);
    expect(server.target).toEqual([
      { kind: "range", start: { run: 0, offset: 25 }, end: { run: 0, offset: 32 } },
    ]);
    expect(retry).toMatchObject({
      term: "try again",
      satisfied: false,
      advisory: true,
      target: [],
    });
  });

  it("reads nothing from a unit with no term check spans", () => {
    expect(unitTerms({ ...container.units[0], overlays: undefined })).toEqual([]);
  });
});

describe("conceptFromServer", () => {
  it("maps a concept and defaults an unmarked term to approved", () => {
    expect(
      conceptFromServer({
        id: "c-server",
        definition: "The server a person signs in to.",
        terms: [
          { text: "server", locale: "en", status: "preferred" },
          { text: "serveur", locale: "fr" },
        ],
      }),
    ).toEqual({
      id: "c-server",
      domain: undefined,
      definition: "The server a person signs in to.",
      doNotTranslate: false,
      terms: [
        {
          text: "server",
          locale: "en",
          status: "preferred",
          partOfSpeech: undefined,
          note: undefined,
        },
        {
          text: "serveur",
          locale: "fr",
          status: "approved",
          partOfSpeech: undefined,
          note: undefined,
        },
      ],
    });
  });
});
