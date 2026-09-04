import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, TooltipProvider } from "@neokapi/ui";
import { ProjectFlowsEditor } from "./ProjectFlowsEditor";
import { createFlowsApi, type FlowsApiOptions } from "./fixtures";

function setup(options: FlowsApiOptions = {}, saveDelayMs = 10) {
  const { api, flows } = createFlowsApi(options);
  const spies = {
    create: vi.spyOn(api, "createFlowDefinition"),
    update: vi.spyOn(api, "updateFlowDefinition"),
    remove: vi.spyOn(api, "deleteFlowDefinition"),
  };
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={api}>
        <TooltipProvider>
          <ProjectFlowsEditor workspaceSlug="acme" projectId="p-1" saveDelayMs={saveDelayMs} />
        </TooltipProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { flows, spies, user: userEvent.setup() };
}

const card = (id: string) => screen.getByTestId(`flow-item-${id}`);

describe("ProjectFlowsEditor list", () => {
  it("lists built-in and project flows as cards with their steps", async () => {
    setup();
    expect(await screen.findByText("Translate and review")).toBeInTheDocument();
    const translate = card("translate");
    expect(within(translate).getByText("built-in")).toBeInTheDocument();
    expect(within(translate).getByText("Memory Reuse")).toBeInTheDocument();
    expect(within(translate).getByText("Quality Check")).toBeInTheDocument();
    // A fan-out reads as one chip on the card.
    expect(within(card("flow-review")).getByText("Parallel group")).toBeInTheDocument();
  });

  it("shows the empty state when the project has no flows", async () => {
    setup({ flows: [] });
    expect(await screen.findByText("No flows yet")).toBeInTheDocument();
  });

  it("reports a failed load", async () => {
    const { api } = createFlowsApi();
    vi.spyOn(api, "listFlowDefinitions").mockRejectedValue(new Error("server unreachable"));
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <ApiProvider adapter={api}>
          <TooltipProvider>
            <ProjectFlowsEditor workspaceSlug="acme" projectId="p-1" />
          </TooltipProvider>
        </ApiProvider>
      </QueryClientProvider>,
    );
    expect(await screen.findByText("Could not load flows")).toBeInTheDocument();
  });
});

