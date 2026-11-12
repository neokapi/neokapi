import type { DeckCard, FixCandidates } from "@neokapi/bowrain-client";
import type { Run } from "@neokapi/kapi-format";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import type { ReactNode } from "react";
import { SafeAreaProvider } from "react-native-safe-area-context";

import "../lib/i18n";
import { FixSheetBody } from "../components/FixSheet";

const name: Run = { ph: { id: "1", type: "x-var", data: "{{name}}", equiv: "{{name}}" } };

const card: DeckCard = {
  key: "b1:nb",
  blockId: "b1",
  item: "i18n/en.json",
  locale: "nb",
  collectionId: "",
  source: [{ text: "Save the workspace for " }, name],
  target: [{ text: "Lagre arbeidsrommet for " }, name],
  targetRevision: "r:0123456789abcdef",
  sourceRevision: "r:aaaaaaaaaaaaaaaa",
  group: "closer",
  notes: [],
  findingsCount: 0,
  failingFindingsCount: 0,
  ownWork: false,
};

const fixes: FixCandidates = {
  candidates: [
    {
      id: "term-0",
      kind: "term",
      label: 'Use "arbeidsområdet"',
      runs: [{ text: "Lagre arbeidsområdet for " }, name],
      reasons: ["wrong_term"],
    },
  ],
  dropped: [],
  notes: [],
};

function wrap(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <SafeAreaProvider
      initialMetrics={{ frame: { x: 0, y: 0, width: 390, height: 844 }, insets: { top: 0, left: 0, right: 0, bottom: 0 } }}
    >
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </SafeAreaProvider>
  );
}

function setup() {
  const onFix = jest.fn();
  const onReject = jest.fn();
  const loadFixes = jest.fn(async () => fixes);
  return { onFix, onReject, loadFixes };
}

describe("FixSheet", () => {
  it("applies a ready-made fix with the chosen reason", async () => {
    const h = setup();
    await render(wrap(<FixSheetBody card={card} onClose={() => {}} {...h} />));
    await fireEvent.press(screen.getByTestId("reason-wrong_term"));
    await fireEvent.press(await screen.findByTestId("use-term-0"));
    expect(h.onFix).toHaveBeenCalledWith(card, fixes.candidates[0].runs, ["wrong_term"]);
    expect(h.loadFixes).toHaveBeenLastCalledWith(card, "wrong_term", false);
  });

  it("edits the text with the placeholder locked and keeps it in the correction", async () => {
    const h = setup();
    await render(wrap(<FixSheetBody card={card} onClose={() => {}} {...h} />));
    await fireEvent.press(screen.getByTestId("fix-start-edit"));
    expect(screen.getAllByTestId("editor-chip")).toHaveLength(1);
    await fireEvent.changeText(screen.getByTestId("editor-field-0"), "Lagre arbeidsområdet for ");
    await fireEvent.press(screen.getByTestId("fix-approve-edit"));
    expect(h.onFix).toHaveBeenCalledWith(card, [{ text: "Lagre arbeidsområdet for " }, name], []);
  });

  it("rejects only with a reason, and carries the note", async () => {
    const h = setup();
    await render(wrap(<FixSheetBody card={card} onClose={() => {}} {...h} />));
    await fireEvent.press(screen.getByTestId("fix-reject"));
    expect(h.onReject).not.toHaveBeenCalled();
    await fireEvent.press(screen.getByTestId("reason-meaning"));
    await fireEvent.changeText(screen.getByTestId("reject-note"), "Says the opposite");
    await fireEvent.press(screen.getByTestId("fix-reject"));
    expect(h.onReject).toHaveBeenCalledWith(card, ["meaning"], "Says the opposite");
  });
});
