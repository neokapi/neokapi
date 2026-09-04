import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { GovernanceSettings } from "../components/GovernanceSettings";
import { ApiProvider } from "../context/ApiContext";
import { AnalyticsProvider } from "../context/AnalyticsContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { DenyRule, Group, Workspace } from "../types/api";

const workspace: Workspace = {
  id: "ws-1",
  name: "Northsea",
  slug: "northsea",
  description: "",
  logo_url: "",
  type: "team",
  role: "owner",
};

function stubAdapter(overrides: Partial<ApiAdapter> = {}) {
  const groups: Group[] = [
    {
      id: "g1",
      workspace_id: "ws-1",
      name: "Reviewers",
      description: "",
      created_at: "2026-01-01T00:00:00Z",
      member_count: 3,
    },
  ];
  const denyRules: DenyRule[] = [
    {
      id: "d1",
      workspace_id: "ws-1",
      subject_type: "role",
      subject_id: "viewer",
      project_id: "",
      denied_perms: 4,
      reason: "",
      created_at: "2026-01-01T00:00:00Z",
    },
  ];
  return {
    getSoDMode: vi.fn(async () => ({ mode: "warn" as const })),
    setSoDMode: vi.fn(async () => {}),
    listGroups: vi.fn(async () => groups),
    createGroup: vi.fn(async (_ws: string, name: string) => ({
      id: "g2",
      workspace_id: "ws-1",
      name,
      description: "",
      created_at: "2026-01-01T00:00:00Z",
    })),
    deleteGroup: vi.fn(async () => {}),
    listDenyRules: vi.fn(async () => denyRules),
    createDenyRule: vi.fn(async () => denyRules[0]),
    deleteDenyRule: vi.fn(async () => {}),
    listRoleOverrides: vi.fn(async () => ({ member: ["review", "translate"] })),
    setRoleOverride: vi.fn(async () => {}),
    ...overrides,
  } as unknown as ApiAdapter & Record<string, ReturnType<typeof vi.fn>>;
}

function renderPage(api = stubAdapter(), capture = vi.fn()) {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <ApiProvider adapter={api}>
      <AnalyticsProvider capture={capture}>
        <WorkspaceProvider initialWorkspace={workspace}>{children}</WorkspaceProvider>
      </AnalyticsProvider>
    </ApiProvider>
  );
  render(<GovernanceSettings />, { wrapper });
  return { api, capture };
}

describe("GovernanceSettings (workspace policy)", () => {
  it("sits on the page header and the two section headings", async () => {
    renderPage();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Governance");
    const sections = screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
    expect(sections[0]).toContain("Who may decide");
    expect(sections[1]).toContain("Teams");
    expect(await screen.findByText("Reviewers")).toBeInTheDocument();
  });

  it("loads the policy: the mode, the overrides, the deny rules and the teams", async () => {
    renderPage();
    await waitFor(() =>
      expect(screen.getByLabelText("Separation of duties")).toHaveTextContent("Warn"),
    );
    expect(screen.getByLabelText("member permissions")).toHaveValue("review,translate");
    expect(screen.getByText(/denied perms 4/)).toBeInTheDocument();
    expect(screen.getByText("3 members")).toBeInTheDocument();
  });

  it("changes the separation-of-duties mode and records the save", async () => {
    const { api, capture } = renderPage();
    await screen.findByText("Reviewers");
    await userEvent.click(screen.getByLabelText("Separation of duties"));
    await userEvent.click(screen.getByRole("option", { name: "Block" }));
    await waitFor(() => expect(api.setSoDMode).toHaveBeenCalledWith("northsea", "block"));
    expect(capture).toHaveBeenCalledWith("settings_saved", { section: "governance" });
    expect(
      screen.getByText("Prevent anyone from reviewing or approving content they authored."),
    ).toBeInTheDocument();
  });

  it("adds a team and reloads", async () => {
    const { api } = renderPage();
    await screen.findByText("Reviewers");
    await userEvent.type(screen.getByLabelText("New team name"), "Editors{Enter}");
    await waitFor(() => expect(api.createGroup).toHaveBeenCalledWith("northsea", "Editors"));
    expect(api.listGroups).toHaveBeenCalledTimes(2);
  });

  it("adds a deny rule from the subject, id and permissions", async () => {
    const { api } = renderPage();
    await screen.findByText("Reviewers");
    await userEvent.click(screen.getByLabelText("Subject"));
    await userEvent.click(screen.getByRole("option", { name: "Group" }));
    await userEvent.type(screen.getByLabelText("Subject id"), "g1");
    await userEvent.type(screen.getByLabelText("Denied permissions"), "review, sign_off");
    await userEvent.click(screen.getByRole("button", { name: "Add deny" }));
    await waitFor(() =>
      expect(api.createDenyRule).toHaveBeenCalledWith("northsea", {
        subject_type: "group",
        subject_id: "g1",
        permissions: ["review", "sign_off"],
      }),
    );
  });

  it("saves a role override and records the save", async () => {
    const { api, capture } = renderPage();
    await screen.findByText("Reviewers");
    const row = screen.getByLabelText("viewer permissions").closest("li")!;
    await userEvent.type(screen.getByLabelText("viewer permissions"), "read");
    await userEvent.click(within(row).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(api.setRoleOverride).toHaveBeenCalledWith("northsea", "viewer", ["read"]),
    );
    expect(capture).toHaveBeenCalledWith("settings_saved", { section: "governance" });
  });

  it("reports a load failure in place", async () => {
    renderPage(
      stubAdapter({
        getSoDMode: vi.fn(async () => {
          throw new Error("boom");
        }),
      } as unknown as Partial<ApiAdapter>),
    );
    expect(await screen.findByText("Couldn't load the governance settings")).toBeInTheDocument();
  });
});
