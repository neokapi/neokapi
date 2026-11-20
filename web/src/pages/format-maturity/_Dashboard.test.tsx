// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vite-plus/test";
import Dashboard from "./_Dashboard";

vi.mock("./_Formats", () => ({
  default: ({ initialSearch, initialTier }: { initialSearch?: string; initialTier?: string }) => (
    <h2>Formats {initialSearch ?? initialTier}</h2>
  ),
}));
vi.mock("./_CapabilityEvidence", () => ({
  default: ({ group, initialStatus }: { group: string; initialStatus?: string }) => (
    <h2>
      {group} {initialStatus}
    </h2>
  ),
  ObservationHistory: () => <h2>Observations</h2>,
}));
vi.mock("../../../static/data/format-maturity.json", () => ({
  default: {
    generated_at: "2026-11-20",
    formats: [
      { id: "eml", tier: { declared: "available" } },
      { id: "csv", tier: { declared: "maintained" } },
    ],
  },
}));
vi.mock("../../../static/data/content-capabilities.json", () => ({
  default: {
    outputs: [
      { id: "sample-output", capabilities: [{ status: "failed" }, { status: "verified" }] },
    ],
    connectors: [
      { id: "sample-source", capabilities: [{ status: "unverified" }, { status: "declared" }] },
    ],
  },
}));
vi.mock("../../../static/data/content-capabilities-history.json", () => ({
  default: [
    { date: "2026-09-15", formats: { csv: "L1" } },
    { date: "2026-11-20", formats: { csv: "L2", eml: "L3" } },
  ],
}));

const show = () =>
  render(
    <Dashboard
      evidenceBaseUrl="https://example.org/revision/"
      dataBaseUrl="/data/"
      guideUrl="/guide"
    />,
  );
beforeEach(() => window.history.replaceState(null, "", "/format-maturity"));
afterEach(cleanup);

it("starts with counts and changes and opens the selected evidence filter", async () => {
  show();
  expect(screen.getByRole("link", { name: "Input formats 2" })).toBeTruthy();
  expect(screen.getByText("1 implementation grade changed")).toBeTruthy();
  expect(screen.queryByRole("table")).toBeNull();
  fireEvent.click(screen.getByRole("link", { name: "Failed output checks 1" }));
  await waitFor(() => expect(screen.getByRole("heading", { name: "outputs failed" })).toBeTruthy());
  expect(screen.getByRole("link", { name: "Outputs" }).getAttribute("aria-current")).toBe("page");
  fireEvent.click(screen.getByRole("link", { name: "Overview" }));
  await waitFor(() => expect(screen.getByRole("heading", { name: "Format support" })).toBeTruthy());
  fireEvent.click(screen.getByRole("link", { name: "eml" }));
  await waitFor(() => expect(screen.getByRole("heading", { name: "Formats eml" })).toBeTruthy());
});

it.each([
  ["capability-evidence", "outputs"],
  ["observation-history", "Observations"],
  ["unverified-connectors", "connectors unverified"],
  ["format/eml", "Formats eml"],
  ["tier/maintained", "Formats maintained"],
])("opens the requested view from #%s", (hash, heading) => {
  window.history.replaceState(null, "", "/format-maturity#" + hash);
  show();
  expect(screen.getByRole("heading", { name: heading })).toBeTruthy();
});
