import { describe, it, expect, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { WorkspaceRail } from "../components/WorkspaceRail";
import type { Workspace, User } from "../types/api";

function ws(id: string, name: string): Workspace {
  return { id, name, slug: name.toLowerCase(), description: "", logo_url: "", role: "owner" };
}

const testUser: User = { id: "u1", name: "Alice", email: "alice@example.com", avatar_url: "" };

describe("WorkspaceRail", () => {
  it("renders workspace icons for each workspace", () => {
    render(
      <WorkspaceRail
        workspaces={[ws("1", "Acme"), ws("2", "Beta")]}
        activeWorkspace={null}
        onSelectWorkspace={() => {}}
        onCreateWorkspace={() => {}}
        user={null}
      />,
    );
    const buttons = screen.getAllByRole("button");
    // 2 workspace icons + 1 create button
    expect(buttons).toHaveLength(3);
    expect(buttons[0]).toHaveAttribute("title", "Acme");
    expect(buttons[1]).toHaveAttribute("title", "Beta");
  });

  it("calls onSelectWorkspace when a workspace icon is clicked", () => {
    const handleSelect = vi.fn();
    const acme = ws("1", "Acme");
    render(
      <WorkspaceRail
        workspaces={[acme]}
        activeWorkspace={null}
        onSelectWorkspace={handleSelect}
        onCreateWorkspace={() => {}}
        user={null}
      />,
    );
    act(() => screen.getByTitle("Acme").click());
    expect(handleSelect).toHaveBeenCalledWith(acme);
  });

  it("renders create workspace button", () => {
    const handleCreate = vi.fn();
    render(
      <WorkspaceRail
        workspaces={[]}
        activeWorkspace={null}
        onSelectWorkspace={() => {}}
        onCreateWorkspace={handleCreate}
        user={null}
      />,
    );
    const createBtn = screen.getByTitle("Create workspace");
    act(() => createBtn.click());
    expect(handleCreate).toHaveBeenCalledOnce();
  });

  it("renders user avatar when user is provided", () => {
    render(
      <WorkspaceRail
        workspaces={[]}
        activeWorkspace={null}
        onSelectWorkspace={() => {}}
        onCreateWorkspace={() => {}}
        user={testUser}
        onAvatarClick={() => {}}
      />,
    );
    const avatar = screen.getByTitle("Alice");
    expect(avatar).toBeInTheDocument();
    expect(avatar.textContent).toBe("A");
  });

  it("does not render user avatar when user is null", () => {
    render(
      <WorkspaceRail
        workspaces={[]}
        activeWorkspace={null}
        onSelectWorkspace={() => {}}
        onCreateWorkspace={() => {}}
        user={null}
      />,
    );
    // Only the create button should be present
    expect(screen.getAllByRole("button")).toHaveLength(1);
  });

  it("calls onAvatarClick when avatar is clicked", () => {
    const handleAvatar = vi.fn();
    render(
      <WorkspaceRail
        workspaces={[]}
        activeWorkspace={null}
        onSelectWorkspace={() => {}}
        onCreateWorkspace={() => {}}
        user={testUser}
        onAvatarClick={handleAvatar}
      />,
    );
    act(() => screen.getByTitle("Alice").click());
    expect(handleAvatar).toHaveBeenCalledOnce();
  });

  it("falls back to email initial when user has no name", () => {
    const noName: User = { id: "u2", name: "", email: "bob@example.com", avatar_url: "" };
    render(
      <WorkspaceRail
        workspaces={[]}
        activeWorkspace={null}
        onSelectWorkspace={() => {}}
        onCreateWorkspace={() => {}}
        user={noName}
      />,
    );
    // Title falls back to email, button text shows "?" (since name is "", `(name || "?")[0]` = "?")
    const avatar = screen.getByTitle("bob@example.com");
    expect(avatar.textContent).toBe("?");
  });
});
