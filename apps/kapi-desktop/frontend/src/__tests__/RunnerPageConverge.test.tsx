import { render, screen, waitFor } from "./testUtils";
import { describe, it, expect, vi, beforeEach } from "vitest";

// Stub the Wails-bridge API so the converge launch path is observable.
const bringUpToDateMock = vi.fn().mockResolvedValue(null);
const runFlowMock = vi.fn().mockResolvedValue(null);
const cancelRunMock = vi.fn().mockResolvedValue(null);
const getRunStateMock = vi.fn().mockResolvedValue("idle");
const getRunEventsMock = vi.fn().mockResolvedValue([]);
vi.mock("../hooks/useApi", () => ({
  api: {
    getRunState: (...args: unknown[]) => getRunStateMock(...args),
    getRunEvents: (...args: unknown[]) => getRunEventsMock(...args),
    cancelRun: (...args: unknown[]) => cancelRunMock(...args),
    aiNeedsModelChoice: vi.fn().mockResolvedValue(false),
    matchContent: vi.fn().mockResolvedValue([]),
    bringUpToDate: (...args: unknown[]) => bringUpToDateMock(...args),
    runFlow: (...args: unknown[]) => runFlowMock(...args),
  },
}));

import { RunnerPage } from "../components/RunnerPage";
import { JobFeedProvider } from "../context/JobFeedContext";

function renderWithProviders(ui: React.ReactElement) {
  return render(<JobFeedProvider>{ui}</JobFeedProvider>);
}

const project = {
  version: "v1",
  name: "Demo",
  defaults: { target_languages: ["fr-FR"], flow: "translate" },
  flows: { translate: { steps: [{ tool: "translate" }] } },
};

describe("RunnerPage converge mode (Bring up to date)", () => {
  beforeEach(() => {
    bringUpToDateMock.mockClear();
    runFlowMock.mockClear();
    cancelRunMock.mockClear();
    getRunStateMock.mockReset();
    getRunStateMock.mockResolvedValue("idle");
    getRunEventsMock.mockReset();
    getRunEventsMock.mockResolvedValue([]);
  });

  it("launches through BringUpToDate — not RunFlow — and renders the passes view", async () => {
    const onLaunched = vi.fn();
    renderWithProviders(
      <RunnerPage
        tabID="t1"
        flowName="translate"
        flow={project.flows.translate}
        project={project}
        autoRun
        converge
        onLaunched={onLaunched}
        onClose={vi.fn()}
      />,
    );

    // The convergence header + passes card render instead of the flow runner.
    expect(screen.getByText("Bring up to date · translate")).toBeInTheDocument();
    expect(screen.getByText("Passes")).toBeInTheDocument();
    // No manual configuration and no pipeline card in converge mode.
    expect(screen.queryByText("Select files...")).not.toBeInTheDocument();
    expect(screen.queryByText("Pipeline")).not.toBeInTheDocument();

    await waitFor(() => expect(bringUpToDateMock).toHaveBeenCalledWith("t1"));
    expect(runFlowMock).not.toHaveBeenCalled();
    expect(onLaunched).toHaveBeenCalledTimes(1);
  });

  it("renders a Cancel control while running and wires it to CancelRun", async () => {
    // Keep the backend "running" so the reconcile poll never settles the job.
    getRunStateMock.mockResolvedValue("running");
    renderWithProviders(
      <RunnerPage
        tabID="t1"
        flowName="translate"
        flow={project.flows.translate}
        project={project}
        autoRun
        converge
        onClose={vi.fn()}
      />,
    );

    await waitFor(() => expect(bringUpToDateMock).toHaveBeenCalledWith("t1"));
    const cancel = await screen.findByRole("button", { name: "Cancel flow execution" });
    const { default: userEvent } = await import("@testing-library/user-event");
    await userEvent.click(cancel);
    await waitFor(() => expect(cancelRunMock).toHaveBeenCalled());
  });

  it("settles into the cancelled state when the backend reports the run canceled", async () => {
    // The launch succeeds; the backend then reports a cancelled run whose
    // terminal event carries the "context canceled" marker (what CancelRun
    // produces through the shared up engine).
    getRunStateMock.mockResolvedValue("canceled");
    getRunEventsMock.mockResolvedValue([
      { type: "error", flow_id: "translate", message: "run canceled (context canceled)" },
    ]);
    renderWithProviders(
      <RunnerPage
        tabID="t1"
        flowName="translate"
        flow={project.flows.translate}
        project={project}
        autoRun
        converge
        onClose={vi.fn()}
      />,
    );

    await waitFor(() => expect(bringUpToDateMock).toHaveBeenCalledWith("t1"));
    // The converge view shows the terminal cancelled row…
    expect(await screen.findByText(/Cancelled\. The run stopped/)).toBeInTheDocument();
    // …and the Cancel control has returned to idle (gone).
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Cancel flow execution" }),
      ).not.toBeInTheDocument(),
    );
  });

  it("keeps the classic runner view for custom-flow runs", async () => {
    renderWithProviders(
      <RunnerPage
        tabID="t1"
        flowName="translate"
        flow={project.flows.translate}
        project={project}
        autoRun={false}
        onClose={vi.fn()}
      />,
    );
    expect(screen.getByText("Run: translate")).toBeInTheDocument();
    expect(screen.queryByText("Passes")).not.toBeInTheDocument();
    expect(bringUpToDateMock).not.toHaveBeenCalled();
  });
});
