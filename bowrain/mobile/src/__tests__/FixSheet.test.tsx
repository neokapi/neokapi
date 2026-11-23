import {
  ApiError,
  NetworkError,
  RequestTimeoutError,
  type DeckCard,
  type FixCandidates,
} from "@neokapi/bowrain-client";
import type { Run } from "@neokapi/kapi-format";
import { withTestProviders } from "./testProviders";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react-native";

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

/** A promise the test settles. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function setup() {
  const onFix = jest.fn();
  const onReject = jest.fn();
  const loadFixes = jest.fn(async (..._args: unknown[]): Promise<FixCandidates> => fixes);
  return { onFix, onReject, loadFixes };
}

describe("FixSheet", () => {
  it.each([
    [
      "the client's bound passed",
      new RequestTimeoutError("/fix-candidates", 65_000),
      /took too long/,
    ],
    ["the load balancer gave up", new ApiError(504, "gateway timeout", undefined), /took too long/],
    ["the connection failed", new NetworkError("Network request failed"), /need a connection/],
  ])("says why the ready-made fixes did not load when %s", async (_why, err, text) => {
    const h = { ...setup(), loadFixes: jest.fn(async () => Promise.reject(err)) };
    await render(withTestProviders(<FixSheetBody card={card} onClose={() => {}} {...h} />));
    expect(await screen.findByText(text)).toBeTruthy();
    expect(screen.getByTestId("fix-retry")).toBeTruthy();
  });

  it("tries the suggestions again after a timeout, and shows them when they come", async () => {
    const h = setup();
    const pending = deferred<FixCandidates>();
    h.loadFixes
      .mockImplementationOnce(async () =>
        Promise.reject(new RequestTimeoutError("/fix-candidates", 65_000)),
      )
      .mockImplementationOnce(() => pending.promise);
    await render(withTestProviders(<FixSheetBody card={card} onClose={() => {}} {...h} />));
    expect(await screen.findByText(/took too long/)).toBeTruthy();
    await fireEvent.press(screen.getByTestId("fix-retry"));
    // While it asks again, the control is gone and the sheet waits.
    await waitFor(() => expect(screen.queryByTestId("fix-retry")).toBeNull());
    expect(screen.queryByText(/took too long/)).toBeNull();
    await act(async () => pending.resolve(fixes));
    expect(await screen.findByTestId("use-term-0")).toBeTruthy();
    expect(h.loadFixes).toHaveBeenCalledTimes(2);
  });

  it("shows the same sentence when a retry ends in a 504 again, and never asks by itself", async () => {
    const h = setup();
    h.loadFixes.mockImplementation(async () =>
      Promise.reject(new ApiError(504, "gateway timeout", undefined)),
    );
    await render(withTestProviders(<FixSheetBody card={card} onClose={() => {}} {...h} />));
    expect(await screen.findByText(/took too long/)).toBeTruthy();
    expect(h.loadFixes).toHaveBeenCalledTimes(1);
    await fireEvent.press(screen.getByTestId("fix-retry"));
    await waitFor(() => expect(h.loadFixes).toHaveBeenCalledTimes(2));
    expect(await screen.findByText(/took too long/)).toBeTruthy();
    expect(await screen.findByTestId("fix-retry")).toBeTruthy();
    await act(async () => new Promise((r) => setTimeout(r, 50)));
    expect(h.loadFixes).toHaveBeenCalledTimes(2);
  });

  it("applies a ready-made fix with the chosen reason", async () => {
    const h = setup();
    await render(withTestProviders(<FixSheetBody card={card} onClose={() => {}} {...h} />));
    await fireEvent.press(screen.getByTestId("reason-wrong_term"));
    await fireEvent.press(await screen.findByTestId("use-term-0"));
    expect(h.onFix).toHaveBeenCalledWith(card, fixes.candidates[0].runs, ["wrong_term"]);
    expect(h.loadFixes).toHaveBeenLastCalledWith(
      card,
      "wrong_term",
      false,
      expect.any(AbortSignal),
    );
  });

  it("edits the text with the placeholder locked and keeps it in the correction", async () => {
    const h = setup();
    await render(withTestProviders(<FixSheetBody card={card} onClose={() => {}} {...h} />));
    await fireEvent.press(screen.getByTestId("fix-start-edit"));
    expect(screen.getAllByTestId("editor-chip")).toHaveLength(1);
    await fireEvent.changeText(screen.getByTestId("editor-field-0"), "Lagre arbeidsområdet for ");
    await fireEvent.press(screen.getByTestId("fix-approve-edit"));
    expect(h.onFix).toHaveBeenCalledWith(card, [{ text: "Lagre arbeidsområdet for " }, name], []);
  });

  it("rejects only with a reason, and carries the note", async () => {
    const h = setup();
    await render(withTestProviders(<FixSheetBody card={card} onClose={() => {}} {...h} />));
    await fireEvent.press(screen.getByTestId("fix-reject"));
    expect(h.onReject).not.toHaveBeenCalled();
    await fireEvent.press(screen.getByTestId("reason-meaning"));
    await fireEvent.changeText(screen.getByTestId("reject-note"), "Says the opposite");
    await fireEvent.press(screen.getByTestId("fix-reject"));
    expect(h.onReject).toHaveBeenCalledWith(card, ["meaning"], "Says the opposite");
  });

  it("opens on the rejection from a left swipe, with the fixes one tap away", async () => {
    const h = setup();
    await render(
      withTestProviders(<FixSheetBody card={card} mode="reject" onClose={() => {}} {...h} />),
    );
    expect(screen.queryByTestId("fix-start-edit")).toBeNull();
    expect(h.loadFixes).not.toHaveBeenCalled();
    await fireEvent.press(screen.getByTestId("reason-tone"));
    await fireEvent.press(screen.getByTestId("fix-reject"));
    expect(h.onReject).toHaveBeenCalledWith(card, ["tone"], "");

    await fireEvent.press(screen.getByTestId("fix-instead"));
    expect(await screen.findByTestId("use-term-0")).toBeTruthy();
    expect(screen.getByTestId("fix-start-edit")).toBeTruthy();
  });
});
