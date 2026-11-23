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
    expect(screen.getByText("Up to date").props.numberOfLines).toBeUndefined();
  });

  it("lets a language card's name wrap at the largest size", async () => {
    atScale(3.1);
    await render(<LanguageCard locale="nb" waiting={nb} onPress={() => {}} />);
    expect(screen.getByText("Norwegian Bokmål").props.numberOfLines).toBeUndefined();
  });

  it("keeps a language card's name to two lines at an ordinary size", async () => {
    atScale(1);
    await render(<LanguageCard locale="nb" waiting={nb} onPress={() => {}} />);
    expect(screen.getByText("Norwegian Bokmål").props.numberOfLines).toBe(2);
  });
});
