// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import {
  HistoryCard,
  JudgementCard,
  LayerCard,
  NeighbourhoodCard,
  PointCard,
  ProvenanceCard,
  decisionLabel,
  originLabel,
  type ReviewPointView,
} from "../components/review";

beforeAll(() => {
  if (typeof globalThis.ResizeObserver === "undefined") {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
  }
});

afterEach(cleanup);

const slot = (name: string) => document.querySelector(`[data-slot='${name}']`);

describe("LayerCard", () => {
  it("draws the summary open or closed, and folds the detail", async () => {
    render(
      <LayerCard title="Provenance" summary="Machine translation" dataSlot="layer" testId="rail">
        <p>the detail</p>
      </LayerCard>,
    );
    expect(screen.getByTestId("rail-summary").textContent).toContain("Machine translation");
    expect(screen.getByText("the detail")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Provenance" }));
    expect(screen.getByTestId("rail-summary").textContent).toContain("Machine translation");
    expect(screen.getByRole("button", { name: "Provenance" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
  });

  it("can be pinned open with no fold control", () => {
    render(
      <LayerCard title="Point" summary="x" collapsible={false}>
        <p>always</p>
      </LayerCard>,
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("always")).toBeTruthy();
  });
});

const point: ReviewPointView = {
  ref: "retail/web",
  collection: "Product UI",
  coordinates: { product: "kapimart", channel: "web" },
  voice: {
    name: "Kapimart retail",
    source: "pack:retail",
    guide: "Speak in the second person.",
    score: 62,
    bar: 90,
  },
  termRules: [
    { term: "cart", replacement: "basket", severity: "major" },
    { term: "sign in", replacement: "log in", severity: "minor" },
    { term: "Kapimart", do_not_translate: true },
  ],
  termsTotal: 40,
  termHits: [{ term: "password", renderings: ["mot de passe"], domain: "account" }],
  profiles: [{ name: "retail", state: "active", valid_from: "2026-01-01" }],
  notes: ["The channel was derived from the collection."],
};

describe("PointCard", () => {
  it("leads with the address: ref, collection and coordinate chips", () => {
    render(<PointCard point={point} />);
    expect(slot("review-point-ref")?.textContent).toBe("retail/web");
    expect(screen.getByTestId("point-collection").textContent).toBe("Product UI");
    const chips = document.querySelectorAll("[data-slot='review-point-coordinate']");
    expect(Array.from(chips).map((c) => c.getAttribute("data-axis"))).toEqual([
      "product",
      "channel",
    ]);
    expect(screen.getByTestId("point-coordinate-product").textContent?.trim()).toBe("kapimart");
  });

  it("names the voice with its score against the bar, and folds the guidance", async () => {
    render(<PointCard point={point} />);
    expect(screen.getByTestId("point-profile-name").textContent).toBe("Kapimart retail");
    const score = screen.getByTestId("point-voice-score");
    expect(score.textContent).toBe("Voice 62 of 90");
    expect(score.getAttribute("data-below-bar")).toBe("true");
    expect(screen.queryByTestId("point-guidance")).toBeNull();
    await userEvent.click(screen.getByTestId("point-guidance-toggle"));
    expect(screen.getByTestId("point-guidance").textContent).toContain("second person");
  });

  it("draws term rules as neutral chips carrying their bite, and the total when capped", () => {
    render(<PointCard point={point} />);
    const chips = Array.from(
      document.querySelectorAll<HTMLElement>("[data-slot='review-point-term']"),
    );
    expect(chips.map((c) => c.getAttribute("data-severity"))).toEqual([
      "blocks",
      "warns",
      "blocks",
    ]);
    expect(chips[2].getAttribute("data-dnt")).toBe("true");
    for (const chip of chips) expect(chip.className).not.toMatch(/destructive|warning/);
    expect(chips[0].textContent).toContain("basket");
    expect(slot("review-point-terms-total")?.textContent).toContain("3 of 40");
  });

  it("lists the terms the source matches, and says when none did", () => {
    const { unmount } = render(<PointCard point={point} />);
    expect(screen.getByTestId("point-terms").textContent).toContain("mot de passe");
    unmount();
    render(<PointCard point={{ ...point, termHits: [] }} />);
    expect(screen.getByTestId("point-terms").textContent).toContain("No terms matched this block.");
  });

  it("leaves the matched-terms row out for a host that looks none up", () => {
    render(<PointCard point={{ ...point, termHits: undefined }} />);
    expect(screen.queryByTestId("point-terms")).toBeNull();
  });

  it("says what an ungoverned point lacks", () => {
    render(<PointCard point={{ termHits: [] }} />);
    const card = screen.getByTestId("review-point");
    expect(card.textContent).toContain("No voice profile is bound at this point.");
    expect(card.textContent).toContain("none in force here");
    expect(slot("review-point-ref")).toBeNull();
    expect(slot("review-point-summary")?.textContent).toContain("No coordinates declared");
  });

  it("names the default point, and reads as loading before the model arrives", () => {
    const { unmount } = render(<PointCard point={{ default: true }} />);
    expect(slot("review-point-ref")?.textContent).toBe("default point");
    unmount();
    render(<PointCard loading />);
    expect(screen.getByTestId("review-point").textContent).toContain("Resolving the point");
  });
});

describe("NeighbourhoodCard", () => {
  it("counts the blocks either side and draws them through the run projection", () => {
    render(
      <NeighbourhoodCard
        neighbourhood={{
          key: "greeting",
          before: [{ key: "welcome", source: [{ text: "Welcome back." }] }],
          after: [
            {
              key: "reset",
              source: [
                { text: "Your credits reset on " },
                { ph: { id: "1", type: "code:variable", data: "{date}", equiv: "{date}" } },
                { text: "." },
              ],
            },
          ],
        }}
        unitKey="greeting"
        unitSource="Hello {name}"
        unitTarget="Bonjour {name}"
        testId="hood"
      />,
    );
    expect(screen.getByTestId("hood-summary").textContent).toContain("1 before, 1 after");
    const rows = document.querySelectorAll("[data-slot='review-neighbour']");
    expect(rows).toHaveLength(2);
    expect(rows[1].textContent).toContain("Your credits reset on");
    expect(rows[1].textContent).toContain("date");
    expect(slot("review-neighbour-unit")?.textContent).toContain("Bonjour {name}");
  });

  it("says a unit stands alone, and says when the document could not be read", () => {
    const { unmount } = render(
      <NeighbourhoodCard neighbourhood={{ before: [], after: [] }} unitKey="k" testId="hood" />,
    );
    expect(screen.getByTestId("hood-summary").textContent).toContain("stands alone");
    unmount();
    render(<NeighbourhoodCard testId="hood" />);
    expect(screen.getByTestId("hood-summary").textContent).toContain("could not be read");
  });
});

describe("HistoryCard", () => {
  it("shows the prior version with its governed mark and the match with its wording", () => {
    render(
      <HistoryCard
        history={{
          prior: { source: "Hi {name}", target: "Salut {name}", governed: false },
          match: { score: 88, source: "Hello {name}!", target: "Bonjour {name} !", kind: "fuzzy" },
        }}
        testId="history"
      />,
    );
    expect(screen.getByTestId("history-summary").textContent).toContain("context that has moved");
    expect(slot("review-history-prior")?.textContent).toContain("Salut {name}");
    expect(slot("review-history-prior")?.textContent).toContain("context has moved");
    expect(screen.getByTestId("memory-match-score").textContent).toBe("88%");
    expect(screen.getByTestId("memory-match-target").textContent).toBe("Bonjour {name} !");
    expect(screen.getByTestId("memory-match").textContent).toContain("fuzzy");
  });

  it("offers the match's wording only where the surface can write", async () => {
    const onUseMatch = vi.fn();
    const { unmount } = render(
      <HistoryCard
        history={{ match: { score: 100, target: "Bonjour" } }}
        onUseMatch={onUseMatch}
      />,
    );
    await userEvent.click(screen.getByTestId("memory-match-use"));
    expect(onUseMatch).toHaveBeenCalled();
    unmount();
    render(<HistoryCard history={{ match: { score: 100, target: "Bonjour" } }} />);
    expect(screen.queryByTestId("memory-match-use")).toBeNull();
  });

  it("states its own empty case, and that an unread memory is not an empty one", () => {
    const { unmount } = render(<HistoryCard history={{}} />);
    expect(slot("review-history-empty")?.textContent).toBe(
      "No content-memory match for this block.",
    );
    unmount();
    render(<HistoryCard history={{ unseeded: true }} />);
    expect(slot("review-history-empty")?.textContent).toContain("has not been read");
    expect(slot("review-history-empty")?.textContent).not.toContain("no close match");
  });
});

describe("JudgementCard", () => {
  it("counts the findings and names the worst, with what each was raised against", () => {
    render(
      <JudgementCard
        findings={[
          {
            id: "finding-issue-0",
            category: "placeholder",
            severity: "warning",
            tone: "warning",
            message: "Placeholder spacing differs.",
          },
          {
            id: "finding-voice-0",
            category: "compliance",
            severity: "major",
            tone: "destructive",
            message: "Uses a term the profile forbids.",
            originalText: "Réinitialisez",
            suggestion: "Changez",
          },
        ]}
        testId="checks"
      />,
    );
    expect(screen.getByTestId("checks-summary").textContent).toContain("2 findings, 1 failing");
    expect(screen.getByTestId("checks-summary").textContent).toContain("major");
    expect(screen.getByTestId("finding-voice-0-original").textContent).toBe("Réinitialisez");
    expect(screen.getByTestId("finding-voice-0-suggestion").textContent).toBe("Changez");
    expect(screen.getByTestId("finding-voice-0").getAttribute("data-tone")).toBe("destructive");
  });

  it("marks a finding judged on the source side", () => {
    render(
      <JudgementCard
        findings={[
          { severity: "major", tone: "destructive", message: "Forbidden term.", field: "source" },
        ]}
      />,
    );
    expect(within(slot("review-finding") as HTMLElement).getByText("source")).toBeTruthy();
  });

  it("says nothing was found, and carries the AI pre-review inside the card", () => {
    render(<JudgementCard findings={[]} aiScore={84} aiModel="claude" testId="checks" />);
    expect(screen.getByTestId("findings-none").textContent).toContain("No findings for this unit.");
    expect(screen.getByTestId("checks-summary").textContent).toContain("AI 84");
    expect(slot("review-findings")?.contains(slot("review-ai-prereview"))).toBe(true);
    expect(slot("review-ai-score")?.textContent).toContain("84");
  });

  it("draws the surface's own controls above the list", () => {
    render(
      <JudgementCard findings={[]}>
        <button type="button">Re-check</button>
      </JudgementCard>,
    );
    expect(screen.getByRole("button", { name: "Re-check" })).toBeTruthy();
  });
});

describe("ProvenanceCard", () => {
  it("names the origin and the decision in force, with the stale mark", () => {
    render(
      <ProvenanceCard
        provenance={{
          origin: { kind: "ai", engine: "claude-sonnet", tool: "translate" },
          decision: {
            state: "rejected",
            by: "maria@bowrain.test",
            at: "2026-08-30T09:12:00Z",
            note: "Reads as machine output.",
            sourceMoved: true,
          },
        }}
        testId="prov"
      />,
    );
    const summary = screen.getByTestId("prov-summary").textContent ?? "";
    expect(summary).toContain("AI translation");
    expect(summary).toContain("Rejected");
    expect(slot("review-provenance-stale")).not.toBeNull();
    const body = screen.getByTestId("review-provenance").textContent ?? "";
    expect(body).toContain("claude-sonnet · translate");
    expect(body).toContain("maria@bowrain.test");
    expect(screen.getByTestId("provenance-note").textContent).toContain("Reads as machine output.");
  });

  it("shows a kind it has no phrase for as recorded, and states its own empty case", () => {
    const { unmount } = render(<ProvenanceCard provenance={{ origin: { kind: "import" } }} />);
    expect(screen.getByTestId("provenance-origin").textContent).toContain("import");
    unmount();
    render(<ProvenanceCard provenance={{}} testId="prov" />);
    expect(screen.getByTestId("review-provenance").textContent).toContain(
      "No decision recorded, and no provenance stamped.",
    );
    expect(screen.getByTestId("prov-summary").textContent).toContain("Nothing recorded");
  });

  it("reads the same words for the same kinds and states everywhere", () => {
    expect(originLabel("memory")).toBe("Recycled from content memory");
    expect(originLabel("nonsense")).toBeUndefined();
    expect(decisionLabel("signed-off")).toBe("Signed off");
    expect(decisionLabel("")).toBeUndefined();
  });
});
