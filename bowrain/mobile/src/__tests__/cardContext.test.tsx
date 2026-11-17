import type { DeckCard, ReviewContext } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import type { ReactNode } from "react";
import { SafeAreaProvider } from "react-native-safe-area-context";

import "../lib/i18n";
import { ContextSheetBody } from "../components/ContextSheet";
import { ReviewCard } from "../components/ReviewCard";
import { fileName, whereParts } from "../lib/where";

const card: DeckCard = {
  key: "b1:nb",
  blockId: "b1",
  item: "apps/kapi-desktop/build/config.yml",
  locale: "nb",
  collectionId: "c1",
  name: "info.comments",
  source: [{ text: "Kapi, the content workbench" }],
  target: [{ text: "Kapi, innholdsarbeidsbenken" }],
  targetRevision: "r:1",
  group: "closer",
  reason: "no pre-review score",
  reasonCode: "no_pre_review",
  notes: [],
  findingsCount: 0,
  failingFindingsCount: 0,
  ownWork: false,
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

describe("whereParts", () => {
  it("reads product and surface from a product/channel binding", () => {
    expect(whereParts({ name: "neokapi-desktop-build", channel: "neokapi/desktop" })).toEqual([
      "Neokapi",
      "Desktop",
    ]);
  });
  it("prefers the declared coordinates", () => {
    expect(
      whereParts({ name: "x", channel: "neokapi/desktop", coordinates: { product: "kapi" } }),
    ).toEqual(["Kapi", "Desktop"]);
  });
  it("falls back to the collection name, and to nothing", () => {
    expect(whereParts({ name: "site-docs" })).toEqual(["Site docs"]);
    expect(whereParts(undefined)).toEqual([]);
  });
  it("names a file by its last segment", () => {
    expect(fileName("apps/kapi-desktop/build/config.yml")).toBe("config.yml");
  });
});

describe("ReviewCard", () => {
  it("shows where the text ships, why it needs a look, and opens the details", async () => {
    const onDetails = jest.fn();
    await render(
      wrap(
        <ReviewCard
          card={card}
          sourceLanguage="en"
          where={["Neokapi", "Desktop"]}
          onDetails={onDetails}
        />,
      ),
    );
    expect(screen.getByTestId("card-where")).toHaveTextContent("Neokapi · Desktop");
    expect(screen.getByTestId("card-why")).toHaveTextContent(/No pre-review yet/);
    expect(screen.getByText("config.yml")).toBeTruthy();
    expect(screen.queryByText(/No context for this text/)).toBeNull();
    await fireEvent.press(screen.getByTestId("card-details"));
    expect(onDetails).toHaveBeenCalled();
  });

  it("words no reason for a quick card", async () => {
    await render(
      wrap(
        <ReviewCard card={{ ...card, group: "fast", reasonCode: "fast" }} sourceLanguage="en" />,
      ),
    );
    expect(screen.queryByTestId("card-why")).toBeNull();
  });
});

describe("ContextSheetBody", () => {
  it("shows the strings around the text and marks the same wording", async () => {
    const context = {
      point: {
        default: false,
        collection: "neokapi-desktop-build",
        voice: { name: "neokapi documentation" },
      },
      neighbourhood: {
        window: 2,
        before: [{ key: "b0", source: [{ text: "Kapi, the content workbench" }] }],
        after: [{ key: "b2", source: [{ text: "© neokapi" }] }],
      },
      history: {},
      judgement: {},
      provenance: {},
    } as unknown as ReviewContext;
    await render(
      wrap(
        <ContextSheetBody
          card={card}
          scope={{ workspace: "w", project: "p" }}
          where={["Neokapi", "Desktop"]}
          load={async () => context}
          onClose={() => {}}
        />,
      ),
    );
    expect(await screen.findByTestId("details-this")).toBeTruthy();
    expect(screen.getByTestId("details-before-0")).toHaveTextContent(/Same text/);
    expect(screen.getByTestId("details-after-0")).not.toHaveTextContent(/Same text/);
    expect(screen.getByText("Neokapi desktop build")).toBeTruthy();
    expect(screen.getByText("neokapi documentation")).toBeTruthy();
    expect(screen.getByText("info.comments")).toBeTruthy();
  });
});
