// @vitest-environment jsdom
import { useState } from "react";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import FilePreview from "../components/preview/FilePreview";
import type { ContentTree } from "../components/preview/types";

// jsdom implements neither of these; the sheet's focus scroll and Radix's
// dismissable layer both reach for them.
beforeAll(() => {
  Element.prototype.scrollIntoView ??= () => {};
  window.HTMLElement.prototype.hasPointerCapture ??= () => false;
});

afterEach(cleanup);

const tree: ContentTree = {
  format: "json",
  root: [
    {
      kind: "block",
      id: "b1",
      name: "greeting",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Please utilize the dashboard" }],
      targets: { fr: [{ text: "Veuillez utiliser le tableau de bord" }] },
    },
    {
      kind: "block",
      id: "b2",
      name: "tagline",
      type: "text",
      translatable: true,
      sourceLocale: "en",
      source: [{ text: "Ship faster" }],
    },
  ],
  stats: { layers: 0, groups: 0, blocks: 2, data: 0, media: 0, runs: 2 },
};

const base = {
  onClose: () => {},
  filename: "locales/en.json",
  description: "Read the document.",
};

describe("FilePreview — the shell", () => {
  it("renders nothing until it is opened", () => {
    render(<FilePreview {...base} open={false} tree={tree} />);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("titles the sheet by the file, badges the format, and names the item underneath", () => {
    render(
      <FilePreview
        {...base}
        open
        format="json"
        subtitle="locales/en.kbf.json"
        subtitleTestId="item-name"
        tree={tree}
      />,
    );
    const sheet = screen.getByRole("dialog");
    // The viewer draws the file name too, so the header is read on its own.
    const title = sheet.querySelector('[data-slot="sheet-title"]') as HTMLElement;
    expect(within(title).getByText("locales/en.json")).toBeTruthy();
    expect(within(title).getByText("json")).toBeTruthy();
    expect(screen.getByTestId("item-name").textContent).toBe("locales/en.kbf.json");
    expect(within(sheet).getByText("Read the document.")).toBeTruthy();
  });

  it("closes on Escape", async () => {
    const onClose = vi.fn();
    render(<FilePreview {...base} open onClose={onClose} tree={tree} />);
    await userEvent.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
  });

  it("shows the loading line, and the host's own wording when it supplies one", () => {
    const { rerender } = render(<FilePreview {...base} open loading tree={tree} />);
    expect(screen.getByText(/Reading locales\/en\.json/)).toBeTruthy();
    expect(screen.queryByRole("tab", { name: /preview/i })).toBeNull();

    rerender(<FilePreview {...base} open loading loadingLabel="Inspecting it…" tree={tree} />);
    expect(screen.getByText("Inspecting it…")).toBeTruthy();
  });

  it("shows the error instead of the document", () => {
    render(<FilePreview {...base} open error="No reader for this file." tree={tree} />);
    expect(screen.getByText("No reader for this file.")).toBeTruthy();
    expect(screen.queryByRole("tab", { name: /preview/i })).toBeNull();
  });

  it("draws the empty slot when there is no body, and never over a body", () => {
    const { rerender } = render(<FilePreview {...base} open empty={<p>Nothing to read.</p>} />);
    expect(screen.getByText("Nothing to read.")).toBeTruthy();

    rerender(<FilePreview {...base} open empty={<p>Nothing to read.</p>} tree={tree} />);
    expect(screen.queryByText("Nothing to read.")).toBeNull();
  });

  it("renders the document through the viewer when given a tree", () => {
    render(<FilePreview {...base} open tree={tree} />);
    expect(screen.getByRole("tab", { name: /preview/i })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /blocks/i })).toBeTruthy();
    expect(screen.getByText(/Please utilize/)).toBeTruthy();
  });

  it("renders a host's own body instead of a tree", () => {
    render(
      <FilePreview {...base} open tree={tree}>
        <p>The host drew this.</p>
      </FilePreview>,
    );
    expect(screen.getByText("The host drew this.")).toBeTruthy();
    expect(screen.queryByRole("tab", { name: /preview/i })).toBeNull();
  });

  it("offers the actions the host wired, in a footer", () => {
    render(
      <FilePreview {...base} open tree={tree} actions={<button type="button">Review</button>} />,
    );
    expect(screen.getByRole("button", { name: "Review" })).toBeTruthy();
  });
});

