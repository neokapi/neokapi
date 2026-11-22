import { fireEvent, render, screen, waitFor } from "./testUtils";
import { describe, it, expect, vi, beforeEach } from "vite-plus/test";

// The launch path's AI setup check decides which dialog, if any, opens.
const bringUpToDateMock = vi.fn().mockResolvedValue(null);
const checkAISetupMock = vi.fn();
const setDefaultModelMock = vi.fn().mockResolvedValue(null);
vi.mock("../hooks/useApi", () => ({
  api: {
    getRunState: vi.fn().mockResolvedValue("idle"),
    getRunEvents: vi.fn().mockResolvedValue([]),
    cancelProjectRun: vi.fn().mockResolvedValue(null),
    checkAISetup: (...args: unknown[]) => checkAISetupMock(...args),
    matchContent: vi.fn().mockResolvedValue([]),
    bringUpToDate: (...args: unknown[]) => bringUpToDateMock(...args),
    runFlow: vi.fn().mockResolvedValue(null),
    listAIModels: vi.fn().mockResolvedValue([]),
    setDefaultModel: (...args: unknown[]) => setDefaultModelMock(...args),
  },
}));

import { RunnerPage } from "../components/RunnerPage";
import { JobFeedProvider } from "../context/JobFeedContext";
import { isGlobalOverlayView } from "../hooks/useTabManager";

const project = {
  version: "v1",
  name: "Demo",
  defaults: { target_languages: ["fr-FR"], flow: "converge" },
  flows: { converge: { steps: [{ tool: "translate", config: { provider: "anthropic" } }] } },
};

function renderRunner(onNavigate = vi.fn()) {
  render(
    <JobFeedProvider>
      <RunnerPage
        tabID="t1"
        flowName="converge"
        flow={project.flows.converge}
        project={project}
        autoRun
        converge
        onClose={vi.fn()}
        onNavigate={onNavigate}
      />
    </JobFeedProvider>,
  );
  return onNavigate;
}

const missing = {
  kind: "missing_credential",
  flow: "converge",
  tool: "translate",
  provider: "anthropic",
  message:
    "no saved credentials found for provider \"anthropic\"; save one with 'kapi credentials add' or set ANTHROPIC_API_KEY",
};

describe("RunnerPage AI setup before a run", () => {
  beforeEach(() => {
    bringUpToDateMock.mockClear();
    setDefaultModelMock.mockClear();
    checkAISetupMock.mockReset();
  });

  it("a named provider without a key leads to its key, not to a model choice", async () => {
    checkAISetupMock.mockResolvedValue(missing);
    const onNavigate = renderRunner();

    const dialog = await screen.findByTestId("ai-credential-missing");
    expect(dialog).toHaveTextContent("No key for anthropic");
    expect(dialog).toHaveTextContent('no saved credentials found for provider "anthropic"');
    expect(screen.queryByText("Choose an AI model")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Add a key" }));
    expect(onNavigate).toHaveBeenCalledWith("app-settings:credentials");
    expect(bringUpToDateMock).not.toHaveBeenCalled();
    expect(setDefaultModelMock).not.toHaveBeenCalled();
  });

  it("Cancel on a missing key starts nothing and changes no default", async () => {
    checkAISetupMock.mockResolvedValue(missing);
    renderRunner();
    await screen.findByTestId("ai-credential-missing");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByTestId("ai-credential-missing")).toBeNull());
    expect(bringUpToDateMock).not.toHaveBeenCalled();
    expect(setDefaultModelMock).not.toHaveBeenCalled();
  });

  it("no model chosen opens the model choice", async () => {
    checkAISetupMock.mockResolvedValue({ kind: "choose_model" });
    renderRunner();
    expect(await screen.findByText("Choose an AI model")).toBeInTheDocument();
    expect(screen.queryByTestId("ai-credential-missing")).toBeNull();
    expect(bringUpToDateMock).not.toHaveBeenCalled();
  });

  it("nothing needed runs at once", async () => {
    checkAISetupMock.mockResolvedValue({ kind: "" });
    renderRunner();
    await waitFor(() => expect(bringUpToDateMock).toHaveBeenCalledWith("t1"));
  });

  // Catch up is a convergence run: it asks as `kapi up` does, with no flow
  // name, so an up-to-date project is not asked for a key.
  it("asks about a convergence run, not about the recipe's flow by name", async () => {
    checkAISetupMock.mockResolvedValue({ kind: "" });
    renderRunner();
    await waitFor(() => expect(checkAISetupMock).toHaveBeenCalledWith("t1", ""));
  });
});

describe("opening App Settings on a section", () => {
  it("treats app-settings:credentials as the App Settings overlay", () => {
    expect(isGlobalOverlayView("app-settings:credentials")).toBe(true);
    expect(isGlobalOverlayView("app-settings")).toBe(true);
    expect(isGlobalOverlayView("context:rules")).toBe(false);
  });
});
