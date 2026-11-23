// At the largest text sizes a fixed-height row cannot hold its title beside
// its icon and count, so the title and status take a line of their own and
// wrap rather than cut a word. At ordinary sizes the row keeps its lines.
import type { ReviewInboxLanguage } from "@neokapi/bowrain-client";
import { render, screen } from "@testing-library/react-native";

import "../lib/i18n";
import { ContextEntryCard, LanguageCard } from "../components/home/HomeCards";

const nb: ReviewInboxLanguage = {
  workspace: "acme",
  workspace_name: "Acme",
  project: "p1",
  project_name: "Website",
  stream: "main",
  locale: "nb",
  waiting: 6,
  fast: 0,
  closer: 6,
  desk: 0,
  unclassified: 0,
};

const mockScale = { current: 1 };
jest.mock("react-native/Libraries/Utilities/useWindowDimensions", () => ({
  __esModule: true,
  default: () => ({ width: 402, height: 874, scale: 3, fontScale: mockScale.current }),
}));

function atScale(fontScale: number) {
  mockScale.current = fontScale;
}

describe("rows at the largest text sizes", () => {
  it("keeps the context row to its lines at an ordinary size", async () => {
    atScale(1);
    await render(
      <ContextEntryCard
        count={0}
        title="Context"
        detail="Up to date"
        detailLines={1}
        onPress={() => {}}
      />,
    );
    expect(screen.getByText("Context").props.numberOfLines).toBe(1);
    expect(screen.getByText("Up to date").props.numberOfLines).toBe(1);
  });

  it("lets the context row's title and detail wrap at the largest size", async () => {
    atScale(3.1);
    await render(
      <ContextEntryCard
        count={0}
        title="Context"
        detail="Up to date"
        detailLines={1}
        onPress={() => {}}
      />,
    );
    expect(screen.getByText("Context").props.numberOfLines).toBeUndefined();
    expect(screen.getByText("Up to date").props.numberOfLines).toBe(4);
  });

  it("gives the context row's detail the same lines while loading and once loaded", async () => {
    atScale(3.1);
    const row = (detail: string) => (
      <ContextEntryCard
        count={3}
        title="Context"
        detail={detail}
        detailLines={1}
        onPress={() => {}}
      />
    );
    const loaded = "1 voice · 12 terms · 4 rules · 3 notes";
    const view = await render(row("Loading"));
    const before = screen.getByText("Loading").props;
    await view.rerender(row(loaded));
    const after = screen.getByText(loaded).props;
    expect(after.numberOfLines).toBe(before.numberOfLines);
    expect(after.style).toEqual(before.style);
    // Four lines of small text at the scale: the most the kinds line takes.
    expect(after.style).toMatchObject({ minHeight: 4 * 20 * 3.1 });
  });

  it("lets a language card's name wrap at the largest size", async () => {
    atScale(3.1);
    await render(<LanguageCard locale="nb" waiting={nb} onPress={() => {}} />);
    expect(screen.getByText("Norwegian Bokmål").props.numberOfLines).toBeUndefined();
  });

  it("gives a language card's status line the legend's two lines while loading", async () => {
    atScale(3.1);
    await render(<LanguageCard locale="nb" loading onPress={() => {}} />);
    const loading = screen.getByText("Loading").props;
    expect(loading.numberOfLines).toBe(2);
    expect(loading.style).toMatchObject({ minHeight: 2 * 16 * 3.1 });
  });

  it("gives the review mix's legend the same two lines once the counts arrive", async () => {
    atScale(3.1);
    await render(<LanguageCard locale="nb" waiting={nb} onPress={() => {}} />);
    const legend = screen.getByText("6 to look closer").props;
    expect(legend.numberOfLines).toBe(2);
    expect(legend.style).toMatchObject({ minHeight: 2 * 16 * 3.1 });
  });

  it("keeps a language card's name to two lines at an ordinary size", async () => {
    atScale(1);
    await render(<LanguageCard locale="nb" waiting={nb} onPress={() => {}} />);
    expect(screen.getByText("Norwegian Bokmål").props.numberOfLines).toBe(2);
  });
});
