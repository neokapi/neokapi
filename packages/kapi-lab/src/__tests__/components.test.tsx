// @vitest-environment jsdom
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import FileExplorer from "../FileExplorer";
import { BlockInspector } from "@neokapi/ui-primitives/preview";
import { useFileLibrary } from "../fileLibrary";
import type { FileSelection } from "../fileLibrary";
import { richBlock } from "../stories/mockData";

function ExplorerHost() {
  const ids = ["messages-json", "app-xliff"];
  const library = useFileLibrary({ sampleIds: ids });
  const [sel, setSel] = useState<FileSelection>({ mode: "single", paths: ["messages.json"] });
  return (
    <>
      <FileExplorer
        library={library}
        selection={sel}
        onSelectionChange={setSel}
        multiple={false}
        sampleIds={ids}
      />
      <span data-testid="sel">{`path:${sel.paths.join(",")}`}</span>
    </>
  );
}

describe("FileExplorer", () => {
  it("lists library files and selects on click", () => {
    render(<ExplorerHost />);
    // Rows render the filename via FileLabel (base + dimmed extension), so query
    // by the row's title attribute (the full path) rather than by split text.
    expect(screen.getByTitle("messages.json")).toBeTruthy();
    expect(screen.getByTitle("app.xliff")).toBeTruthy();
    fireEvent.click(screen.getByTitle("app.xliff"));
    expect(screen.getByTestId("sel").textContent).toBe("path:app.xliff");
  });
});

describe("BlockInspector", () => {
  it("renders source, target provenance, overlays and annotations", () => {
    render(<BlockInspector node={richBlock} defaultOpen />);
    expect(screen.getByText("greeting")).toBeTruthy();
    // target variant + lifecycle ("fr-FR" also appears as the QA overlay side)
    expect(screen.getAllByText("fr-FR").length).toBeGreaterThan(0);
    expect(screen.getByText("reviewed")).toBeTruthy();
    // overlay + annotation section labels
    expect(screen.getByText("overlays")).toBeTruthy();
    expect(screen.getByText("annotations")).toBeTruthy();
    expect(screen.getByText("term")).toBeTruthy();
  });
});
