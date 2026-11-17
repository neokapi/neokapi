import type { DeckCard, ReviewContainer } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import type { ReactNode } from "react";
import { SafeAreaProvider } from "react-native-safe-area-context";

import "../lib/i18n";
import { ContextSheetBody } from "../components/ContextSheet";
import { InContextView } from "../components/InContextView";

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

const unit = (
  blockId: string,
  role: string,
  src: string,
  tgt: string,
  waiting: boolean,
  level?: number,
) => ({
  blockId,
  item: "OfflineLaunch.kbf.json",
  key: blockId,
  role,
  level,
  source: [{ text: src }],
  target: [{ text: tgt }],
  waiting,
  sourceRevision: `s:${blockId}`,
  targetRevision: `r:${blockId}`,
});

const container: ReviewContainer = {
  kind: "component",
  title: "OfflineLaunch",
  item: "OfflineLaunch.kbf.json",
  locale: "nb",
  focus: "o2",
  moreBefore: false,
  moreAfter: false,
  units: [
    unit("o1", "heading", "Can't reach your server", "Får ikke kontakt med serveren", true, 2),
    unit("o2", "paragraph", "Bowrain keeps trying.", "Bowrain prøver igjen.", true),
    unit("o3", "button", "Try again", "Prøv igjen", false),
  ],
};

function wrap(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <SafeAreaProvider
      initialMetrics={{
        frame: { x: 0, y: 0, width: 390, height: 844 },
        insets: { top: 0, left: 0, right: 0, bottom: 0 },
      }}
    >
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </SafeAreaProvider>
  );
}

describe("InContextView", () => {
  it("draws the component's strings in their roles, in the translation, with the card's text marked", async () => {
    await render(
      wrap(
        <InContextView
          card={card}
          scope={{ workspace: "w", project: "p" }}
          load={async () => container}
          onApprove={() => {}}
          onFix={() => {}}
        />,
      ),
    );
    expect(await screen.findByTestId("context-title")).toHaveTextContent("OfflineLaunch");
    expect(screen.getByTestId("context-kind")).toHaveTextContent("Component");
    expect(screen.getByTestId("context-unit-o1")).toHaveTextContent(
      "Får ikke kontakt med serveren",
    );
    expect(screen.getByTestId("context-unit-o3")).toHaveTextContent("Prøv igjen");
    // Two waiting, the approved button has no dot.
    expect(screen.getAllByTestId("context-waiting-dot")).toHaveLength(2);

    await fireEvent.press(screen.getByTestId("context-reading-source"));
    expect(screen.getByTestId("context-unit-o1")).toHaveTextContent("Can't reach your server");
  });

  it("approves a unit in place, and all the waiting ones at once", async () => {
    const onApprove = jest.fn();
    await render(
      wrap(
        <InContextView
          card={card}
          scope={{ workspace: "w", project: "p" }}
          load={async () => container}
          onApprove={onApprove}
          onFix={() => {}}
        />,
      ),
    );
    // The card's own unit is selected and offers its actions.
    await fireEvent.press(await screen.findByTestId("context-approve"));
    expect(onApprove).toHaveBeenCalledWith(
      expect.objectContaining({
        blockId: "o2",
        targetRevision: "r:o2",
        sourceRevision: "s:o2",
        locale: "nb",
      }),
    );
    expect(screen.getAllByTestId("context-waiting-dot")).toHaveLength(1);

    await fireEvent.press(screen.getByTestId("context-unit-o1"));
    const fixed = jest.fn();
    expect(screen.getByTestId("context-fix")).toBeTruthy();
    expect(fixed).not.toHaveBeenCalled();
  });

  it("offers to approve every waiting translation when more than one waits", async () => {
    const onApprove = jest.fn();
    await render(
      wrap(
        <InContextView
          card={card}
          scope={{ workspace: "w", project: "p" }}
          load={async () => container}
          onApprove={onApprove}
          onFix={() => {}}
        />,
      ),
    );
    await fireEvent.press(await screen.findByTestId("context-approve-all"));
    expect(onApprove.mock.calls.map(([c]) => c.blockId)).toEqual(["o1", "o2"]);
    expect(screen.queryAllByTestId("context-waiting-dot")).toHaveLength(0);
  });
});

describe("ContextSheetBody", () => {
  it("opens on In context when it can draw it, and switches to the details", async () => {
    await render(
      wrap(
        <ContextSheetBody
          card={card}
          scope={{ workspace: "w", project: "p" }}
          where={[]}
          load={async () => ({}) as never}
          onClose={() => {}}
          inContext={{
            loadContainer: async () => container,
            onApprove: () => {},
            onFix: () => {},
          }}
        />,
      ),
    );
    expect(await screen.findByTestId("context-title")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("sheet-tab-details"));
    expect(screen.queryByTestId("context-title")).toBeNull();
    expect(screen.getByText("OfflineLaunch.kbf.json")).toBeTruthy();
  });
});
