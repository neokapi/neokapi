import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { ProjectMemberManager } from "../components/ProjectMemberManager";
import { ApiProvider } from "../context/ApiContext";
import type { ApiAdapter } from "../api/adapter";
import type { ProjectMembership, RoleTemplate, Workspace } from "../types/api";

const workspace: Workspace = {
  id: "ws-1",
  name: "Northsea",
  slug: "northsea",
  description: "",
  logo_url: "",
  type: "team",
  role: "owner",
};

const roles = [
  { id: "role-reviewer", name: "reviewer", display_name: "Reviewer" },
] as RoleTemplate[];

function member(overrides: Partial<ProjectMembership> = {}): ProjectMembership {
  return {
    project_id: "p1",
    user_id: "u1",
    role_id: "role-reviewer",
    workspace_id: "ws-1",
    languages: [],
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function renderManager(members: ProjectMembership[]) {
  const api = {
    listProjectMembers: vi.fn(async () => members),
    listRoleTemplates: vi.fn(async () => roles),
    updateProjectMember: vi.fn(async (_ws: string, _p: string, userId: string, patch: object) =>
      member({ user_id: userId, ...patch }),
    ),
    addProjectMember: vi.fn(async (_ws: string, _p: string, input: object) => member(input)),
  } as unknown as ApiAdapter & Record<string, ReturnType<typeof vi.fn>>;
  const wrapper = ({ children }: { children: ReactNode }) => (
    <ApiProvider adapter={api}>{children}</ApiProvider>
  );
  render(
    <ProjectMemberManager workspace={workspace} projectId="p1" projectLanguages={["fr", "de"]} />,
    { wrapper },
  );
  return api;
}

describe("ProjectMemberManager: the region a member governs", () => {
  it("opens the member's region as axis rows and saves the edited map", async () => {
    const api = renderManager([member({ coordinates: { brand: "acme" } })]);
    await userEvent.click(await screen.findByTestId("project-member-edit-btn"));
    const editor = screen.getByTestId("project-member-coordinates");
    expect(within(editor).getByLabelText("brand")).toHaveValue("acme");

    await userEvent.type(within(editor).getByLabelText("New axis"), "channel");
    await userEvent.click(within(editor).getByRole("button", { name: /Add axis/ }));
    await userEvent.type(within(editor).getByLabelText("channel"), "support");
    await userEvent.click(screen.getByTestId("project-member-save-btn"));

    await waitFor(() =>
      expect(api.updateProjectMember).toHaveBeenCalledWith("northsea", "p1", "u1", {
        role_id: "role-reviewer",
        languages: undefined,
        coordinates: { brand: "acme", channel: "support" },
      }),
    );
  });

  it("refuses to save an axis without a value, and says which", async () => {
    const api = renderManager([member({ coordinates: { brand: "acme" } })]);
    await userEvent.click(await screen.findByTestId("project-member-edit-btn"));
    const editor = screen.getByTestId("project-member-coordinates");
    await userEvent.type(within(editor).getByLabelText("New axis"), "channel");
    await userEvent.click(within(editor).getByRole("button", { name: /Add axis/ }));
    await userEvent.click(screen.getByTestId("project-member-save-btn"));

    expect(screen.getByTestId("project-member-coordinates-error")).toHaveTextContent("channel");
    expect(within(editor).getByTestId("coordinate-value-required")).toHaveTextContent(
      "channel needs a value.",
    );
    expect(api.updateProjectMember).not.toHaveBeenCalled();
  });

  it("saves the whole space when the last axis is removed", async () => {
    const api = renderManager([member({ coordinates: { brand: "acme" } })]);
    await userEvent.click(await screen.findByTestId("project-member-edit-btn"));
    const editor = screen.getByTestId("project-member-coordinates");
    await userEvent.click(within(editor).getByRole("button", { name: "Remove brand" }));
    expect(within(editor).getByText("The whole space.")).toBeInTheDocument();
    await userEvent.click(screen.getByTestId("project-member-save-btn"));
    await waitFor(() =>
      expect(api.updateProjectMember).toHaveBeenCalledWith(
        "northsea",
        "p1",
        "u1",
        expect.objectContaining({ coordinates: undefined }),
      ),
    );
  });
});
