// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vite-plus/test";
import CapabilityEvidence, { ObservationHistory } from "./_CapabilityEvidence";

vi.mock("../../../static/data/content-capabilities.json", () => ({
  default: {
    date: "2026-11-20",
    outputs: [
      {
        id: "doclang",
        capabilities: [
          {
            id: "project-text",
            status: "verified",
            detail: "Recovered the fixture text.",
            evidence: [],
          },
        ],
      },
      {
        id: "markdown",
        capabilities: [
          {
            id: "structure",
            status: "verified",
            detail: "Retained the heading.",
            evidence: ["tools/structure.go"],
          },
          {
            id: "email",
            status: "failed",
            detail: "The output lost the subject.",
            evidence: ["tools/mail_test.go#TestConvertEmail"],
          },
        ],
      },
    ],
  },
}));

vi.mock("../../../static/data/content-capabilities-history.json", () => ({
  default: [
    { date: "2026-05-01", revision: "a".repeat(40), formats: { markdown: "L1" } },
    {
      date: "2026-06-01",
      revision: "b".repeat(40),
      formats: { markdown: "L2" },
      outputs: { markdown: { email: "failed", structure: "verified" } },
    },
    {
      date: "2026-11-20",
      formats: { markdown: "L2" },
      outputs: { markdown: { email: "verified" }, doclang: { structure: "verified" } },
    },
  ],
}));

afterEach(cleanup);

it("collapses evidence by subject and leads with the failed operation", () => {
  render(
    <CapabilityEvidence evidenceBaseUrl="https://github.com/neokapi/neokapi/blob/review-commit/" />,
  );
  const subjects = within(screen.getByRole("list")).getAllByRole("button");
  expect(subjects).toHaveLength(2);
  expect(subjects[0].textContent).toMatch(/Markdown.*Convert email.*Failed/);
  expect(subjects[0].getAttribute("aria-expanded")).toBe("false");
  expect(screen.queryByText("The output lost the subject.")).toBeNull();
  expect(screen.queryByRole("link")).toBeNull();
  expect(screen.queryByRole("heading", { name: "Observation history" })).toBeNull();
  fireEvent.click(subjects[0]);
  const details = within(document.getElementById(subjects[0].getAttribute("aria-controls")!)!);
  expect(subjects[0].getAttribute("aria-expanded")).toBe("true");
  expect(details.getByText("The output lost the subject.")).toBeTruthy();
  expect(details.getByText("Retained the heading.")).toBeTruthy();
  expect(details.getByRole("link", { name: "TestConvertEmail" }).getAttribute("href")).toBe(
    "https://github.com/neokapi/neokapi/blob/review-commit/tools/mail_test.go",
  );
  fireEvent.click(subjects[0]);
  expect(screen.queryByRole("link")).toBeNull();
});

it("filters subjects and disclosed operations by status and text, including raw operation IDs", () => {
  render(
    <CapabilityEvidence
      initialStatus="failed"
      evidenceBaseUrl="https://github.com/neokapi/neokapi/blob/review-commit/"
    />,
  );
  expect((screen.getByLabelText("Evidence status") as HTMLSelectElement).value).toBe("failed");
  expect(screen.getByRole("status").textContent).toBe("1 of 2 output formats");
  const subject = screen.getByRole("button", { name: /Markdown/ });
  fireEvent.click(subject);
  expect(screen.getByText("The output lost the subject.")).toBeTruthy();
  expect(screen.queryByText("Retained the heading.")).toBeNull();

  fireEvent.change(screen.getByRole("searchbox"), { target: { value: "structure" } });
  expect(screen.getByText("No operations match these filters.")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
  expect(screen.getByRole("status").textContent).toBe("2 of 2 output formats");
  fireEvent.change(screen.getByRole("searchbox"), { target: { value: " PROJECT-TEXT " } });
  expect(screen.getByRole("button", { name: /DocLang.*Convert text/ })).toBeTruthy();
  expect(screen.queryByRole("button", { name: /Markdown/ })).toBeNull();
});

it("shows published observations newest first, preserving missing coverage, deltas and source links", () => {
  render(<ObservationHistory dataBaseUrl="/preview/data/" />);
  expect(screen.queryByRole("searchbox")).toBeNull();
  const dates = screen.getAllByRole("heading", { level: 3 }).slice(0, 3);
  expect(dates.map((date) => date.textContent)).toEqual(["2026-11-20", "2026-06-01", "2026-05-01"]);
  const earliest = within(dates[2].closest("li")!);
  expect(earliest.getByText("First observation")).toBeTruthy();
  expect(earliest.getAllByText("No observation")).toHaveLength(1);
  expect(earliest.getByRole("link").getAttribute("href")).toBe(
    "https://github.com/neokapi/neokapi/blob/" +
      "a".repeat(40) +
      "/web/static/data/format-maturity.json",
  );
  const second = within(dates[1].closest("li")!);
  expect(second.getByText("Markdown · L1 → L2")).toBeTruthy();
  expect(second.getByText("1 subject · 1 verified · 1 failed")).toBeTruthy();
  const latest = within(dates[0].closest("li")!);
  expect(latest.getByText("No Engine grade changes")).toBeTruthy();
  expect(latest.getByText("2 subjects · 2 verified · 0 failed · 3 changes")).toBeTruthy();
  expect(latest.getByText("Markdown / Convert email · Failed → Verified")).toBeTruthy();
  expect(
    latest.getByText("Markdown / Preserve document structure · Verified → absent"),
  ).toBeTruthy();
  expect(
    latest.getByText("DocLang / Preserve document structure · absent → Verified"),
  ).toBeTruthy();
  for (const [label, file] of [
    ["Current capability evidence (JSON)", "content-capabilities.json"],
    ["Historical observations (JSON)", "content-capabilities-history.json"],
    ["Format grade totals by axis and date (JSON)", "format-maturity-history.json"],
  ]) {
    expect(screen.getByRole("link", { name: label }).getAttribute("href")).toBe(
      "/preview/data/" + file,
    );
  }
});
