// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor, within } from "./testUtils";
import { cleanup } from "@testing-library/react";
import { ContextExplorerView } from "../components/ContextExplorerView";
import { createLocalContextSource, type ContextBackend } from "../lib/localContextSource";

afterEach(cleanup);

function backend(): ContextBackend {
  return {
    at: vi.fn().mockResolvedValue({
      point: { profile: "support", channel: "docs", default: false },
      scope: "project",
      voice: { name: "Support" },
      terms: [],
      profiles: [],
      notes: [],
    }),
    governs: vi.fn().mockResolvedValue({
      point: { profile: "support", channel: "docs", collection: "Docs", default: false },
      voice: { name: "Support", field: "profiles.support.voice" },
      concepts: [
        {
          id: "c-signin",
          terms: [{ text: "sign in", locale: "en-US", status: "preferred" }],
        },
      ],
      concepts_total: 1,
      profiles: [],
      notes: [],
    }),
    lives: vi.fn().mockResolvedValue({
      point: { collection: "Docs", default: false },
      collections: [],
      coverage: [{ locale: "nb-NO", units: 2, covered: 1 }],
      items: [],
      items_total: 0,
      notes: [],
    }),
    relates: vi.fn().mockResolvedValue({
      subject: "sign in",
      reach: "project",
      occurrences: [],
      occurrences_total: 0,
      blessings: [],
      notes: [],
    }),
    search: vi
      .fn()
      .mockResolvedValue({ answer: { query: "", scope: "project", notes: [] }, content: [] }),
    options: vi.fn().mockResolvedValue([{ value: "Docs" }]),
  };
}

describe("ContextExplorerView", () => {
  it("pins the project and stream to the open tab", async () => {
    render(
      <ContextExplorerView
        tabID="tab-1"
        projectName="Northsea"
        source={createLocalContextSource("tab-1", backend())}
      />,
    );

    const ladder = screen.getByTestId("context-ladder");
    expect(within(ladder).getByText("Northsea")).toBeInTheDocument();
    // A pinned rung reads but does not move.
    expect(screen.queryByRole("button", { name: "Northsea" })).not.toBeInTheDocument();

    const bar = screen.getByTestId("scope-filter-bar");
    expect(within(bar).queryByLabelText("Project")).not.toBeInTheDocument();
    await waitFor(() => expect(within(bar).getByLabelText("Collection")).toBeInTheDocument());
  });

  it("renders the three panes over the project's own answer", async () => {
    render(
      <ContextExplorerView
        tabID="tab-1"
        projectName="Northsea"
        source={createLocalContextSource("tab-1", backend())}
      />,
    );

    await waitFor(() => expect(screen.getByTestId("governs-voice")).toHaveTextContent("Support"));
    expect(screen.getByTestId("governs-profile")).toHaveTextContent("support");
    await waitFor(() => expect(screen.getByTestId("lives-coverage")).toHaveTextContent("50%"));
    expect(screen.getByTestId("relates-pane")).toHaveAttribute("data-reach", "project");
  });

  it("stands on the stream the project names, not a literal", () => {
    render(
      <ContextExplorerView
        tabID="tab-1"
        projectName="Northsea"
        stream="release-2"
        source={createLocalContextSource("tab-1", backend())}
      />,
    );
    const bar = screen.getByTestId("scope-filter-bar");
    expect(within(bar).getByText("release-2")).toBeInTheDocument();
    expect(within(bar).queryByText("main")).not.toBeInTheDocument();
  });

  it("says which finding pinned it, and stands at that point", () => {
    render(
      <ContextExplorerView
        tabID="tab-1"
        projectName="Northsea"
        stream="main"
        pin={{ coordinate: "support/docs", collection: "Docs", rule: "log in" }}
        source={createLocalContextSource("tab-1", backend())}
      />,
    );
    const strip = screen.getByTestId("explorer-pin");
    expect(strip).toHaveTextContent("log in");
    expect(strip).toHaveTextContent("support/docs");
  });

  it("shows no pin strip when the reader arrived on their own", () => {
    render(
      <ContextExplorerView
        tabID="tab-1"
        projectName="Northsea"
        stream="main"
        source={createLocalContextSource("tab-1", backend())}
      />,
    );
    expect(screen.queryByTestId("explorer-pin")).not.toBeInTheDocument();
  });
});
