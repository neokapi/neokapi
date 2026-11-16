import type { ContextCard, ContextOrigin, ContextOriginPage } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import type { ReactNode } from "react";
import { SafeAreaProvider } from "react-native-safe-area-context";

import i18n from "../lib/i18n";
import { ContextCard as Card } from "../components/ContextCard";
import { OriginSheetBody } from "../components/OriginSheet";
import { originLine, originSummary } from "../lib/contextOrigin";

// Reanimated needs its native worklets runtime even through its own mock,
// so its layout animations stand in here as views that render their
// children without animating.
jest.mock("react-native-reanimated", () => {
  const { View } = require("react-native");
  const animation = { delay: () => animation, duration: () => animation };
  return {
    __esModule: true,
    default: { View },
    FadeIn: animation,
    FadeInDown: animation,
  };
});

const t = i18n.t.bind(i18n);

const termsImport: ContextOrigin = {
  id: "0op-terms",
  short: "0op",
  kind: "import",
  source: "terms",
  path: ".kapi/terms.json",
  count: 61,
  author: { kind: "person", name: "Asgeir", host: "claude-code", agent_authored: true },
  at: "2026-11-10T09:30:00Z",
};

function concept(id: string, terms: [string, string][], definition?: string): ContextCard {
  return {
    item: { kind: "concept", id },
    kind: "concept",
    revision: "r1",
    terms: terms.map(([locale, text]) => ({ locale, text, status: "preferred" })),
    text: definition,
    language: "en",
    where: { level: "workspace" },
    author: { kind: "person", name: "Asgeir", agent_authored: false },
    own_work: false,
    actions: ["keep", "reject", "later"],
    origin: termsImport,
  };
}

const current = concept(
  "c-memory",
  [
    ["en", "content memory"],
    ["nb", "innhaldsminne"],
  ],
  "The store of source and target pairs.",
);
const sibling = { ...concept("c-kapi", [["en", "kapi"]]), reviewed: true };

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <SafeAreaProvider
      initialMetrics={{
        frame: { x: 0, y: 0, width: 390, height: 844 },
        insets: { top: 0, left: 0, right: 0, bottom: 0 },
      }}
    >
      <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
    </SafeAreaProvider>
  );
}

describe("the origin of a context item", () => {
  it("names the file and how many items it carried", () => {
    expect(originLine(t, termsImport)).toBe("Imported from .kapi/terms.json · 61 items");
    expect(originSummary(t, termsImport)).toBe("Added 61 concepts to this project's terms");
    expect(originSummary(t, { ...termsImport, count: 1 })).toBe(
      "Added 1 concept to this project's terms",
    );
  });

  it("names the profile a voice edit wrote", () => {
    const edit: ContextOrigin = {
      ...termsImport,
      kind: "voice_edit",
      source: undefined,
      path: undefined,
      profile: "neokapi-documentation",
      count: 3,
    };
    expect(originLine(t, edit)).toBe("Voice profile edited");
    expect(originSummary(t, edit)).toBe("Edited the voice profile neokapi-documentation");
  });

  it("opens from the card's provenance line", async () => {
    const open = jest.fn();
    await render(wrap(<Card card={current} onOpenOrigin={open} />));
    await fireEvent.press(screen.getByTestId("context-origin"));
    expect(open).toHaveBeenCalledTimes(1);
  });

  it("shows no provenance line on an item no bulk operation brought in", async () => {
    await render(wrap(<Card card={{ ...current, origin: undefined }} />));
    expect(screen.queryByTestId("context-origin")).toBeNull();
  });
});

describe("the origin sheet", () => {
  const page: ContextOriginPage = {
    origin: termsImport,
    total: 61,
    pending: 60,
    cards: [current, sibling],
    next_cursor: "2",
  };

  it("says who ran the operation, from where, and lists what it brought in", async () => {
    const load = jest.fn(async () => page);
    await render(
      wrap(<OriginSheetBody card={current} origin={termsImport} load={load} onClose={() => {}} />),
    );

    expect(screen.getByTestId("origin-summary")).toHaveTextContent(
      "Added 61 concepts to this project's terms",
    );
    expect(screen.getByText("Asgeir")).toBeTruthy();
    expect(screen.getByTestId("origin-host")).toHaveTextContent(/claude-code/);
    expect(screen.getByTestId("origin-path")).toHaveTextContent(".kapi/terms.json");

    expect(await screen.findByTestId("origin-item-current")).toBeTruthy();
    expect(load).toHaveBeenCalledWith("0op-terms", undefined);
    expect(screen.getByTestId("origin-waiting")).toHaveTextContent("60 of 61 wait for review");
    expect(screen.getByText("This card")).toBeTruthy();
    expect(screen.getAllByTestId("origin-item")).toHaveLength(1);
    expect(screen.getByLabelText("Reviewed")).toBeTruthy();
  });

  it("expands a concept to its definition and its other languages", async () => {
    await render(
      wrap(
        <OriginSheetBody
          card={sibling}
          origin={termsImport}
          load={async () => page}
          onClose={() => {}}
        />,
      ),
    );
    const items = await screen.findAllByTestId("origin-item");
    expect(screen.queryByTestId("origin-item-detail")).toBeNull();
    await fireEvent.press(items[0]);
    expect(screen.getByTestId("origin-item-detail")).toBeTruthy();
    expect(screen.getByText("The store of source and target pairs.")).toBeTruthy();
  });

  it("explains why the item is a card, and pages the rest", async () => {
    const load = jest.fn(async (_op: string, cursor: string | undefined) =>
      cursor
        ? { ...page, cards: [concept("c-bowrain", [["en", "Bowrain"]])], next_cursor: undefined }
        : page,
    );
    await render(
      wrap(<OriginSheetBody card={current} origin={termsImport} load={load} onClose={() => {}} />),
    );
    await fireEvent.press(screen.getByTestId("origin-why"));
    expect(screen.getByTestId("origin-why-body")).toBeTruthy();

    await fireEvent.press(await screen.findByTestId("origin-more"));
    expect(await screen.findByText("Bowrain")).toBeTruthy();
    expect(load).toHaveBeenLastCalledWith("0op-terms", "2");
  });
});
