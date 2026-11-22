import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, TooltipProvider } from "@neokapi/ui";
import { ProjectFlowsEditor } from "./ProjectFlowsEditor";
import { createFlowsApi } from "./fixtures";

// Two people edit the same project flow. The second save, based on the flow as
// it was before the first, is refused, and the person decides what stands.

function setup() {
  const { api, flows } = createFlowsApi();
  const update = vi.spyOn(api, "updateFlowDefinition");
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={api}>
        <TooltipProvider>
          <ProjectFlowsEditor workspaceSlug="acme" projectId="p-1" saveDelayMs={10} />
        </TooltipProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { api, flows, update, user: userEvent.setup() };
}

/** A teammate renames the flow from another window. */
async function teammateSaves(api: ReturnType<typeof createFlowsApi>["api"]) {
  const theirs = await api.getFlowDefinition("acme", "p-1", "flow-review");
  await api.updateFlowDefinition("acme", "p-1", "flow-review", {
    ...theirs,
    name: "Teammate's flow",
  });
}

async function addAStep(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByTestId("add-step"));
  await user.click(screen.getByRole("button", { name: /Quality Check/ }));
}

describe("ProjectFlowsEditor when someone else saved the flow", () => {
  it("refuses the stale save and keeps the teammate's flow until the person decides", async () => {
    const { api, flows, user } = setup();
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await teammateSaves(api);

    await addAStep(user);
    expect(await screen.findByTestId("flow-changed-dialog")).toBeInTheDocument();
    expect(flows.find((f) => f.id === "flow-review")?.name).toBe("Teammate's flow");
    expect(screen.getByTestId("flow-changed-current")).toBeInTheDocument();
    expect(screen.getByTestId("flow-changed-mine")).toBeInTheDocument();
  });

  it("Keep the current flow shows the teammate's version and sends nothing more", async () => {
    const { api, flows, update, user } = setup();
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await teammateSaves(api);
    await addAStep(user);
    await screen.findByTestId("flow-changed-dialog");
    const calls = update.mock.calls.length;

    await user.click(screen.getByTestId("flow-changed-keep"));
    await waitFor(() => expect(screen.queryByTestId("flow-changed-dialog")).toBeNull());
    expect(await screen.findByText("Teammate's flow")).toBeInTheDocument();
    expect(update.mock.calls.length).toBe(calls);
    expect(flows.find((f) => f.id === "flow-review")?.name).toBe("Teammate's flow");
  });

  it("Save my version saves over the flow as it now stands", async () => {
    const { api, flows, user } = setup();
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await teammateSaves(api);
    await addAStep(user);
    await screen.findByTestId("flow-changed-dialog");

    await user.click(screen.getByTestId("flow-changed-save-mine"));
    await waitFor(() => {
      const stored = flows.find((f) => f.id === "flow-review");
      expect(stored?.name).toBe("Translate and review");
      expect(stored?.revision).not.toBe("r-review");
    });
  });

  it("dismissing the dialog keeps the person's edit unsaved and on screen", async () => {
    const { api, flows, user } = setup();
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    const before = screen.getAllByText("Quality Check").length;
    await teammateSaves(api);
    await addAStep(user);
    await screen.findByTestId("flow-changed-dialog");

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByTestId("flow-changed-dialog")).toBeNull());
    // The added step is still in the editor, and nothing was written.
    expect(screen.getAllByText("Quality Check").length).toBeGreaterThan(before);
    expect(screen.getByTestId("flow-save-state")).not.toHaveTextContent("Saved");
    expect(flows.find((f) => f.id === "flow-review")?.name).toBe("Teammate's flow");
  });
});

describe("ProjectFlowsEditor after the person's own save", () => {
  it("reopening the flow before the list refreshes saves against the revision just written", async () => {
    const { api, update, user } = setup();
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    // The list's refetch after the save does not arrive before the person
    // reopens the flow.
    vi.spyOn(api, "listFlowDefinitions").mockImplementation(() => new Promise(() => {}));
    await addAStep(user);
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Saved"));

    await user.click(screen.getByLabelText("Back to flow list"));
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await addAStep(user);

    await waitFor(() => expect(update).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Saved"));
    expect(screen.queryByTestId("flow-changed-dialog")).toBeNull();
  });
});
