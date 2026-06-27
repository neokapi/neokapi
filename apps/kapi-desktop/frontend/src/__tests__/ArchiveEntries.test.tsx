import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ArchiveEntries, isArchivePath } from "../components/ArchiveEntries";

const entries = [
  { name: "locales/en.json", format: "json", size: 42 },
  { name: "README.md", format: "markdown", size: 120 },
  { name: "logo.png", format: "", size: 2048 }, // no reader → disabled
];

describe("isArchivePath", () => {
  it("recognises archive extensions", () => {
    expect(isArchivePath("a.zip")).toBe(true);
    expect(isArchivePath("deep/b.tar.gz")).toBe(true);
    expect(isArchivePath("c.tgz")).toBe(true);
    expect(isArchivePath("d.TAR")).toBe(true);
    expect(isArchivePath("e.json")).toBe(false);
    expect(isArchivePath("f.docx")).toBe(false);
  });
});

describe("ArchiveEntries", () => {
  it("lists preset entries and selects a recognised one", async () => {
    const onSelect = vi.fn();
    render(<ArchiveEntries archivePath="/abs/bundle.zip" onSelect={onSelect} entries={entries} />);

    expect(screen.getByText("locales/en.json")).toBeInTheDocument();
    expect(screen.getByText("README.md")).toBeInTheDocument();
    expect(screen.getByText("logo.png")).toBeInTheDocument();

    await userEvent.click(screen.getByText("locales/en.json"));
    expect(onSelect).toHaveBeenCalledWith("locales/en.json");
  });

  it("disables entries kapi has no reader for", async () => {
    const onSelect = vi.fn();
    render(<ArchiveEntries archivePath="/abs/bundle.zip" onSelect={onSelect} entries={entries} />);

    const png = screen.getByText("logo.png").closest("button");
    expect(png).toBeDisabled();
    if (png) await userEvent.click(png);
    expect(onSelect).not.toHaveBeenCalled();
  });
});
