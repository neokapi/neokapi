import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";

import { ConvergencePanel } from "../components/ConvergencePanel";
import type { ConvergenceReport } from "../types/api";

function reportFixture(overrides: Partial<ConvergenceReport> = {}): ConvergenceReport {
  return {
    project: "Demo",
    locales: [
      {
        locale: "nb",
        total: 10,
        pct: { draft: 100, translated: 100, reviewed: 50, "signed-off": 0 },
        gated: true,
        shippable: true,
      },
      {
        locale: "de",
        total: 10,
        pct: { draft: 100, translated: 60, reviewed: 0, "signed-off": 0 },
        gated: true,
        shippable: false,
        pending: [{ state: "translated", actual: 60, required: 100 }],
      },
    ],
    review: [
      { locale: "nb", file: "en.json", key: "greeting", source: "Welcome back" },
      { locale: "de", file: "en.json", key: "farewell", source: "See you soon" },
    ],
    ...overrides,
  };
}

describe("ConvergencePanel", () => {
  it("renders a coverage row per locale scope", () => {
    render(<ConvergencePanel tabID="t1" report={reportFixture()} />);
    const grid = document.querySelector("[data-slot='convergence-coverage']") as HTMLElement;
    expect(grid).not.toBeNull();
    expect(grid.querySelectorAll("[data-locale]")).toHaveLength(2);
  });

  it("marks a shippable scope and a pending scope distinctly", () => {
    render(<ConvergencePanel tabID="t1" report={reportFixture()} />);
    expect(screen.getByText("shippable")).toBeInTheDocument();
    expect(screen.getByText("pending")).toBeInTheDocument();
  });

  it("lists the review queue with its count", () => {
    render(<ConvergencePanel tabID="t1" report={reportFixture()} />);
    const review = document.querySelector("[data-slot='convergence-review']") as HTMLElement;
    expect(review).not.toBeNull();
    expect(screen.getByText("Welcome back")).toBeInTheDocument();
    expect(screen.getByText("See you soon")).toBeInTheDocument();
  });

  it("shows the empty-queue state when nothing awaits review", () => {
    render(<ConvergencePanel tabID="t1" report={reportFixture({ review: [] })} />);
    expect(document.querySelector("[data-slot='convergence-review-empty']")).not.toBeNull();
    expect(document.querySelector("[data-slot='convergence-review']")).toBeNull();
  });

  it("renders the source-readiness row when source coverage is present", () => {
    render(
      <ConvergencePanel
        tabID="t1"
        report={reportFixture({
          source: {
            total: 10,
            pct: { authored: 100, checked: 80, approved: 0 },
            gated: true,
            shippable: false,
            pending: [{ state: "checked", actual: 80, required: 100 }],
          },
        })}
      />,
    );
    expect(document.querySelector("[data-slot='convergence-source']")).not.toBeNull();
    expect(screen.getByText("source")).toBeInTheDocument();
  });

  it("renders the bring-up-to-date action", () => {
    render(<ConvergencePanel tabID="t1" report={reportFixture()} />);
    expect(document.querySelector("[data-slot='convergence-bring-up-to-date']")).not.toBeNull();
  });
});
