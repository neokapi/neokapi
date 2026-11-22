// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vite-plus/test";
import Formats from "./_Formats";

vi.mock("../../../static/data/format-maturity.json", () => ({
  default: {
    generated_at: "2026-11-20",
    summary: {},
    dimension_labels: { reader: "Reader", located: "Located" },
    formats: [
      {
        id: "openxml",
        level: "L2",
        next_level: "L3",
        dimensions: { reader: "complete" },
        tier: { declared: "maintained", last_certified: "2026-01-01", gates: ["make test"] },
        blocking_gaps: ["Test edited fields."],
        levels: { engine: "L2", prose: "P0" },
        axes: {
          prose: {
            blocking_gaps: ["Locate comments."],
            rungs: { P1: { outcome: "not-met", reason: "No test" } },
          },
        },
        dims: { prose: { located: "none" } },
      },
      {
        id: "eml",
        level: "L1",
        dimensions: { reader: "complete" },
        blocking_gaps: [],
        levels: { engine: "L1" },
        tier: { declared: "available", last_certified: null },
      },
      {
        id: "legacy",
        type: "read-only",
        level: "L1",
        dimensions: { reader: "partial" },
        blocking_gaps: ["Add a fixture."],
        next_level: "L1",
        evidence: { reader: "core/formats/legacy/reader.go" },
      },
    ],
    languages: [
      {
        id: "go",
        name: "Go",
        presence: "present",
        level: "P1",
        dims: { located: "complete" },
        rungs: { P1: { outcome: "met", reason: "Passed locator test." } },
        provider: "go-reader",
      },
    ],
  },
}));

afterEach(cleanup);

it("sorts by registry name, searches names and IDs, and combines the tier filter", () => {
  render(<Formats />);
  const table = within(screen.getByRole("table"));
  expect(table.getAllByRole("button").map((b) => b.textContent)).toEqual([
    "Email message (EML)",
    "legacy",
    "OpenXML (Office)",
  ]);
  expect(screen.getByRole("status").textContent).toBe("3 of 3 formats");
  fireEvent.change(screen.getByLabelText("Find a format"), { target: { value: " OFFICE " } });
  expect(table.getAllByRole("button")).toHaveLength(1);
  fireEvent.change(screen.getByLabelText("Find a format"), { target: { value: "openxml" } });
  expect(table.getByText("OpenXML (Office)")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Support tier"), { target: { value: "available" } });
  expect(screen.getByText("No formats match these filters.")).toBeTruthy();
});

it("opens a format from the keyboard and exposes certification and axis-specific evidence", async () => {
  render(<Formats initialSearch="Office" />);
  const button = screen.getByRole("button", { name: "OpenXML (Office)" });
  expect(button.getAttribute("aria-expanded")).toBe("false");
  expect(screen.queryByText("Test edited fields.")).toBeNull();
  button.focus();
  await userEvent.keyboard("{Enter}");
  expect(button.getAttribute("aria-expanded")).toBe("true");
  expect(document.getElementById(button.getAttribute("aria-controls")!)).toBeTruthy();
  expect(screen.getByText("Certification expired. The effective tier is Available.")).toBeTruthy();
  expect(screen.getByText("make test")).toBeTruthy();
  expect(screen.getByText("Test edited fields.").closest("details")?.open).toBe(true);
  fireEvent.change(screen.getByLabelText("Assessment axis"), { target: { value: "prose" } });
  expect(screen.getByText("Locate comments.").closest("details")?.open).toBe(true);
  expect(screen.getByText("No test")).toBeTruthy();
  expect(screen.getByText("Test edited fields.").closest("details")?.open).toBe(false);
  fireEvent.click(button);
  expect(screen.queryByRole("heading", { name: "Certification" })).toBeNull();
});

it("keeps legacy engine evidence and distinguishes an unrecorded axis", () => {
  render(<Formats initialSearch="legacy" />);
  expect(screen.getByText("Readable")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "legacy" }));
  expect(screen.getByText("Partial")).toBeTruthy();
  expect(screen.getByText("core/formats/legacy/reader.go")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Assessment axis"), { target: { value: "prose" } });
  expect(
    within(screen.getByRole("row", { name: /legacy Not recorded/ })).getAllByText("Not recorded"),
  ).toHaveLength(2);
});

it("shows an uncertified format and keeps the language probe in a disclosure", () => {
  render(<Formats initialSearch="eml" />);
  fireEvent.click(screen.getByRole("button", { name: "Email message (EML)" }));
  expect(screen.getByText("Never certified")).toBeTruthy();
  const languages = screen.getByText("Source languages (1)").closest("details")!;
  expect(languages.open).toBe(false);
  fireEvent.click(screen.getByText("Source languages (1)"));
  fireEvent.click(within(languages).getByText("Go"));
  expect(within(languages).getByText("Passed locator test.").closest("details")?.open).toBe(true);
});

it("opens a support tier selected on the overview", () => {
  render(<Formats initialTier="maintained" />);
  expect(screen.getByRole("combobox", { name: "Support tier" })).toHaveProperty(
    "value",
    "maintained",
  );
  expect(screen.getByRole("button", { name: "OpenXML (Office)" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Email message (EML)" })).toBeNull();
});
