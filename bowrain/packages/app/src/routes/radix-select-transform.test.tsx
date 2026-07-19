import { describe, it, expect, vi, beforeAll } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import { useState } from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@neokapi/ui";

/**
 * Regression guard for the github/setup Select outage (#1348/#1353).
 *
 * This package's vitest runs the same @neokapi/i18n-react build
 * transform the web/desktop shells use (see vite.config.ts), so the
 * JSX below goes through the exact production i18n pipeline. The
 * transform used to recognize ICU authoring components by bare
 * identifier (`tag === "Select"`); a controlled shadcn/Radix
 * `<Select value={...}>` matched (it has a `value` pivot, zero
 * `Case`/`Other` forms), the labeled row was promoted to one
 * translation block, and the whole widget was serialized to the
 * empty ICU template `{value, select, }` — rendered as "" at
 * runtime. The trigger silently vanished from the compiled chunk on
 * app.bowrain.cloud while sibling spans/buttons kept rendering.
 *
 * These tests render the incident's exact markup shapes (including
 * the auth layout's wrapper div) and assert the trigger reaches the
 * DOM. If ICU recognition ever regresses to name-based matching,
 * the transform deletes the widget from THIS file too and the
 * queries below come back empty.
 */

// jsdom lacks these pointer APIs Radix Select relies on.
beforeAll(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.setPointerCapture ??= vi.fn();
  Element.prototype.releasePointerCapture ??= vi.fn();
  Element.prototype.scrollIntoView ??= vi.fn();
});

function WorkspaceChooserRow() {
  // Mirrors the pre-#1353 github-setup workspace chooser: a label
  // span and a controlled Select in one flex row, under the auth
  // layout's wrapper element.
  const [slug, setSlug] = useState("ada");
  return (
    <div className="relative z-10">
      <div className="mb-4 flex items-center gap-2">
        <span className="text-sm text-muted-foreground">Workspace</span>
        <Select value={slug} onValueChange={setSlug}>
          <SelectTrigger className="w-56" data-testid="regression-workspace-select">
            <SelectValue>Ada Lovelace</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ada">Ada Lovelace</SelectItem>
            <SelectItem value="acme">Acme Corp</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}

function ActionRow() {
  // Mirrors the pre-#1353 RepoRow: a controlled Select next to a
  // conditional action button — no label text at all. The old
  // transform swallowed this row too (`{projectId, select, }`).
  const [projectId, setProjectId] = useState("__new__");
  return (
    <div className="flex shrink-0 items-center gap-2">
      <Select value={projectId} onValueChange={setProjectId}>
        <SelectTrigger className="w-44" data-testid="regression-project-select">
          <SelectValue placeholder="Choose a project" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__new__">+ New project…</SelectItem>
        </SelectContent>
      </Select>
      {projectId === "__new__" ? <button type="button">Create project</button> : null}
    </div>
  );
}

describe("Radix Select survives the neokapi-i18n build transform", () => {
  it("renders the trigger of a labeled workspace chooser row", () => {
    render(<WorkspaceChooserRow />);
    const trigger = screen.getByTestId("regression-workspace-select");
    expect(trigger).toBeInTheDocument();
    expect(trigger).toHaveRole("combobox");
    expect(trigger).toHaveTextContent("Ada Lovelace");
    // The sibling label must not absorb the widget into its block.
    expect(screen.getByText("Workspace")).toBeInTheDocument();
  });

  it("renders the trigger of an unlabeled Select + action row", () => {
    render(<ActionRow />);
    const trigger = screen.getByTestId("regression-project-select");
    expect(trigger).toBeInTheDocument();
    expect(trigger).toHaveRole("combobox");
    expect(screen.getByRole("button", { name: "Create project" })).toBeInTheDocument();
  });
});
