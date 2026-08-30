// @vitest-environment jsdom
import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { ContextExplorer } from "../ContextExplorer";
import { ContextSearchPanel } from "../ContextSearchPanel";
import { GovernsPane } from "../GovernsPane";
import { LadderBreadcrumbs } from "../LadderBreadcrumbs";
import { LivesPane } from "../LivesPane";
import { RelatesPane } from "../RelatesPane";
import { ScopeFilterBar } from "../ScopeFilterBar";
import { ScopeProvider } from "../ScopeProvider";
import type { ContextDataSource } from "../source";
import type { Dimension } from "../scope";
import {
  STALENESS_NOTE,
  freeScope,
  makeFreeSource,
  makePinnedSource,
  pinnedScope,
} from "../stories/fixtures";

const PINNED: Dimension[] = ["workspace", "project", "stream"];

function mountPinned(
  ui: ReactNode,
  source: ContextDataSource = makePinnedSource(),
  onScopeChange?: (next: unknown) => void,
) {
  return render(
    <ScopeProvider
      source={source}
      pinned={PINNED}
      scope={pinnedScope}
      onScopeChange={onScopeChange as never}
    >
      {ui}
    </ScopeProvider>,
  );
}

function mountFree(ui: ReactNode, source: ContextDataSource = makeFreeSource()) {
  return render(
    <ScopeProvider source={source} pinned={["workspace"]} scope={freeScope}>
      {ui}
    </ScopeProvider>,
  );
}

describe("LadderBreadcrumbs", () => {
  it("renders one rung per ladder step and the point's address", () => {
    mountPinned(<LadderBreadcrumbs />);
    const ladder = screen.getByTestId("context-ladder");
    expect(within(ladder).getByText("docs")).toBeInTheDocument();
    expect(within(ladder).getByText("billing.md")).toBeInTheDocument();
    expect(screen.getByTestId("context-address")).toHaveTextContent(
      "context://docs/help/billing.md",
    );
  });

  it("names the free dimensions in the address of a workspace mount", () => {
    render(
      <ScopeProvider source={makeFreeSource()} pinned={["workspace"]} scope={pinnedScope}>
        <LadderBreadcrumbs />
      </ScopeProvider>,
    );
    expect(screen.getByTestId("context-address")).toHaveTextContent(
      "context://site@main/docs/help/billing.md",
    );
  });

  it("moves the scope when a navigable rung is chosen", async () => {
    const onScopeChange = vi.fn();
    mountPinned(<LadderBreadcrumbs />, makePinnedSource(), onScopeChange);
    await userEvent.click(screen.getByRole("button", { name: "docs" }));
    expect(onScopeChange).toHaveBeenCalledWith({
      workspace: "northsea",
      project: "site",
      stream: "main",
      collection: "docs",
    });
  });

  it("renders a pinned rung as text, not a control", () => {
    mountPinned(<LadderBreadcrumbs />);
    expect(screen.queryByRole("button", { name: "northsea" })).not.toBeInTheDocument();
    expect(screen.getByText("northsea")).toBeInTheDocument();
  });
});

describe("ScopeFilterBar", () => {
  it("shows pinned dimensions as chips and free ones as pickers", async () => {
    mountFree(<ScopeFilterBar />);
    const bar = screen.getByTestId("scope-filter-bar");
    expect(within(bar).getByText("northsea")).toBeInTheDocument();
    await waitFor(() => expect(within(bar).getByLabelText("Project")).toBeInTheDocument());
    expect(within(bar).queryByLabelText("Workspace")).not.toBeInTheDocument();
  });

  it("offers no picker for a dimension the mount pins", () => {
    mountPinned(<ScopeFilterBar />);
    const bar = screen.getByTestId("scope-filter-bar");
    expect(within(bar).queryByLabelText("Project")).not.toBeInTheDocument();
    expect(within(bar).getByText("site")).toBeInTheDocument();
  });

  it("drops a dimension the source claims but the mount pins", () => {
    // The source can answer across projects; this mount shows exactly one. A
    // picker that renders and then refuses to move is worse than none.
    const overreaching = makePinnedSource();
    overreaching.capabilities = { ...overreaching.capabilities, free: ["project", "collection"] };
    mountPinned(<ScopeFilterBar />, overreaching);
    const bar = screen.getByTestId("scope-filter-bar");
    expect(within(bar).queryByLabelText("Project")).not.toBeInTheDocument();
    expect(within(bar).getByLabelText("Collection")).toBeInTheDocument();
  });
});

