import { useState } from "react";
import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { ErrorProvider } from "../components/ErrorBanner";
import { ChecksPanel } from "../components/ChecksPanel";
import { MapPage } from "../components/MapPage";
import { api } from "../hooks/useApi";
import { MAP_PENDING } from "../stories/fixtures/mapSummary";
import type { CheckRunResult } from "../types/api";

afterEach(() => {
  vi.restoreAllMocks();
});

/** The run the Map counted: four findings over two files. */
const LAST_RUN: CheckRunResult = {
  pass: false,
  verdict: "failed",
  score: 71,
  files: [
    {
      path: "/work/fernwell/docs/billing.md",
      findings: [
        { category: "vocabulary", fails: true, message: 'Forbidden term "customer" found' },
        { category: "vocabulary", fails: true, message: 'Forbidden term "business" found' },
        { category: "voice", fails: false, message: "Sentence is longer than the voice allows" },
      ],
    },
    {
      path: "/work/fernwell/app/strings/en.json",
      findings: [
        { category: "placeholder", fails: true, message: "Placeholder {name} is missing" },
      ],
    },
  ],
};

/** The Map, and the checks page its findings count leads to, as ViewSwitch routes them. */
function MapThenChecks() {
  const [view, setView] = useState("map");
  return view === "checks" ? (
    <ChecksPanel tabID="t1" />
  ) : (
    <MapPage
      tabID="t1"
      projectName="Fernwell"
      summary={MAP_PENDING}
      onDecide={vi.fn()}
      onNavigate={setView}
    />
  );
}

describe("the Map's findings count", () => {
  it("opens the checks page on the findings it counted", async () => {
    vi.spyOn(api, "lastProjectChecks").mockResolvedValue(LAST_RUN);
    const run = vi.spyOn(api, "runChecks").mockResolvedValue(null);
    render(
      <ErrorProvider>
        <MapThenChecks />
      </ErrorProvider>,
    );
    await userEvent.click(screen.getByRole("button", { name: "4 findings" }));
    expect(await screen.findByText('Forbidden term "customer" found')).toBeInTheDocument();
    expect(screen.getByText("Placeholder {name} is missing")).toBeInTheDocument();
    expect(run).not.toHaveBeenCalled();
  });
});
