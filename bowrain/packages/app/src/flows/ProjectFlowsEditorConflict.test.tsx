import { afterEach, describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, TooltipProvider } from "@neokapi/ui";
import { ProjectFlowsEditor } from "./ProjectFlowsEditor";
import { createFlowsApi } from "./fixtures";
import { discardAllPendingSaves } from "../pendingSaves";

// The record of refused versions outlives the editor, so each test starts clean.
afterEach(() => discardAllPendingSaves());

// Two people edit the same project flow. The second save, based on the flow as
// it was before the first, is refused, and the person decides what stands.

function setup(saveDelayMs = 10) {
  const { api, flows } = createFlowsApi();
  const realUpdate = api.updateFlowDefinition.bind(api);
  const update = vi.spyOn(api, "updateFlowDefinition");
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
  return { api, flows, update, realUpdate, user: userEvent.setup() };
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

describe("ProjectFlowsEditor when the refused save is dismissed away from the flow", () => {
  it("a conflict raised by Back and dismissed on the list keeps the edit for the flow", async () => {
    const { api, flows, user } = setup(60_000);
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await teammateSaves(api);
    const before = screen.getAllByText("Quality Check").length;
    await addAStep(user);
    const edited = screen.getAllByText("Quality Check").length;
    expect(edited).toBeGreaterThan(before);

    // Back sends the resting edit; the refusal arrives over the list.
    await user.click(screen.getByLabelText("Back to flow list"));
    await screen.findByTestId("flow-changed-dialog");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByTestId("flow-changed-dialog")).toBeNull());

    expect(await screen.findByTestId("flow-unsaved-flow-review")).toBeInTheDocument();
    await user.click(await screen.findByText("Teammate's flow"));
    await screen.findByTestId("linear-flow-editor");
    expect(screen.getByTestId("flow-changed-elsewhere")).toBeInTheDocument();
    expect(screen.getAllByText("Quality Check").length).toBe(edited);
    expect(flows.find((f) => f.id === "flow-review")?.name).toBe("Teammate's flow");
  });

  it("a conflict that arrives while another flow is open keeps the edit for its own flow", async () => {
    const { api, update, realUpdate, user } = setup(60_000);
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await teammateSaves(api);
    // The person's save is held until another flow is open.
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    update.mockImplementation(async (...args) => {
      await gate;
      return realUpdate(...args);
    });
    await addAStep(user);
    await user.click(screen.getByLabelText("Back to flow list"));
    // The person opens another flow before the save's refusal comes back.
    await user.click(await screen.findByText("Pseudo Translate"));
    await screen.findByTestId("linear-flow-editor");
    release();

    await screen.findByTestId("flow-changed-dialog");
    await user.keyboard("{Escape}");
    await user.click(screen.getByLabelText("Back to flow list"));
    expect(await screen.findByTestId("flow-unsaved-flow-review")).toBeInTheDocument();
  });

  it("after a dismissal later changes do not ask again; Review does", async () => {
    const { api, update, user } = setup();
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    await teammateSaves(api);
    await addAStep(user);
    await screen.findByTestId("flow-changed-dialog");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByTestId("flow-changed-dialog")).toBeNull());
    const calls = update.mock.calls.length;

    await addAStep(user);
    await new Promise((r) => setTimeout(r, 100));
    expect(screen.queryByTestId("flow-changed-dialog")).toBeNull();
    expect(update.mock.calls.length).toBe(calls);
    expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Changed elsewhere");

    await user.click(screen.getByRole("button", { name: "Review" }));
    expect(await screen.findByTestId("flow-changed-dialog")).toBeInTheDocument();
  });
});