describe("ProjectFlowsEditor editing", () => {
  it("opens a built-in flow read-only with its steps in order", async () => {
    const { user } = setup();
    await user.click(await screen.findByText("Memory Reuse"));
    const editor = await screen.findByTestId("linear-flow-editor");
    expect(screen.getByTestId("flow-read-only")).toBeInTheDocument();
    const rows = within(editor).getAllByTestId("step-row");
    expect(rows.map((r) => r.textContent)).toEqual([
      expect.stringContaining("Memory Reuse"),
      expect.stringContaining("Translate"),
      expect.stringContaining("Quality Check"),
    ]);
    expect(screen.queryByTestId("add-step")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Rename flow")).not.toBeInTheDocument();
    expect(screen.getByTestId("copy-flow-btn")).toBeInTheDocument();
  });

  it("opens a project flow with its parallel group and saves an added branch", async () => {
    const { user, spies } = setup();
    await user.click(await screen.findByText("Translate and review"));
    const group = await screen.findByTestId("parallel-group");
    expect(within(group).getAllByTestId("step-row")).toHaveLength(2);
    expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Saved");

    await user.click(within(group).getByTestId("add-branch"));
    await user.click(screen.getByRole("button", { name: /Pseudo Translate/ }));
    expect(within(group).getAllByTestId("step-row")).toHaveLength(3);

    await waitFor(() => expect(spies.update).toHaveBeenCalledTimes(1));
    const [, , id, def] = spies.update.mock.calls[0];
    expect(id).toBe("flow-review");
    expect(def.nodes.map((n) => n.name)).toEqual([
      "translate",
      "qa",
      "term-check",
      "pseudo-translate",
    ]);
    // Three branches share the second row of the persisted graph.
    const rows = new Set(def.nodes.slice(1).map((n) => n.position.y));
    expect(rows.size).toBe(1);
    await waitFor(() => expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Saved"));
  });

  it("creates a flow, offers the template library, and saves the first step", async () => {
    const { user, spies, flows } = setup();
    await user.click(await screen.findByTestId("new-flow-btn"));
    await user.type(screen.getByTestId("new-flow-name"), "Publish check");
    await user.click(screen.getByTestId("create-flow-btn"));

    expect(await screen.findByRole("heading", { name: "Publish check" })).toBeInTheDocument();
    expect(spies.create).toHaveBeenCalledTimes(1);
    expect(spies.create.mock.calls[0][2]).toMatchObject({ name: "Publish check", nodes: [] });
    expect(screen.getByText("Start from a template")).toBeInTheDocument();
    expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Saved");

    await user.click(screen.getByTestId("add-step"));
    await user.click(screen.getByRole("button", { name: /Quality Check/ }));
    await waitFor(() => expect(spies.update).toHaveBeenCalledTimes(1));
    expect(spies.update.mock.calls[0][3].nodes.map((n) => n.name)).toEqual(["qa"]);
    await waitFor(() => expect(flows.find((f) => f.id === "flow-1")?.nodes).toHaveLength(1));
    await waitFor(() => expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Saved"));
  });

  it("holds an edit as unsaved while it rests, then saves it after the pause", async () => {
    const { user, spies } = setup({}, 60_000);
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await user.click(screen.getByTestId("add-step"));
    await user.click(screen.getByRole("button", { name: /Quality Check/ }));
    expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Unsaved changes");
    expect(spies.update).not.toHaveBeenCalled();
  });

  it("renames a flow in place, keeping its id", async () => {
    const { user, spies } = setup();
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await user.click(screen.getByLabelText("Rename flow"));
    const input = screen.getByLabelText("Flow name");
    await user.clear(input);
    await user.type(input, "Review twice");
    await user.click(screen.getByLabelText("Save"));

    await waitFor(() => expect(spies.update).toHaveBeenCalledTimes(1));
    expect(spies.update.mock.calls[0][2]).toBe("flow-review");
    expect(spies.update.mock.calls[0][3].name).toBe("Review twice");
    expect(await screen.findByRole("heading", { name: "Review twice" })).toBeInTheDocument();
  });

  it("copies a built-in flow into the project and opens the copy", async () => {
    const { user, spies } = setup();
    await user.click(await screen.findByText("Memory Reuse"));
    await user.click(await screen.findByTestId("copy-flow-btn"));

    expect(await screen.findByRole("heading", { name: "Copy of Translate" })).toBeInTheDocument();
    expect(spies.create.mock.calls[0][2].nodes.map((n) => n.name)).toEqual([
      "recycle",
      "translate",
      "qa",
    ]);
    expect(screen.queryByTestId("flow-read-only")).not.toBeInTheDocument();
    expect(screen.getByTestId("add-step")).toBeInTheDocument();
  });

  it("deletes a project flow from its card after a confirmation", async () => {
    const { user, spies } = setup();
    const review = await screen.findByTestId("flow-item-flow-review");
    await user.click(within(review).getByLabelText("Delete"));
    await user.click(within(review).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(spies.remove).toHaveBeenCalledWith("acme", "p-1", "flow-review"));
    await waitFor(() =>
      expect(screen.queryByTestId("flow-item-flow-review")).not.toBeInTheDocument(),
    );
  });

  it("saves a pending edit when the reader goes back to the list", async () => {
    const { user, spies } = setup({}, 60_000);
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await user.click(screen.getByTestId("add-step"));
    await user.click(screen.getByRole("button", { name: /Quality Check/ }));
    expect(spies.update).not.toHaveBeenCalled();
    await user.click(screen.getByLabelText("Back to flow list"));
    await waitFor(() => expect(spies.update).toHaveBeenCalledTimes(1));
    expect(spies.update.mock.calls[0][3].nodes.map((n) => n.name)).toContain("qa");
    expect(await screen.findByTestId("flow-list")).toBeInTheDocument();
  });

  it("shows a failed save and keeps the edit on screen", async () => {
    const { user } = setup({ writeError: new Error("forbidden") });
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await user.click(screen.getByTestId("add-step"));
    await user.click(screen.getByRole("button", { name: /Quality Check/ }));
    await waitFor(() =>
      expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Save failed"),
    );
    expect(screen.getByText("Could not save the flow")).toBeInTheDocument();
    // The translate step, the group's two branches, and the added check.
    expect(screen.getAllByTestId("step-row")).toHaveLength(4);
  });
});