describe("FilePreview — the reading strip", () => {
  it("offers no strip when there is nothing to choose", () => {
    render(<FilePreview {...base} open tree={tree} />);
    expect(screen.queryByRole("group")).toBeNull();
  });

  it("names the target side and reports each choice", async () => {
    const onChange = vi.fn();
    render(
      <FilePreview
        {...base}
        open
        tree={tree}
        sides={{
          value: "source",
          onChange,
          targetLabel: "French",
          "data-testid": "side",
        }}
      />,
    );
    const group = screen.getByTestId("side");
    expect(within(group).getByRole("button", { name: "Source" }).getAttribute("aria-pressed")).toBe(
      "true",
    );
    await userEvent.click(within(group).getByRole("button", { name: "French" }));
    expect(onChange).toHaveBeenCalledWith("target");
  });

  it("falls back to Target when the host names no language", () => {
    render(
      <FilePreview {...base} open tree={tree} sides={{ value: "target", onChange: vi.fn() }} />,
    );
    expect(screen.getByRole("button", { name: "Target" }).getAttribute("aria-pressed")).toBe(
      "true",
    );
  });

  it("shows no tabs for a single reading, and still draws it", () => {
    render(
      <FilePreview
        {...base}
        open
        views={[{ value: "document", label: "Document", content: <p>The document.</p> }]}
        viewsTestId="reading"
      />,
    );
    expect(screen.queryByTestId("reading")).toBeNull();
    expect(screen.getByText("The document.")).toBeTruthy();
  });

  it("switches the body when a second reading is picked", async () => {
    function Host() {
      const [view, setView] = useState("document");
      return (
        <FilePreview
          {...base}
          open
          view={view}
          onViewChange={setView}
          viewsLabel="Reading"
          viewsTestId="reading"
          views={[
            { value: "document", label: "Document", content: <p>The document.</p> },
            { value: "context", label: "In context", content: <p>The component.</p> },
          ]}
        />
      );
    }
    render(<Host />);
    expect(screen.getByText("The document.")).toBeTruthy();

    await userEvent.click(screen.getByRole("button", { name: "In context" }));
    expect(screen.getByText("The component.")).toBeTruthy();
    expect(screen.queryByText("The document.")).toBeNull();
  });

  it("falls back to the first reading when the active one is gone", () => {
    render(
      <FilePreview
        {...base}
        open
        view="context"
        views={[{ value: "document", label: "Document", content: <p>The document.</p> }]}
      />,
    );
    expect(screen.getByText("The document.")).toBeTruthy();
  });

  it("carries the host's own controls on the same strip", () => {
    render(<FilePreview {...base} open tree={tree} toolbar={<span>fr-FR</span>} />);
    expect(screen.getByText("fr-FR")).toBeTruthy();
  });
});

describe("FilePreview — opening at one unit", () => {
  const unitStates = { greeting: "approved", tagline: "needs work" };

  it("names the focused unit and its state, and marks its block", () => {
    render(<FilePreview {...base} open tree={tree} focusKey="greeting" unitStates={unitStates} />);
    const row = document.querySelector('[data-slot="file-preview-focus"]');
    expect(row).toBeTruthy();
    expect(within(row as HTMLElement).getByText("greeting")).toBeTruthy();
    expect(within(row as HTMLElement).getByText("approved")).toBeTruthy();
    expect(document.querySelector('[data-review-focus="true"]')).toBeTruthy();
  });

  it("marks a unit with a state but no focus, and says so when the key is unknown", () => {
    render(<FilePreview {...base} open tree={tree} focusKey="missing" unitStates={unitStates} />);
    expect(screen.getByText("This unit is not in the rendered document.")).toBeTruthy();
    expect(screen.getByText("awaiting review")).toBeTruthy();
    expect(document.querySelector('[data-review-state="needs work"]')).toBeTruthy();
    expect(document.querySelector('[data-review-focus="true"]')).toBeNull();
  });

  it("takes the reader back where they came from", async () => {
    const onBack = vi.fn();
    const onClose = vi.fn();
    render(
      <FilePreview
        {...base}
        open
        onClose={onClose}
        tree={tree}
        focusKey="greeting"
        backLabel="Back to review"
        onBack={onBack}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /Back to review/ }));
    expect(onBack).toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("closes when the back button has nowhere else to go", async () => {
    const onClose = vi.fn();
    render(
      <FilePreview
        {...base}
        open
        onClose={onClose}
        tree={tree}
        focusKey="greeting"
        backLabel="Back"
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /^Back$/ }));
    expect(onClose).toHaveBeenCalled();
  });
});
