import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, TooltipProvider } from "@neokapi/ui";
import { ProjectFlowsEditor } from "./ProjectFlowsEditor";
import { createFlowsApi } from "./fixtures";
import { flushPendingSaves, hasPendingSaves } from "../pendingSaves";

// The desktop window closing while an edit rests before its delayed save.
// The window goes with the process, so whatever has not reached the server by
// then is lost; the host flushes the waiting saves first (flushPendingSaves).

function setup(saveDelayMs = 60_000) {
  const { api, flows } = createFlowsApi();
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
  return { flows, update, user: userEvent.setup() };
}

async function editAFlow(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByText("Translate and review"));
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
    expect(String((result as { errors: unknown[] }).errors[0])).toContain("server unreachable");
    // The edit is still waiting, so a second close asks again.
    expect(hasPendingSaves()).toBe(true);
  });

  it("a save that fails after the person left the flow still shows", async () => {
    const { user, update } = setup();
    update.mockRejectedValue(new Error("server unreachable"));
    await editAFlow(user);
    await user.click(screen.getByLabelText("Back to flow list"));
    expect(await screen.findByText("Could not update the flows")).toBeInTheDocument();
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
  });
});
