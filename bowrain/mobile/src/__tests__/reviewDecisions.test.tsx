import type { BowrainClient, DeckCard, FeedDecisions } from "@neokapi/bowrain-client";
import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react-native";

import "../lib/i18n";
import { ReviewDecisions } from "../components/feed/ReviewDecisions";
import { setDecisionClient } from "../lib/decisions";
import { createQueryClient } from "../lib/queryClient";

const mockFeedDecisions = jest.fn<Promise<FeedDecisions>, unknown[]>();
jest.mock("../lib/auth", () => ({
  useAuth: () => ({ client: { feedDecisions: (...a: unknown[]) => mockFeedDecisions(...a) } }),
}));

const card = (n: number): DeckCard => ({
  key: `b${n}:nb`,
  blockId: `b${n}`,
  item: "i18n/en.json",
  locale: "nb",
  collectionId: "",
  source: [{ text: `Source ${n}` }],
  target: [{ text: `Mål ${n}` }],
  targetRevision: `r:t${n}`,
  sourceRevision: "r:s",
  group: "fast",
  notes: [],
  findingsCount: 0,
  failingFindingsCount: 0,
  ownWork: false,
});

describe("a review entry's decisions", () => {
  afterEach(() => setDecisionClient(null));

  it("withdraws the reader's own decision on the revision the server listed", async () => {
    mockFeedDecisions.mockResolvedValue({
      locale: "nb",
      bulkApproved: 0,
      decisions: [
        {
          card: card(1),
          decision: "approved",
          at: new Date().toISOString(),
          current: true,
          canUndo: true,
        },
        {
          card: card(2),
          decision: "rejected",
          at: new Date().toISOString(),
          current: false,
          canUndo: false,
        },
      ],
    });
    const sent: unknown[] = [];
    setDecisionClient({
      applyChanges: async (_scope: unknown, set: unknown) => {
        sent.push(set);
        return {
          schema: "kapi.change-result/v1",
          status: "applied",
          record: "rec",
          docs: [],
          ops: [{ i: 0, op: "decide", status: "applied", after: "r:after" }],
        };
      },
    } as unknown as BowrainClient);
    const qc = createQueryClient();
    await render(
      <QueryClientProvider client={qc}>
        <ReviewDecisions ws="ws" project="p" entry="rvw_1" />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Mål 1")).toBeTruthy();
    expect(screen.queryByTestId("feed-decision-b2-undo")).toBeNull();
    expect(screen.getByText(/Changed since/)).toBeTruthy();

    fireEvent.press(screen.getByTestId("feed-decision-b1-undo"));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toMatchObject({
      ops: [{ op: "decide", outcome: "withdraw", if_match: "r:t1", at: { block: "b1" } }],
    });
    qc.clear();
  });
});
