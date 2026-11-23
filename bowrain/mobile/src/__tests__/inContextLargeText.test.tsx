// At the largest text sizes the in-context view's header (the kind, the
// title, the file and the reading switch) is taller than the sheet. It then
// scrolls with the strings, so every string stays reachable; at ordinary
// sizes it stands above them.
import type { DeckCard, ReviewContainer } from "@neokapi/bowrain-client";
import { render, screen } from "@testing-library/react-native";

import "../lib/i18n";
import { InContextView } from "../components/InContextView";
import { withTestProviders } from "./testProviders";

const mockScale = { current: 1 };
jest.mock("react-native/Libraries/Utilities/useWindowDimensions", () => ({
  __esModule: true,
  default: () => ({ width: 402, height: 874, scale: 3, fontScale: mockScale.current }),
}));

const card: DeckCard = {
  key: "o2:nb",
  blockId: "o2",
  item: "OfflineLaunch.kbf.json",
  locale: "nb",
  collectionId: "c1",
  source: [{ text: "Bowrain keeps trying." }],
  target: [{ text: "Bowrain prøver igjen." }],
  targetRevision: "r:2",
  group: "closer",
  notes: [],
  findingsCount: 0,
  failingFindingsCount: 0,
  ownWork: false,
};

const container: ReviewContainer = {
  kind: "component",
  title: "OfflineLaunch",
  item: "OfflineLaunch.kbf.json",
  locale: "nb",
  focus: "o2",
  moreBefore: false,
  moreAfter: false,
  units: [
    {
      blockId: "o2",
      item: "OfflineLaunch.kbf.json",
      key: "o2",
      role: "paragraph",
      source: [{ text: "Bowrain keeps trying." }],
      target: [{ text: "Bowrain prøver igjen." }],
      waiting: true,
      sourceRevision: "s:o2",
      targetRevision: "r:o2",
    },
  ],
};

/** Whether `testID`'s element sits inside a scroll view. */
function scrolls(testID: string): boolean {
  for (let node = screen.getByTestId(testID).parent; node; node = node.parent) {
    if (node.type === "RCTScrollView") return true;
  }
  return false;
}

async function open() {
  await render(
    withTestProviders(
      <InContextView
        card={card}
        scope={{ workspace: "w", project: "p" }}
        load={async () => container}
        onApprove={() => {}}
        onFix={() => {}}
      />,
    ),
  );
  await screen.findByTestId("context-title");
}

describe("the in-context view's header", () => {
  it("scrolls with the strings at the largest text size", async () => {
    mockScale.current = 3.1;
    await open();
    expect(scrolls("context-title")).toBe(true);
    expect(scrolls("context-reading-source")).toBe(true);
    expect(scrolls("context-unit-o2")).toBe(true);
  });

  it("stands above the strings at an ordinary size", async () => {
    mockScale.current = 1;
    await open();
    expect(scrolls("context-title")).toBe(false);
    expect(scrolls("context-unit-o2")).toBe(true);
  });
});
