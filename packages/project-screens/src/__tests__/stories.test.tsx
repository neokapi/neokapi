// @vitest-environment jsdom
import { describe, expect, it } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { composeStories } from "@storybook/react-vite";
import * as mapStories from "../stories/Map.stories";
import * as readerStories from "../stories/Reader.stories";
import * as rulesStories from "../stories/Rules.stories";

// Every story renders without throwing, and each read-only preview draws no
// control for an action its port does not implement.
describe.each([
  ["Map", mapStories],
  ["Reader", readerStories],
  ["Rules", rulesStories],
])("%s stories", (_, stories) => {
  const composed = composeStories(stories);
  it.each(Object.entries(composed))("%s renders", async (name, Story) => {
    const { container } = render(<Story />);
    if (name === "ReadOnly") await screen.findAllByRole("heading");
    expect(container.textContent?.length ?? 0).toBeGreaterThan(0);
  });
});

describe("read-only stories", () => {
  it("draw no action", async () => {
    const { ReadOnly: MapReadOnly } = composeStories(mapStories);
    const { ReadOnly: ReaderReadOnly } = composeStories(readerStories);
    const { ReadOnly: RulesReadOnly } = composeStories(rulesStories);
    render(<MapReadOnly />);
    render(<ReaderReadOnly />);
    render(<RulesReadOnly />);
    await screen.findByRole("heading", { name: "docs/billing.md" });
    await screen.findByRole("listbox", { name: "Suggestions to decide" });
    expect(screen.queryByRole("button", { name: "Run on everything" })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Fix/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Keep/ })).toBeNull();
  });
});
