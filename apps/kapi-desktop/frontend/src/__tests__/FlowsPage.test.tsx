import { render, screen, waitFor } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock the Wails-bridge API so adoption resolves deterministically without a
// Wails runtime. Must be declared before importing the component under test.
const adoptMock = vi.fn();
const getFlowMock = vi.fn();
vi.mock("../hooks/useApi", () => ({
  api: {
    adoptUserFlowIntoProject: (...args: unknown[]) => adoptMock(...args),
    getFlow: (...args: unknown[]) => getFlowMock(...args),
    listTools: () => Promise.resolve([]),
    listProjectTools: () => Promise.resolve([]),
    saveFlow: () => Promise.resolve(),
  },
}));

import { Toaster } from "@neokapi/ui-primitives";
import { FlowsPage, type FlowListItem } from "../components/FlowsPage";
import { ErrorProvider } from "../components/ErrorBanner";

const userFlows: FlowListItem[] = [
  {
    id: "my-translate",
    name: "my-translate",
    description: "",
    source: "user",
    stepCount: 2,
    steps: ["Recycle", "Translate"],
    isDefault: false,
  },
];

function renderAdhoc(props: Partial<React.ComponentProps<typeof FlowsPage>> = {}) {
  // Adoption feedback is a Sonner toast, mounted app-wide via <Toaster>; render
  // one here so the toast surfaces in the DOM under test.
  return render(
    <ErrorProvider>
      <FlowsPage flows={userFlows} {...props} />
      <Toaster />
    </ErrorProvider>,
  );
}

describe("FlowsPage adopt-into-project", () => {
  beforeEach(() => adoptMock.mockReset());

  it("hides the Add to project action when no project tab is open", () => {
    renderAdhoc();
    expect(screen.queryByLabelText("Add to project")).not.toBeInTheDocument();
  });

  it("adopts a user flow and surfaces the result", async () => {
    adoptMock.mockResolvedValue({ name: "my-translate", renamed: false });
    renderAdhoc({ adoptTabID: "tab-1", adoptProjectName: "Acme" });

    const btn = screen.getByLabelText("Add to project");
    await userEvent.click(btn);

    expect(adoptMock).toHaveBeenCalledWith("tab-1", "my-translate");
    await waitFor(() =>
      expect(screen.getByText(/Added "my-translate" to Acme/)).toBeInTheDocument(),
    );
  });

  it("mentions a rename when the adopted flow was renamed", async () => {
    adoptMock.mockResolvedValue({ name: "my-translate-2", renamed: true });
    renderAdhoc({ adoptTabID: "tab-1", adoptProjectName: "Acme" });

    await userEvent.click(screen.getByLabelText("Add to project"));
    await waitFor(() => expect(screen.getByText(/renamed to avoid a clash/)).toBeInTheDocument());
  });
});

describe("FlowsPage running a flow", () => {
  const projectFlows: FlowListItem[] = [
    {
      id: "convert",
      name: "convert",
      description: "",
      source: "project",
      stepCount: 1,
      steps: ["word-count"],
      isDefault: false,
    },
  ];

  beforeEach(() => {
    getFlowMock.mockReset();
    getFlowMock.mockResolvedValue({ steps: [{ tool: "word-count" }] });
  });

  function renderProject(onRunFlow?: React.ComponentProps<typeof FlowsPage>["onRunFlow"]) {
    return render(
      <ErrorProvider>
        <FlowsPage tabID="tab-1" flows={projectFlows} onRunFlow={onRunFlow} />
      </ErrorProvider>,
    );
  }

  it("runs the open flow through the runner", async () => {
    const onRunFlow = vi.fn();
    renderProject(onRunFlow);

    await userEvent.click(screen.getByText("convert"));
    const run = await screen.findByLabelText("Run flow");
    await userEvent.click(run);

    expect(onRunFlow).toHaveBeenCalledWith("convert", { steps: [{ tool: "word-count" }] });
  });

  it("offers no run action where nothing can run the flow", async () => {
    renderProject(undefined);

    await userEvent.click(screen.getByText("convert"));
    await waitFor(() => expect(screen.getByLabelText("Back to flow list")).toBeInTheDocument());
    expect(screen.queryByLabelText("Run flow")).not.toBeInTheDocument();
  });
});
