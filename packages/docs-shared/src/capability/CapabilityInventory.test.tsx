// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vite-plus/test";
import CapabilityInventory, {
  CapabilityCoverage,
  type CapabilityLabels,
} from "./CapabilityInventory";
import { operationChanges } from "./history";

const labels: CapabilityLabels = {
  heading: "Connector operations",
  filter: "Find a connector or operation",
  status: "Evidence status",
  allStatuses: "All statuses",
  empty: "No operations match these filters.",
  clear: "Clear filters",
  count: (shown, total) => `${shown} of ${total} connectors`,
  states: {
    failed: "Failed",
    unverified: "Unverified",
    verified: "Verified",
    declared: "Declared",
  },
  subject: (id) => ({ wordpress: "WordPress", figma: "Figma" })[id] ?? id,
  operation: (id) => ({ fetch: "Fetch content", list: "List content" })[id] ?? id,
};

const subjects = [
  {
    id: "figma",
    capabilities: [
      {
        id: "fetch",
        status: "verified",
        detail: "Read design text.",
        evidence: ["figma_test.go#TestFigmaFetch"],
      },
    ],
  },
  {
    id: "wordpress",
    capabilities: [
      { id: "fetch", status: "verified", detail: "Read posts.", evidence: ["wordpress_test.go"] },
      { id: "list", status: "unverified", detail: "No listing fixture.", evidence: [] },
    ],
  },
];

afterEach(cleanup);

it("leads with the weakest operation and links each test to its file", () => {
  render(
    <CapabilityInventory
      subjects={subjects}
      labels={labels}
      evidenceBaseUrl="https://example.org/blob/abc/"
    />,
  );
  const rows = within(screen.getByRole("list")).getAllByRole("button");
  expect(rows.map((row) => row.textContent)).toEqual([
    "WordPressList contentUnverified",
    "FigmaFetch contentVerified",
  ]);
  fireEvent.click(rows[1]);
  expect(screen.getByRole("link", { name: "TestFigmaFetch" }).getAttribute("href")).toBe(
    "https://example.org/blob/abc/figma_test.go",
  );
});

it("filters by status and text and clears both", () => {
  render(
    <CapabilityInventory
      subjects={subjects}
      labels={labels}
      evidenceBaseUrl="/"
      initialStatus="unverified"
    />,
  );
  expect(screen.getByRole("status").textContent).toBe("1 of 2 connectors");
  fireEvent.change(screen.getByRole("searchbox"), { target: { value: "figma" } });
  expect(screen.getByText("No operations match these filters.")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
  expect(screen.getByRole("status").textContent).toBe("2 of 2 connectors");
});

it("summarises an observation and lists the operations that changed", () => {
  const previous = { wordpress: { fetch: "unverified", list: "unverified" } };
  const profile = { wordpress: { fetch: "verified" }, figma: { fetch: "failed" } };
  expect(operationChanges(profile, previous)).toEqual([
    { subject: "figma", operation: "fetch", before: undefined, after: "failed" },
    { subject: "wordpress", operation: "fetch", before: "unverified", after: "verified" },
    { subject: "wordpress", operation: "list", before: "unverified", after: undefined },
  ]);
  expect(operationChanges(profile, undefined)).toEqual([]);
  render(
    <CapabilityCoverage
      profile={profile}
      previous={previous}
      labels={{
        none: "No observation",
        summary: ({ subjects, verified, failed }) => `${subjects} · ${verified} · ${failed}`,
        changes: (count) => `${count} changed`,
        change: ({ subject, operation, before, after }) =>
          `${subject}/${operation} ${before ?? "absent"} → ${after ?? "absent"}`,
      }}
    />,
  );
  expect(screen.getByText("2 · 1 · 1 · 3 changed")).toBeTruthy();
  expect(screen.getByText("wordpress/list unverified → absent")).toBeTruthy();
});