describe("GovernsPane", () => {
  it("names the profile, the channel and the voice in force", async () => {
    mountPinned(<GovernsPane />);
    await waitFor(() => expect(screen.getByTestId("governs-voice")).toBeInTheDocument());
    expect(screen.getByTestId("governs-profile")).toHaveTextContent("support");
    expect(screen.getByTestId("governs-channel")).toHaveTextContent("northsea/docs");
    expect(screen.getByTestId("governs-voice")).toHaveTextContent("Support");
  });

  it("marks a discouraged term and names its replacement", async () => {
    mountPinned(<GovernsPane />);
    await waitFor(() => expect(screen.getAllByTestId("governs-term").length).toBe(2));
    const discouraged = screen
      .getAllByTestId("governs-term")
      .find((el) => el.getAttribute("data-discouraged") === "true");
    expect(discouraged).toHaveTextContent("log in");
    expect(discouraged).toHaveTextContent("use sign in");
  });

  it("shows an expired validity window rather than hiding the profile", async () => {
    mountPinned(<GovernsPane />);
    await waitFor(() => expect(screen.getByTestId("governs-windows")).toBeInTheDocument());
    expect(screen.getByTestId("governs-windows")).toHaveTextContent("expired");
  });

  it("renders the staleness note above the answer", async () => {
    mountPinned(<GovernsPane />, makePinnedSource({ notes: [STALENESS_NOTE] }));
    await waitFor(() => expect(screen.getByTestId("context-notes")).toBeInTheDocument());
    expect(screen.getByTestId("context-notes")).toHaveTextContent("terms moved");
  });

  // An empty section suggests a broken app; an absent one suggests nothing.
  it("leaves out the rules and clearance sections until a source fills them", async () => {
    mountPinned(<GovernsPane />, makePinnedSource({ bareGovernance: true }));
    await waitFor(() => expect(screen.getByTestId("governs-voice")).toBeInTheDocument());
    expect(screen.queryByTestId("governs-rules")).not.toBeInTheDocument();
    expect(screen.queryByTestId("governs-clearance")).not.toBeInTheDocument();
  });

  it("renders them once a source does", async () => {
    mountPinned(
      <GovernsPane />,
      makePinnedSource({
        governance: {
          clearance: "internal",
          rules: [{ id: "r1", label: "No unhedged claims", gate: "voice", state: "active" }],
        },
      }),
    );
    await waitFor(() => expect(screen.getByTestId("governs-rules")).toBeInTheDocument());
    expect(screen.getByTestId("governs-rules")).toHaveTextContent("No unhedged claims");
    expect(screen.getByTestId("governs-clearance")).toHaveTextContent("internal");
  });

  it("renders the voice guide the source carries", async () => {
    mountPinned(
      <GovernsPane />,
      makePinnedSource({
        governance: { voice: { name: "Support", guide: "Write plainly. Lead with the fix." } },
      }),
    );
    await waitFor(() => expect(screen.getByTestId("governs-voice-guide")).toBeInTheDocument());
    expect(screen.getByTestId("governs-voice-guide")).toHaveTextContent("Lead with the fix.");
  });

  it("shows a failed read as a failure, never as an empty answer", async () => {
    mountPinned(<GovernsPane />, makePinnedSource({ failing: true }));
    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(screen.getByRole("alert")).toHaveTextContent("unreachable");
  });
});

describe("LivesPane", () => {
  it("renders per-locale coverage with its ship state", async () => {
    mountPinned(<LivesPane />);
    await waitFor(() => expect(screen.getAllByTestId("lives-coverage").length).toBe(2));
    const nb = screen
      .getAllByTestId("lives-coverage")
      .find((el) => el.getAttribute("data-locale") === "nb");
    expect(nb).toHaveTextContent("83%");
    expect(within(nb!).getByText("pending")).toBeInTheDocument();
    expect(nb).toHaveTextContent("3 stale");
  });

  it("flags an item whose decision cites a superseded source", async () => {
    mountPinned(<LivesPane />);
    await waitFor(() => expect(screen.getAllByTestId("lives-item").length).toBe(2));
    const stale = screen
      .getAllByTestId("lives-item")
      .filter((el) => el.getAttribute("data-stale") === "true");
    expect(stale).toHaveLength(1);
  });

  it("states its reach rather than rendering an empty section", () => {
    mountPinned(<LivesPane />, makePinnedSource({ withoutContent: true }));
    expect(screen.getByTestId("lives-pane")).toHaveTextContent("does not read content");
  });
});

