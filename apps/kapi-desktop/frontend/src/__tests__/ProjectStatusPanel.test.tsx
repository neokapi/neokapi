import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { ProjectStatusPanel } from "../components/ProjectStatusPanel";
import type { ProjectStatus } from "../components/TranslationStatusPanel";
import type { ConvergenceReport } from "../types/api";

const STATUS: ProjectStatus = {
  projectPath: "/p/app.kapi",
  projectName: "App",
  collections: [
    { name: "ui", blockCount: 100, coverage: { fr: 80 }, targetLanguages: ["fr", "de"] },
  ],
};

const REPORT: ConvergenceReport = {
  project: "App",
  locales: [
    {
      locale: "nb",
      total: 10,
      pct: { draft: 100, translated: 100, reviewed: 50, "signed-off": 0 },
      gated: true,
      shippable: true,
    },
  ],
  review: [{ locale: "nb", file: "en.json", key: "greeting", source: "Welcome back" }],
};

describe("ProjectStatusPanel", () => {
  it("defaults to the ship stage and renders the convergence body without its own title", () => {
    render(<ProjectStatusPanel tabID="t1" status={STATUS} report={REPORT} />);
    // Ship body present...
    expect(document.querySelector("[data-slot='convergence-panel']")).not.toBeNull();
    expect(document.querySelector("[data-slot='convergence-coverage']")).not.toBeNull();
    // ...working body absent...
    expect(document.querySelector("[data-slot='translation-status-panel']")).toBeNull();
    // ...and the embedded panel suppresses its own "Convergence" heading (the
    // toggle is the heading).
    expect(screen.queryByText("Convergence")).toBeNull();
    expect(screen.getByText("Project status")).toBeInTheDocument();
  });

  it("toggles to the working stage and back", async () => {
    render(<ProjectStatusPanel tabID="t1" status={STATUS} report={REPORT} />);

    await userEvent.click(screen.getByRole("tab", { name: /working/i }));
    expect(document.querySelector("[data-slot='translation-status-panel']")).not.toBeNull();
    expect(document.querySelector("[data-slot='convergence-panel']")).toBeNull();
    expect(document.querySelector("[data-slot='locale-coverage']")).not.toBeNull();

    await userEvent.click(screen.getByRole("tab", { name: /ship/i }));
    expect(document.querySelector("[data-slot='convergence-panel']")).not.toBeNull();
    expect(document.querySelector("[data-slot='translation-status-panel']")).toBeNull();
  });

  it("honors defaultView=working", () => {
    render(<ProjectStatusPanel tabID="t1" defaultView="working" status={STATUS} report={REPORT} />);
    expect(document.querySelector("[data-slot='translation-status-panel']")).not.toBeNull();
    const shipTab = screen.getByRole("tab", { name: /ship/i });
    expect(shipTab.getAttribute("aria-selected")).toBe("false");
  });

  it("marks the active stage tab as selected", () => {
    render(<ProjectStatusPanel tabID="t1" status={STATUS} report={REPORT} />);
    expect(screen.getByRole("tab", { name: /ship/i }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("tab", { name: /working/i }).getAttribute("aria-selected")).toBe(
      "false",
    );
  });
});
