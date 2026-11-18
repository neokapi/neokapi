import type { ReviewContainer } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import type { ReactNode } from "react";

import "../lib/i18n";
import { InContextView } from "../components/InContextView";
import { markedParts } from "../components/RunText";

type Overlays = NonNullable<ReviewContainer["units"][0]["overlays"]>;

const range = (start: number, end: number) => ({
  kind: "range",
  start: { run: 0, offset: start },
  end: { run: 0, offset: end },
});

// The term check's spans as the server returns them: the app draws these and
// matches no text of its own.
const termSpans = (
  props: Record<string, string>,
  source: [number, number],
  target?: [number, number],
): Overlays => [
  { type: "term", side: "source", spans: [{ id: "term-check:0", range: range(...source), props }] },
  ...(target
    ? [
        {
          type: "term",
          side: "nb",
          spans: [{ id: "term-check:0", range: range(...target), props }],
        },
      ]
    : []),
];

const unit = (blockId: string, src: string, tgt: string, overlays: Overlays) => ({
  blockId,
  item: "OfflineLaunch.kbf.json",
  key: blockId,
  role: "text",
  source: [{ text: src }],
  target: [{ text: tgt }],
  waiting: true,
  sourceRevision: `s:${blockId}`,
  targetRevision: `r:${blockId}`,
  runtimeText: tgt,
  overlays,
});

const container: ReviewContainer = {
  kind: "component",
  title: "OfflineLaunch",
  item: "OfflineLaunch.kbf.json",
  locale: "nb",
  focus: "o1",
  moreBefore: false,
  moreAfter: false,
  units: [
    unit(
      "o1",
      "Can't reach your server",
      "Får ikke kontakt med serveren",
      termSpans(
        { term: "server", replacement: "server", concept_id: "c-server", satisfied: "true" },
        [17, 23],
        [21, 27],
      ),
    ),
    unit(
      "o2",
      "Open the workspace",
      "Åpne arbeidsrommet",
      termSpans(
        { term: "workspace", replacement: "arbeidsområde", concept_id: "c-ws", satisfied: "false" },
        [9, 18],
      ),
    ),
  ],
};

function wrap(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe("terms in a string", () => {
  it("marks the parts of the text runs an anchor covers, skipping codes", () => {
    const runs = [
      { text: "Reach " },
      { ph: { id: "1", type: "var", data: "{name}", equiv: "name" } },
      { text: "your server now" },
    ];
    const mark = {
      anchor: { ...range(0, 0), start: { run: 2, offset: 5 }, end: { run: 2, offset: 11 } },
      onPress: () => {},
    };
    const parts = markedParts(runs, [mark]);
    expect(parts.get(runs[2])).toEqual([{ start: 5, end: 11, mark }]);
    expect(parts.has(runs[0])).toBe(false);
  });

  it("opens a term from the string, says whether the translation uses it, and steps to the next", async () => {
    const loadConcept = jest.fn(async (id: string) => ({
      id,
      definition: id === "c-ws" ? "Where a team's projects live." : "The Bowrain server.",
      doNotTranslate: false,
      terms: [
        { text: id === "c-ws" ? "workspace" : "server", locale: "en", status: "preferred" },
        { text: id === "c-ws" ? "arbeidsområde" : "server", locale: "nb", status: "preferred" },
        { text: "tjener", locale: "nb", status: "forbidden" },
      ],
    }));
    await render(
      wrap(
        <InContextView
          card={{
            key: "o1:nb",
            blockId: "o1",
            item: "OfflineLaunch.kbf.json",
            locale: "nb",
            collectionId: "c1",
            source: [],
            target: [],
            targetRevision: "",
            group: "unsorted",
            notes: [],
            findingsCount: 0,
            failingFindingsCount: 0,
            ownWork: false,
          }}
          scope={{ workspace: "w", project: "p" }}
          load={async () => container}
          loadConcept={loadConcept}
          onApprove={() => {}}
          onFix={() => {}}
        />,
      ),
    );
    const marks = await screen.findAllByTestId("run-mark", {}, { timeout: 5000 });
    // The check's span in the translation: the rendering inside "serveren".
    expect(marks.map((m) => m.props.children)).toEqual(["server"]);
    await fireEvent.press(marks[0]);
    expect(screen.getByTestId("term-source").props.children).toBe("server");
    expect(await screen.findByText("The Bowrain server.")).toBeTruthy();
    expect(screen.getByTestId("term-used")).toBeTruthy();
    expect(screen.getByText("do not use")).toBeTruthy();

    await fireEvent.press(screen.getByTestId("term-next"));
    expect(screen.getByTestId("term-source").props.children).toBe("workspace");
    expect(screen.getByTestId("term-unused")).toBeTruthy();
    expect(loadConcept).toHaveBeenCalledWith("c-ws");
  });
});

describe("terms inside plural forms", () => {
  // "{count, plural, one {# arbeidsområde} other {# rom}}": the check found the
  // term in the `one` form and missed it in `other`.
  const one = [{ text: "1 arbeidsområde" }];
  const other = [{ text: "# rom" }];
  const target = [{ plural: { pivot: "count", forms: { one, other } } }];
  const pathed = (form: string, start: number, end: number) => ({
    kind: "range",
    path: [0, { plural: form }],
    start: { run: 0, offset: start },
    end: { run: 0, offset: end },
  });

  it("marks a term where its form is drawn", () => {
    const mark = { anchor: pathed("one", 2, 15), onPress: () => {} };
    const parts = markedParts(target, [mark]);
    expect(parts.get(one[0])).toEqual([{ start: 2, end: 15, mark }]);
    expect(parts.has(other[0])).toBe(false);
  });

  it("names the forms that miss the term", async () => {
    const props = {
      term: "workspace",
      replacement: "arbeidsområde",
      satisfied: "false",
      missing_forms: "other",
    };
    const plural: ReviewContainer = {
      ...container,
      units: [
        {
          ...unit("p1", "", "", []),
          source: [
            {
              plural: {
                pivot: "count",
                forms: { one: [{ text: "1 workspace" }], other: [{ text: "# workspaces" }] },
              },
            },
          ],
          target,
          overlays: [
            {
              type: "term",
              side: "source",
              spans: [
                { id: "term-check:0", range: pathed("one", 2, 11), props },
                { id: "term-check:0", range: pathed("other", 2, 12), props },
              ],
            },
            {
              type: "term",
              side: "nb",
              spans: [{ id: "term-check:0", range: pathed("one", 2, 15), props }],
            },
          ],
        },
      ],
    };
    await render(
      wrap(
        <InContextView
          card={{
            key: "p1:nb",
            blockId: "p1",
            item: "OfflineLaunch.kbf.json",
            locale: "nb",
            collectionId: "c1",
            source: [],
            target: [],
            targetRevision: "",
            group: "unsorted",
            notes: [],
            findingsCount: 0,
            failingFindingsCount: 0,
            ownWork: false,
          }}
          scope={{ workspace: "w", project: "p" }}
          load={async () => plural}
          onApprove={() => {}}
          onFix={() => {}}
        />,
      ),
    );
    const marks = await screen.findAllByTestId("run-mark", {}, { timeout: 5000 });
    expect(marks.map((m) => m.props.children)).toEqual(["arbeidsområde"]);
    await fireEvent.press(marks[0]);
    expect(screen.getByTestId("term-unused").props.children).toBe(
      "This translation does not use arbeidsområde in these forms: other.",
    );
  });
});
