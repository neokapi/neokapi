import { describe, it, expect } from "vite-plus/test";
import { render, screen, act } from "@testing-library/react";
import { WorkspaceProvider, useWorkspace } from "../context/WorkspaceContext";
import type { Workspace } from "../types/api";

const ws1: Workspace = {
  id: "1",
  name: "Acme",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
  role: "owner",
};
const ws2: Workspace = {
  id: "2",
  name: "Beta",
  slug: "beta",
  description: "",
  logo_url: "",
  type: "team",
  role: "member",
};

function WorkspaceDisplay() {
  const { workspaces, activeWorkspace, setWorkspaces, setActiveWorkspace } = useWorkspace();
  return (
    <div>
      <span data-testid="count">{workspaces.length}</span>
      <span data-testid="active">{activeWorkspace?.name ?? "none"}</span>
      <button data-testid="set-workspaces" onClick={() => setWorkspaces([ws1, ws2])} />
      <button data-testid="set-active" onClick={() => setActiveWorkspace(ws2)} />
      <button data-testid="clear-active" onClick={() => setActiveWorkspace(null)} />
    </div>
  );
}

describe("WorkspaceContext", () => {
  it("starts empty with no initial workspace", () => {
    render(
      <WorkspaceProvider>
        <WorkspaceDisplay />
      </WorkspaceProvider>,
    );
    expect(screen.getByTestId("count").textContent).toBe("0");
    expect(screen.getByTestId("active").textContent).toBe("none");
  });

  it("accepts initial workspace", () => {
    render(
      <WorkspaceProvider initialWorkspace={ws1}>
        <WorkspaceDisplay />
      </WorkspaceProvider>,
    );
    expect(screen.getByTestId("count").textContent).toBe("1");
    expect(screen.getByTestId("active").textContent).toBe("Acme");
  });

  it("allows setting workspaces", () => {
    render(
      <WorkspaceProvider>
        <WorkspaceDisplay />
      </WorkspaceProvider>,
    );

    act(() => screen.getByTestId("set-workspaces").click());
    expect(screen.getByTestId("count").textContent).toBe("2");
  });

  it("allows setting and clearing active workspace", () => {
    render(
      <WorkspaceProvider initialWorkspace={ws1}>
        <WorkspaceDisplay />
      </WorkspaceProvider>,
    );

    act(() => screen.getByTestId("set-active").click());
    expect(screen.getByTestId("active").textContent).toBe("Beta");

    act(() => screen.getByTestId("clear-active").click());
    expect(screen.getByTestId("active").textContent).toBe("none");
  });

  it("throws when useWorkspace is called outside WorkspaceProvider", () => {
    expect(() => render(<WorkspaceDisplay />)).toThrow(
      "useWorkspace must be used within WorkspaceProvider",
    );
  });
});
