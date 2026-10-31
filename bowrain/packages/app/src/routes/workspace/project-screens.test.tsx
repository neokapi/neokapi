// @vitest-environment jsdom
import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PermissionName, ReaderFixResult } from "@neokapi/ui";
import { READER_WITH_FIXES } from "@neokapi/project-screens/fixtures";
import { ProjectMapPage } from "./project-map";
import { ProjectReaderPage } from "./project-reader";
import { ProjectRulesPage } from "./project-rules";
import { fixtureScreensPort } from "./project-screens-fixture-port";
import { bowrainOpens, permittedPort, proposalsByFinding } from "./project-screens-port";

/** What a member holding exactly `held` may do. */
const holding =
  (...held: PermissionName[]) =>
  (p: PermissionName) =>
    held.includes(p);

const MEMBER = holding("view_content", "review", "manage_terms");
const VIEWER = holding();

describe("the Map route", () => {
  it("offers the project-wide check to a member who may view the content", async () => {
    const runChecks = vi.fn(async () => {});
    render(
      <ProjectMapPage
        port={permittedPort(fixtureScreensPort({ runChecks }), holding("view_content"))}
        projectName="Fernwell"
        navigate={vi.fn()}
      />,
    );
    await userEvent.click(await screen.findByRole("button", { name: "Run on everything" }));
    expect(runChecks).toHaveBeenCalled();
  });

  it("draws no check action for a member without the permission", async () => {
    render(
      <ProjectMapPage
        port={permittedPort(fixtureScreensPort(), VIEWER)}
        projectName="Fernwell"
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("heading", { name: "Fernwell" });
    expect(screen.queryByRole("button", { name: "Run on everything" })).toBeNull();
  });

  it("opens a file in the Reader by its path", async () => {
    const navigate = vi.fn();
    render(
      <ProjectMapPage
        port={permittedPort(fixtureScreensPort(), MEMBER)}
        projectName="Fernwell"
        navigate={navigate}
      />,
    );
    const open = await screen.findAllByRole("button", { name: "Open" });
    await userEvent.click(open[0]);
    expect(navigate).toHaveBeenCalledWith(expect.objectContaining({ kind: "reader" }));
  });
});

describe("the Reader route", () => {
  it("files a fix as a proposal and shows its id under the finding", async () => {
    const { container } = render(
      <ProjectReaderPage
        port={permittedPort(fixtureScreensPort(), MEMBER)}
        projectId="p1"
        path="docs/billing.md"
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("heading", { name: "docs/billing.md" });
    const fix = screen.getAllByRole("button", { name: "Fix" });
    await userEvent.click(fix[0]);
    await waitFor(() =>
      expect(container.querySelector("[data-slot='reader-proposed']")).toHaveTextContent(
        "Proposed sp-fernw",
      ),
    );
  });

  it("draws no Fix for a member who may not file a proposal", async () => {
    render(
      <ProjectReaderPage
        port={permittedPort(fixtureScreensPort(), holding("view_content"))}
        projectId="p1"
        path="docs/billing.md"
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("heading", { name: "docs/billing.md" });
    expect(screen.queryByRole("button", { name: /^Fix/ })).toBeNull();
  });

  it("goes back to the Map", async () => {
    const navigate = vi.fn();
    render(
      <ProjectReaderPage
        port={fixtureScreensPort()}
        projectId="p1"
        path="docs/billing.md"
        navigate={navigate}
      />,
    );
    await userEvent.click(await screen.findByRole("button", { name: "Map" }));
    expect(navigate).toHaveBeenCalledWith({ kind: "map" });
  });
});

describe("the Rules route", () => {
  it("offers Keep to a member who governs rules, and lists the kept rules", async () => {
    render(
      <ProjectRulesPage port={permittedPort(fixtureScreensPort(), MEMBER)} navigate={vi.fn()} />,
    );
    expect(await screen.findByRole("button", { name: /^Keep/ })).toBeInTheDocument();
    const kept = await screen.findByRole("region", { name: "Kept by" });
    expect(within(kept).getAllByRole("listitem").length).toBeGreaterThan(0);
  });

  it("draws no Keep, Not a rule or Later for a member who governs no rule", async () => {
    render(
      <ProjectRulesPage
        port={permittedPort(fixtureScreensPort(), holding("view_content", "review"))}
        navigate={vi.fn()}
      />,
    );
    await screen.findByRole("listbox", { name: "Suggestions to decide" });
    expect(screen.queryByRole("button", { name: /^Keep/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Not a rule/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Later/ })).toBeNull();
  });
});

describe("the port", () => {
  it("opens the Map, the Reader, Rules, the kept rules and the Source content page", () => {
    expect(bowrainOpens({ kind: "map" })).toBe(true);
    expect(bowrainOpens({ kind: "reader", path: "a.md" })).toBe(true);
    expect(bowrainOpens({ kind: "rules" })).toBe(true);
    expect(bowrainOpens({ kind: "kept" })).toBe(true);
    expect(bowrainOpens({ kind: "blocks" })).toBe(true);
    expect(bowrainOpens({ kind: "checks" })).toBe(false);
    expect(bowrainOpens({ kind: "setup" })).toBe(false);
    expect(bowrainOpens({ kind: "connect-assistant" })).toBe(false);
  });

  it("sets each proposal of a whole-file fix on the findings of its block", () => {
    // READER_WITH_FIXES has two fixable findings, both on block p2.
    const result: ReaderFixResult = { proposals: ["sp-1"], refused: [] };
    expect(proposalsByFinding(READER_WITH_FIXES, result)).toEqual([
      { finding_id: "a1f0-1", proposal: "sp-1" },
      { finding_id: "b2c3-1", proposal: "sp-1" },
    ]);
  });

  it("skips a block the server refused", () => {
    const result: ReaderFixResult = {
      proposals: [],
      refused: [{ finding_id: "b2c3-1", reason: "The change was refused." }],
    };
    expect(proposalsByFinding(READER_WITH_FIXES, result)).toEqual([]);
  });
});