describe("RelatesPane", () => {
  const subject = { kind: "concept" as const, id: "c-signin", label: "sign in" };

  it("is idle until something is selected", () => {
    mountPinned(<RelatesPane />);
    expect(screen.getByTestId("relates-pane")).toHaveTextContent("Nothing selected");
  });

  it("says a project-reach answer covers one project", async () => {
    mountPinned(<RelatesPane subject={subject} />);
    await waitFor(() => expect(screen.getByTestId("relates-occurrences")).toBeInTheDocument());
    expect(screen.getByTestId("relates-pane")).toHaveAttribute("data-reach", "project");
    expect(screen.getByTestId("relates-pane")).toHaveTextContent("this project only");
    expect(screen.queryByTestId("relates-projects")).not.toBeInTheDocument();
  });

  it("answers which projects use a concept at workspace reach", async () => {
    mountFree(<RelatesPane subject={subject} />);
    await waitFor(() => expect(screen.getByTestId("relates-projects")).toBeInTheDocument());
    expect(screen.getByTestId("relates-pane")).toHaveAttribute("data-reach", "workspace");
    expect(screen.getAllByTestId("relates-project")).toHaveLength(2);
  });

  // Naming a project is half an answer. The reader is deciding whether to act,
  // and "which projects use this" without "and how it stands there" leaves them
  // to guess — which is the guess a workspace-global flag was making for them.
  it("says how the concept stands in each project it names", async () => {
    mountFree(<RelatesPane subject={subject} />);
    await waitFor(() => expect(screen.getByTestId("relates-projects")).toBeInTheDocument());

    const rows = screen.getAllByTestId("relates-project");
    expect(rows[0]).toHaveAttribute("data-discouraged", "true");
    expect(rows[0]).toHaveTextContent("deprecated");
    expect(rows[0]).toHaveTextContent("say sign in");
    expect(rows[0]).toHaveTextContent("log in");

    expect(rows[1]).not.toHaveAttribute("data-discouraged", "true");
    expect(rows[1]).toHaveTextContent("preferred");
  });

  it("marks a decision whose basis no longer matches", async () => {
    mountPinned(<RelatesPane subject={subject} />);
    await waitFor(() => expect(screen.getByTestId("relates-blessings")).toBeInTheDocument());
    expect(screen.getByTestId("relates-blessing")).toHaveAttribute("data-stale", "true");
  });

  it("hides itself when the surface holds no relations", () => {
    mountPinned(<RelatesPane subject={subject} />, makePinnedSource({ withoutRelations: true }));
    expect(screen.getByTestId("relates-pane")).toHaveTextContent("no relations");
  });
});

describe("ContextSearchPanel", () => {
  it("asks nothing until a query is submitted", () => {
    mountPinned(<ContextSearchPanel />);
    expect(screen.queryByTestId("search-results")).not.toBeInTheDocument();
  });

  it("groups results by kind and never merges them into one list", async () => {
    mountPinned(<ContextSearchPanel />);
    await userEvent.type(screen.getByTestId("search-input"), "sign in");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    await waitFor(() => expect(screen.getByTestId("search-results")).toBeInTheDocument());
    const kinds = screen.getAllByTestId("search-group").map((el) => el.getAttribute("data-kind"));
    expect(kinds).toEqual(["terms", "precedent", "profiles"]);
  });

  it("makes a chosen result the subject of the relations question", async () => {
    const onSelect = vi.fn();
    mountPinned(<ContextSearchPanel onSelect={onSelect} />);
    await userEvent.type(screen.getByTestId("search-input"), "sign in");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    await waitFor(() => expect(screen.getAllByTestId("search-hit").length).toBeGreaterThan(0));
    await userEvent.click(screen.getAllByTestId("search-hit")[0]);
    expect(onSelect).toHaveBeenCalledWith({
      kind: "concept",
      id: "c-signin",
      label: "sign in",
    });
  });
});

describe("ContextExplorer", () => {
  it("puts three panes over one selection", async () => {
    mountPinned(<ContextExplorer />);
    expect(screen.getByTestId("governs-pane")).toBeInTheDocument();
    expect(screen.getByTestId("lives-pane")).toBeInTheDocument();
    expect(screen.getByTestId("relates-pane")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("governs-voice")).toBeInTheDocument());
  });

  it("carries a search selection into the relations pane", async () => {
    mountPinned(<ContextExplorer />);
    await userEvent.type(screen.getByTestId("search-input"), "sign in");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    await waitFor(() => expect(screen.getAllByTestId("search-hit").length).toBeGreaterThan(0));
    await userEvent.click(screen.getAllByTestId("search-hit")[0]);
    await waitFor(() => expect(screen.getByTestId("relates-occurrences")).toBeInTheDocument());
  });
});
