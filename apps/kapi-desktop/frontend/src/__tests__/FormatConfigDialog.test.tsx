import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock the Wails-bridge API: a null schema keeps the right pane on the
// "no configurable options" message so the test exercises the format picker
// without standing up SchemaForm + its host.
const getFormatSchema = vi.fn();
const listFormatPresets = vi.fn();
vi.mock("../hooks/useApi", () => ({
  api: {
    getFormatSchema: (...a: unknown[]) => getFormatSchema(...a),
    listFormatPresets: (...a: unknown[]) => listFormatPresets(...a),
    // useSchemaFormHost pulls these on mount; stub them out.
    listProviders: () => Promise.resolve([]),
    browsePath: () => Promise.resolve(""),
  },
}));

import { FormatConfigDialog } from "../components/FormatConfigDialog";
import { ErrorProvider } from "../components/ErrorBanner";
import type { FormatInfo } from "../types/api";

const allFormats: FormatInfo[] = [
  {
    name: "okf_vtt",
    extensions: [".vtt", ".srt"],
    has_reader: true,
    has_writer: true,
    has_schema: true,
  },
  { name: "okf_regex", extensions: [".srt"], has_reader: true, has_writer: true, has_schema: true },
  { name: "okf_json", extensions: [".json"], has_reader: true, has_writer: true, has_schema: true },
];

function renderDialog(props: Partial<React.ComponentProps<typeof FormatConfigDialog>> = {}) {
  return render(
    <ErrorProvider>
      <FormatConfigDialog
        open
        onOpenChange={() => {}}
        title="Configure formats"
        formats={["okf_vtt"]}
        allFormats={allFormats}
        values={{}}
        onChange={() => {}}
        {...props}
      />
    </ErrorProvider>,
  );
}

describe("FormatConfigDialog", () => {
  beforeEach(() => {
    getFormatSchema.mockReset().mockResolvedValue(null);
    listFormatPresets.mockReset().mockResolvedValue([]);
  });

  it("wildcard mode shows the matched formats and an add control", async () => {
    renderDialog({ allowAdd: true, formats: ["okf_vtt"] });
    // Left picker lists the matched format.
    expect(await screen.findByRole("button", { name: "okf_vtt" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add format/i })).toBeInTheDocument();
  });

  it("reveals the add picker when adding a format", async () => {
    renderDialog({ allowAdd: true, formats: [], filterExtension: ".srt" });
    await userEvent.click(screen.getByRole("button", { name: /add format/i }));
    // The searchable format picker (cmdk combobox) appears.
    await waitFor(() => expect(screen.getByText(/pick a format/i)).toBeInTheDocument());
  });

  it("single-format mode shows no picker", async () => {
    renderDialog({ formats: ["okf_json"] });
    // Right pane resolves to the no-options message; no left picker header.
    await screen.findByText(/no configurable options/i);
    expect(screen.queryByText("Formats")).not.toBeInTheDocument();
  });

  // A property whose value differs from its schema default (the baseline) is
  // flagged modified (border-primary accent), so the user sees each dirty field.
  const boolSchema = {
    title: "Test Format",
    type: "object",
    properties: {
      translateCodeBlocks: {
        type: "boolean",
        title: "Translate Code Blocks",
        default: false,
      },
    },
  };

  it("flags an overridden property as modified", async () => {
    getFormatSchema.mockResolvedValue(boolSchema);
    renderDialog({
      formats: ["okf_md"],
      values: { okf_md: { config: { translateCodeBlocks: true } } },
    });
    await screen.findByText("Translate Code Blocks");
    // The field carries the modified accent (Sheet portals to document.body).
    expect(document.querySelector(".border-primary")).toBeInTheDocument();
  });

  it("does not flag a property left at its default", async () => {
    getFormatSchema.mockResolvedValue(boolSchema);
    renderDialog({ formats: ["okf_md"], values: {} });
    await screen.findByText("Translate Code Blocks");
    expect(document.querySelector(".border-primary")).not.toBeInTheDocument();
  });
});
