// @vitest-environment jsdom
import { describe, expect, it } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { composeStories } from "@storybook/react-vite";
import * as stories from "../stories/ContextAtlas.stories";
import * as mapStories from "../stories/MapView.stories";

// Every story renders without throwing: the fixture, the views and the
// detail pane hold together for empty, one, many and long content.
describe("stories", () => {
  const composed = composeStories(stories);
  it.each(Object.entries(composed))("%s renders", (_, Story) => {
    const { container } = render(<Story />);
    expect(container.textContent?.length ?? 0).toBeGreaterThan(0);
  });
});

// Each map story draws once its shapes load, or its tiles at once.
describe("map stories", () => {
  const composed = composeStories(mapStories);
  it.each(Object.entries(composed))("%s draws", async (name, Story) => {
    render(<Story />);
    if (name === "Grid") {
      expect(screen.getByTestId("atlas-map-grid")).toBeInTheDocument();
      return;
    }
    const svg = screen.getByTestId("atlas-map-shapes");
    await screen.findByText((_, el) => el === svg && svg.getAttribute("aria-busy") === "false");
    expect(svg.querySelectorAll("[role=button]").length).toBeGreaterThan(0);
  });
});
