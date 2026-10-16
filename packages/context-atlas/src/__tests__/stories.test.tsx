// @vitest-environment jsdom
import { describe, expect, it } from "vite-plus/test";
import { render } from "@testing-library/react";
import { composeStories } from "@storybook/react-vite";
import * as stories from "../stories/ContextAtlas.stories";

// Every story renders without throwing: the fixture, the views and the
// detail pane hold together for empty, one, many and long content.
describe("stories", () => {
  const composed = composeStories(stories);
  it.each(Object.entries(composed))("%s renders", (_, Story) => {
    const { container } = render(<Story />);
    expect(container.textContent?.length ?? 0).toBeGreaterThan(0);
  });
});
