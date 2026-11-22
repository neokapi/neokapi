import { afterEach, beforeEach, describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, TooltipProvider } from "@neokapi/ui";
import { ProjectFlowsEditor } from "./ProjectFlowsEditor";
import { createFlowsApi, projectReview, sampleFlows } from "./fixtures";
import {
  discardAllPendingSaves,
  flushPendingSaves,
  hasPendingSaves,
  setPendingSavesUser,
} from "../pendingSaves";

// The desktop window closing while an edit rests before its delayed save.
// The window goes with the process, so whatever has not reached the server by
// then is lost; the host flushes the waiting saves first (flushPendingSaves).

// The record of unsaved edits outlives the editor, so each test starts clean.
beforeEach(() => setPendingSavesUser("u1"));
afterEach(() => discardAllPendingSaves());

const projectGlossary = {
  ...projectReview,
  id: "flow-terms",
  name: "Terms only",
  nodes: [{ id: "tool-0", type: "tool", name: "term-check", position: { x: 0, y: 0 } }],
  edges: [],
};

function setup(saveDelayMs = 60_000) {
  const { api, flows } = createFlowsApi({ flows: [...sampleFlows, projectGlossary] });
  const update = vi.spyOn(api, "updateFlowDefinition");
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const tree = (
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={api}>
        <TooltipProvider>
          <ProjectFlowsEditor workspaceSlug="acme" projectId="p-1" saveDelayMs={saveDelayMs} />
        </TooltipProvider>
      </ApiProvider>
    </QueryClientProvider>
  );
  const view = render(tree);
  return { flows, update, user: userEvent.setup(), view };
}

async function editAFlow(user: ReturnType<typeof userEvent.setup>, name = "Translate and review") {
  await user.click(await screen.findByText(name));
  await screen.findByTestId("linear-flow-editor");
  await user.click(screen.getByTestId("add-step"));
  await user.click(screen.getByRole("button", { name: /Quality Check/ }));
}

describe("ProjectFlowsEditor when the window closes", () => {
  it("a resting edit has not reached the server: closing now would lose it", async () => {
    const { user, update } = setup();
    await editAFlow(user);
    expect(update).not.toHaveBeenCalled();
    expect(hasPendingSaves()).toBe(true);
  });

  it("flushing the waiting saves sends the resting edit and waits for the server", async () => {
    const { user, update, flows } = setup();
    await editAFlow(user);

    const result = await flushPendingSaves(5_000);
    expect(result).toEqual({ ok: true });
    expect(update).toHaveBeenCalledTimes(1);
    expect(flows.find((f) => f.id === "flow-review")?.nodes.map((n) => n.name)).toContain("qa");
    expect(hasPendingSaves()).toBe(false);
  });

  it("a save that fails is reported, not dropped", async () => {
    const { user, update } = setup();
    update.mockRejectedValue(new Error("server unreachable"));
    await editAFlow(user);

    const result = await flushPendingSaves(5_000);
    expect(result).toMatchObject({ ok: false, timedOut: false });
    const errors = (result as { errors: { label: string; error: unknown }[] }).errors;
    expect(errors[0].label).toBe("Translate and review");
    expect(String(errors[0].error)).toContain("server unreachable");
    // The edit is still waiting, so a second close asks again.
    expect(hasPendingSaves()).toBe(true);
  });

  it("a save that fails after the person left the flow stays pending and shows", async () => {
    const { user, update } = setup();
    update.mockRejectedValue(new Error("server unreachable"));
    await editAFlow(user);
    await user.click(screen.getByLabelText("Back to flow list"));
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(/Not saved: Translate and review/)).toBeInTheDocument();
    expect(hasPendingSaves()).toBe(true);

    const result = await flushPendingSaves(5_000);
    expect(result).toMatchObject({ ok: false, timedOut: false });
  });

  it("one flow's successful save does not clear another flow's failure", async () => {
    const { user, update } = setup();
    update.mockRejectedValueOnce(new Error("server unreachable"));
    await editAFlow(user, "Translate and review");
    await user.click(screen.getByLabelText("Back to flow list"));
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));

    await editAFlow(user, "Terms only");
    await user.click(screen.getByLabelText("Back to flow list"));
    await waitFor(() => expect(update).toHaveBeenCalledTimes(2));

    expect(hasPendingSaves()).toBe(true);
    // The close retries the failed flow; this time the server takes it.
    expect(await flushPendingSaves(5_000)).toEqual({ ok: true });
    expect(update).toHaveBeenCalledTimes(3);
    expect(hasPendingSaves()).toBe(false);
  });

  it("the save sent when the editor goes away is still tracked", async () => {
    const { user, update, view } = setup();
    let fail!: (e: Error) => void;
    update.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          fail = reject;
        }),
    );
    await editAFlow(user);
    view.unmount();
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
    expect(hasPendingSaves()).toBe(true);

    fail(new Error("401 unauthorized"));
    await waitFor(() => expect(hasPendingSaves()).toBe(true));
    const result = await flushPendingSaves(5_000);
    // The retry reaches the server: the edit made in the unmounted editor is kept.
    expect(result).toEqual({ ok: true });
    expect(update).toHaveBeenCalledTimes(2);
  });

  it("discarding a failed edit lets the close go through", async () => {
    const { user, update } = setup();
    update.mockRejectedValue(new Error("server unreachable"));
    await editAFlow(user);
    await user.click(screen.getByLabelText("Back to flow list"));
    await user.click(await screen.findByRole("button", { name: "Discard" }));
    expect(hasPendingSaves()).toBe(false);
  });

  it("reopening a flow whose edit did not save shows the edit, not the server's copy", async () => {
    const { user, update } = setup();
    update.mockRejectedValue(new Error("server unreachable"));
    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    const before = screen.getAllByText("Quality Check").length;
    await user.click(screen.getByTestId("add-step"));
    await user.click(screen.getByRole("button", { name: /Quality Check/ }));
    await waitFor(() =>
      expect(screen.getAllByText("Quality Check").length).toBeGreaterThan(before),
    );
    const edited = screen.getAllByText("Quality Check").length;
    await user.click(screen.getByLabelText("Back to flow list"));
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));

    await user.click(await screen.findByText("Translate and review"));
    await screen.findByTestId("linear-flow-editor");
    expect(screen.getByTestId("flow-save-state")).toHaveTextContent("Save failed");
    expect(screen.getAllByText("Quality Check").length).toBe(edited);
  });
});
