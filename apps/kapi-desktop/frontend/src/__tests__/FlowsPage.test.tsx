import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock the Wails-bridge API so adoption resolves deterministically without a
// Wails runtime. Must be declared before importing the component under test.
const adoptMock = vi.fn();
vi.mock("../hooks/useApi", () => ({
  api: {
    adoptUserFlowIntoProject: (...args: unknown[]) => adoptMock(...args),
  },
}));

import { FlowsPage, type FlowListItem } from "../components/FlowsPage";
import { ErrorProvider } from "../components/ErrorBanner";

const userFlows: FlowListItem[] = [
  { id: "my-translate", name: "my-translate", description: "", source: "user", stepCount: 2 },
];

function renderAdhoc(props: Partial<React.ComponentProps<typeof FlowsPage>> = {}) {
  return render(
    <ErrorProvider>
      <FlowsPage flows={userFlows} {...props} />
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
