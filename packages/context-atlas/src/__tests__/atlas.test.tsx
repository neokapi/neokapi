// @vitest-environment jsdom
import { describe, expect, it, vi } from "vite-plus/test";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ContextAtlas } from "../ContextAtlas";
import { DERIVED_AXES, defaultQuery, parseQueryHash, prepareAtlas, serialiseQuery } from "../model";
import { makeAtlasFixture, makeEmptyAtlas, makeOneRecordAtlas } from "../stories/fixtures";

const data = makeAtlasFixture();
const atlas = prepareAtlas(data);

function counts() {
  return within(screen.getByTestId("context-atlas"))
    .getAllByRole("listitem")
    .filter((li) => li.getAttribute("data-slot") === "stat")
    .map((li) => li.textContent);
}

describe("ContextAtlas", () => {
  it("renders the time brush, the counts and the flow", () => {
    render(<ContextAtlas data={data} commandHint="kapi context atlas" />);
    expect(screen.getByTestId("atlas-time-brush")).toBeInTheDocument();
    expect(screen.getByTestId("atlas-flow")).toBeInTheDocument();
    expect(screen.getByText("kapi context atlas")).toBeInTheDocument();
    expect(counts().length).toBeGreaterThanOrEqual(4);
    expect(screen.getByTestId("flow-node-family-rule")).toBeInTheDocument();
  });

  it("previews on hover without changing the query", () => {
    const onQueryChange = vi.fn();
    render(<ContextAtlas data={data} onQueryChange={onQueryChange} />);
    const before = counts();
    const node = screen.getByTestId("flow-node-family-rule");
    fireEvent.mouseEnter(node);
    fireEvent.focus(node);
    expect(onQueryChange).not.toHaveBeenCalled();
    expect(counts()).toEqual(before);
    expect(
      within(screen.getByTestId("atlas-scope")).getByText("Everything recorded by the snapshot"),
    ).toBeInTheDocument();
    fireEvent.mouseLeave(node);
    expect(onQueryChange).not.toHaveBeenCalled();
  });

  it("retains a value on click, and adds to it on shift-click", () => {
    const onQueryChange = vi.fn();
    render(<ContextAtlas data={data} onQueryChange={onQueryChange} />);
    fireEvent.click(screen.getByTestId("flow-node-family-rule"));
    expect(onQueryChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ filters: { family: ["rule"] } }),
    );
    expect(screen.getByTestId("flow-node-family-rule")).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(screen.getByTestId("flow-node-family-note"), { shiftKey: true });
    expect(onQueryChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ filters: { family: ["rule", "note"] } }),
    );
  });

  it("retains a value with Enter on a focused node", async () => {
    const user = userEvent.setup();
    const onQueryChange = vi.fn();
    render(<ContextAtlas data={data} onQueryChange={onQueryChange} />);
    const node = screen.getByTestId("flow-node-family-term");
    node.focus();
    await user.keyboard("{Enter}");
    expect(onQueryChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ filters: { family: ["term"] } }),
    );
  });

  it("searches the table only", async () => {
    const user = userEvent.setup();
    const onQueryChange = vi.fn();
    render(
      <ContextAtlas
        data={data}
        initialQuery={{ ...defaultQuery(atlas), view: "records" }}
        onQueryChange={onQueryChange}
      />,
    );
    const table = screen.getByTestId("atlas-records");
    const before = counts();
    const rowsBefore = within(table).getAllByRole("row").length;
    const box = within(table).getByRole("searchbox", { name: "Search this table only" });
    await user.type(box, "travel pass");
    const rowsAfter = within(table).getAllByRole("row").length;
    expect(rowsAfter).toBeLessThan(rowsBefore);
    expect(rowsAfter).toBeGreaterThan(1);
    expect(counts()).toEqual(before);
    expect(onQueryChange).not.toHaveBeenCalled();
    expect(within(table).getByText(/search narrows this table only/)).toBeInTheDocument();
  });

  it("opens a record in the detail pane with its CLI chip", () => {
    const rule = data.records.find((r) => r.family === "rule" && r.short)!;
    render(
      <ContextAtlas
        data={data}
        initialQuery={{ ...defaultQuery(atlas), from: atlas.firstDay, view: "records" }}
      />,
    );
    fireEvent.click(screen.getAllByRole("button", { name: rule.label })[0]);
    // Labels repeat across rules; whichever row opened, its own short id is on the chip.
    const pane = screen.getByTestId("atlas-inspector");
    expect(within(pane).getByRole("heading", { name: rule.label })).toBeInTheDocument();
    expect(within(pane).getByText(/^kapi context log [0-9a-f]{6}$/u)).toBeInTheDocument();
  });

  it("restores a query from a link", () => {
    const q = { ...defaultQuery(atlas), filters: { [DERIVED_AXES.statusSnapshot]: ["contested"] } };
    render(<ContextAtlas data={data} initialQuery={parseQueryHash(`#${serialiseQuery(q)}`)} />);
    expect(within(screen.getByTestId("atlas-scope")).getByText("Contested")).toBeInTheDocument();
  });

  it("shows an empty state for an empty atlas", () => {
    render(<ContextAtlas data={makeEmptyAtlas()} />);
    expect(screen.getByText("No context recorded yet")).toBeInTheDocument();
    expect(screen.getByText("kapi context note")).toBeInTheDocument();
  });

  it("renders one record", () => {
    render(<ContextAtlas data={makeOneRecordAtlas()} />);
    expect(screen.getByTestId("atlas-flow")).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Map" })).not.toBeInTheDocument();
  });

  it("links evidence only to http(s) pages", () => {
    const one = makeOneRecordAtlas();
    one.records[0].evidence = [
      { url: "https://example.com/help/refunds", quote: "a page" },
      { url: "javascript:alert(1)", quote: "a script" },
    ];
    render(<ContextAtlas data={one} initialRecord={one.records[0].id} />);
    const links = within(screen.getByTestId("atlas-inspector")).getAllByRole("link");
    expect(links).toHaveLength(1);
    expect(links[0]).toHaveAttribute("href", "https://example.com/help/refunds");
  });

  it("offers the map only where geography is placed", () => {
    render(<ContextAtlas data={data} />);
    expect(screen.getByRole("tab", { name: "Map" })).toBeInTheDocument();
  });
});
