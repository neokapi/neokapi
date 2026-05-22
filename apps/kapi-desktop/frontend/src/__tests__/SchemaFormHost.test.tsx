import { render, screen, waitFor, renderHook } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { SchemaForm, type ComponentSchema, type SchemaFormHost } from "@neokapi/ui-primitives";

// Mock the Wails-bridge API so the host hook resolves deterministically without
// a Wails runtime. Must be declared before importing the hook under test.
const browsePathMock = vi.fn();
const listProvidersMock = vi.fn();
vi.mock("../hooks/useApi", () => ({
  api: {
    browsePath: (...args: unknown[]) => browsePathMock(...args),
    listProviders: (...args: unknown[]) => listProvidersMock(...args),
  },
}));

import { useSchemaFormHost } from "../hooks/useSchemaFormHost";

const schema: ComponentSchema = {
  title: "Host Widgets",
  type: "object",
  properties: {
    inputFile: {
      type: "string",
      title: "Input File",
      "ui:widget": "path",
      "x-path": { browseTitle: "Choose a document", accepts: ["html"] },
    },
    outputDir: {
      type: "string",
      title: "Output Folder",
      "ui:widget": "folder-picker",
    },
    provider: {
      type: "string",
      title: "AI Provider",
      "ui:widget": "credential-picker",
      "x-path": { resourceKind: "anthropic" },
    },
  },
};

function HostForm({ host }: { host?: SchemaFormHost }) {
  return <SchemaForm schema={schema} values={{}} onChange={() => {}} host={host} />;
}

describe("SchemaForm host widgets", () => {
  it("renders a Browse button and calls onBrowse when a host is provided", async () => {
    const onBrowse = vi.fn().mockResolvedValue("/picked/file.html");
    render(<HostForm host={{ onBrowse }} />);

    const browseButtons = screen.getAllByRole("button", { name: "Browse" });
    expect(browseButtons.length).toBeGreaterThanOrEqual(1);

    await userEvent.click(browseButtons[0]);
    await waitFor(() => expect(onBrowse).toHaveBeenCalledTimes(1));
    expect(onBrowse).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "file", field: "inputFile" }),
    );
  });

  it("sends kind=directory for folder-picker fields", async () => {
    const onBrowse = vi.fn().mockResolvedValue("/picked/dir");
    render(<HostForm host={{ onBrowse }} />);

    const browseButtons = screen.getAllByRole("button", { name: "Browse" });
    // inputFile (file) + outputDir (directory) = 2 browse buttons.
    expect(browseButtons).toHaveLength(2);
    await userEvent.click(browseButtons[1]);
    await waitFor(() => expect(onBrowse).toHaveBeenCalled());
    expect(onBrowse).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "directory", field: "outputDir" }),
    );
  });

  it("lists host credentials in the credential picker", () => {
    const credentials = vi
      .fn()
      .mockReturnValue([{ value: "anthropic-prod", label: "Anthropic (claude)" }]);
    render(<HostForm host={{ credentials }} />);
    expect(credentials).toHaveBeenCalledWith("anthropic");
    expect(screen.getByRole("option", { name: "Anthropic (claude)" })).toBeInTheDocument();
  });

  it("degrades to text inputs when no host is provided", () => {
    render(<HostForm host={undefined} />);
    expect(screen.queryByRole("button", { name: "Browse" })).not.toBeInTheDocument();
  });
});

describe("useSchemaFormHost (Kapi Desktop wiring)", () => {
  beforeEach(() => {
    browsePathMock.mockReset();
    listProvidersMock.mockReset();
  });

  it("maps a browse request onto api.browsePath and returns the picked path", async () => {
    listProvidersMock.mockResolvedValue([]);
    browsePathMock.mockResolvedValue("/Users/me/doc.html");

    const { result } = renderHook(() => useSchemaFormHost());
    const picked = await result.current.onBrowse!({ kind: "file", field: "inputFile" });

    expect(browsePathMock).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "file", field: "inputFile" }),
    );
    expect(picked).toBe("/Users/me/doc.html");
  });

  it("returns null when the user cancels (empty string) or runs outside Wails (null)", async () => {
    listProvidersMock.mockResolvedValue([]);
    browsePathMock.mockResolvedValueOnce("").mockResolvedValueOnce(null);

    const { result } = renderHook(() => useSchemaFormHost());
    expect(await result.current.onBrowse!({ kind: "file", field: "f" })).toBeNull();
    expect(await result.current.onBrowse!({ kind: "file", field: "f" })).toBeNull();
  });

  it("surfaces the provider vault as credentials, scoped by resourceKind", async () => {
    listProvidersMock.mockResolvedValue([
      { id: "1", name: "MyAnthropic", provider_type: "anthropic", model: "claude-sonnet" },
      { id: "2", name: "MyOpenAI", provider_type: "openai" },
    ]);
    browsePathMock.mockResolvedValue("");

    const { result } = renderHook(() => useSchemaFormHost());
    await waitFor(() => expect(listProvidersMock).toHaveBeenCalled());
    // Wait for the prefetched provider list to land in state.
    await waitFor(() => expect(result.current.credentials!(undefined)).toHaveLength(2));

    const anthropicOnly = result.current.credentials!("anthropic");
    expect(anthropicOnly).toEqual([{ value: "MyAnthropic", label: "MyAnthropic (claude-sonnet)" }]);
  });
});
