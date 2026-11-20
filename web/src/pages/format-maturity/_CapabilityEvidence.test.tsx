// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vite-plus/test";
import CapabilityEvidence from "./_CapabilityEvidence";

vi.mock("../../../static/data/content-capabilities.json", () => ({
  default: {
    date: "2026-11-20",
    outputs: [
      {
        id: "sample-format",
        capabilities: [
          {
            id: "write",
            status: "verified",
            detail: "Writes text.",
            evidence: ["writer_test.go#TestWrite"],
          },
        ],
      },
    ],
    connectors: [
      {
        id: "sample-connector",
        capabilities: [
          { id: "fetch", status: "unverified", detail: "No observation.", evidence: [] },
        ],
      },
    ],
  },
}));
vi.mock("../../../static/data/content-capabilities-history.json", () => ({
  default: [
    { date: "2026-06-01", revision: "a".repeat(40), formats: { "sample-format": "L1" } },
    {
      date: "2026-11-20",
      formats: { "sample-format": "L2" },
      outputs: { "sample-format": { write: "verified" } },
      connectors: { "sample-connector": { fetch: "unverified" } },
    },
  ],
}));

afterEach(cleanup);

it("filters evidence by operation, state and inventory", () => {
  render(<CapabilityEvidence dataBaseUrl="/data/" />);
  const table = within(screen.getAllByRole("table")[0]);
  fireEvent.change(screen.getByLabelText("Find"), { target: { value: "sample-format" } });
  expect(table.getByRole("link", { name: "TestWrite" })).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Evidence"), { target: { value: "unverified" } });
  expect(screen.getByText("No operations match these filters.")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Inventory"), { target: { value: "connectors" } });
  fireEvent.change(screen.getByLabelText("Find"), { target: { value: "" } });
  expect(table.getAllByText("unverified").length).toBeGreaterThan(0);
  expect(table.queryByText("verified")).toBeNull();
});

it("links prior format observations and shows when operation history begins", () => {
  render(<CapabilityEvidence dataBaseUrl="/preview/data/" />);
  expect(screen.getByRole("link", { name: "Historical observations (JSON)" }).getAttribute("href"))
    .toBe("/preview/data/content-capabilities-history.json");
  const table = within(screen.getAllByRole("table")[1]);
  expect(table.getByText("First observation")).toBeTruthy();
  expect(table.getAllByText("No observation").length).toBeGreaterThan(0);
  expect(table.getAllByRole("link")[0].getAttribute("href")).toMatch(/\/blob\/[a-f0-9]{40}\//);
});
