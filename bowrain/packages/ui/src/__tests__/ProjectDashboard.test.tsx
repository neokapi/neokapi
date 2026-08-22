import { describe, it, expect, vi } from "vite-plus/test";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { ProjectDashboard } from "../components/ProjectDashboard";
import type { ProjectDashboardProps } from "../components/ProjectDashboard";
import { ApiProvider } from "../context/ApiContext";
import type { ApiAdapter } from "../api/adapter";
import type { ProjectInfo } from "../types/api";

// ProjectDashboard reaches the adapter only through useLocales.
const stubAdapter = {
  getKnownLocales: async () => [],
} as unknown as ApiAdapter;

function renderDashboard(projects: ProjectInfo[], extra: Partial<ProjectDashboardProps> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={stubAdapter}>{children}</ApiProvider>
    </QueryClientProvider>
  );
  return render(
    <ProjectDashboard
      projects={projects}
      onCreateProject={() => {}}
      onOpenProject={() => {}}
      workspaceName="Acme"
      {...extra}
    />,
    { wrapper },
  );
}

function makeProject(overrides: Partial<ProjectInfo> = {}): ProjectInfo {
  return {
    id: "p1",
    name: "Website",
    default_source_language: "en",
    target_languages: ["fr", "de"],
    created_at: "2026-01-01T00:00:00Z",
    modified_at: "2026-01-02T00:00:00Z",
    ...overrides,
  };
}

describe("ProjectDashboard (summary aggregates)", () => {
  it("renders card and stats totals from server aggregates without items[]", () => {
    // The summary list shape: aggregates only, items empty.
    renderDashboard([
      makeProject({
        id: "p1",
        name: "Website",
        items: [],
        item_count: 12,
        block_count: 300,
        word_count: 4500,
        stream_count: 2,
      }),
      makeProject({
        id: "p2",
        name: "Docs",
        target_languages: ["fr", "ja"],
        items: [],
        item_count: 3,
        word_count: 500,
        stream_count: 1,
      }),
    ]);

    const card = screen.getByTestId("project-card-p1");
    expect(within(card).getByText("12 files")).toBeInTheDocument();
    expect(within(card).getByText("4.5k words")).toBeInTheDocument();
    expect(within(card).getByText("2 streams")).toBeInTheDocument();

    // Stats bar: 15 files, 5.0k words, 3 unique locales across both projects.
    expect(screen.getByText("Files")).toBeInTheDocument();
    expect(screen.getByText("15")).toBeInTheDocument();
    expect(screen.getByText("5.0k")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("falls back to embedded items for adapters without aggregates", () => {
    // Legacy/full shape: no aggregate fields, embedded items.
    renderDashboard([
      makeProject({
        id: "p1",
        items: [
          {
            id: "i1",
            name: "a.json",
            format: "json",
            type: "file",
            size: 0,
            block_count: 5,
            word_count: 120,
          },
          {
            id: "i2",
            name: "b.json",
            format: "json",
            type: "file",
            size: 0,
            block_count: 2,
            word_count: 80,
          },
        ],
      }),
    ]);

    const card = screen.getByTestId("project-card-p1");
    expect(within(card).getByText("2 files")).toBeInTheDocument();
    expect(within(card).getByText("200 words")).toBeInTheDocument();
  });
});

describe("ProjectDashboard (first-run)", () => {
  it("renders the three distinct setup paths", () => {
    renderDashboard([], {
      onInviteTeam: () => {},
      onScanVoice: () => {},
      serverUrl: "https://bw.example",
    });
    expect(screen.getByText("Build your brand starter pack with your AI")).toBeInTheDocument();
    expect(screen.getByText("Create or import a project")).toBeInTheDocument();
    expect(screen.getByText("Invite your team")).toBeInTheDocument();
  });

  it("composes the assistant prompt from workspace name and server URL", () => {
    renderDashboard([], { serverUrl: "https://bw.example" });
    const prompt = screen.getByTestId("starter-prompt").textContent ?? "";
    expect(prompt).toContain("Install the kapi skill");
    expect(prompt).toContain("the Acme workspace");
    expect(prompt).toContain("https://bw.example");
  });

  it("omits the server clause without a serverUrl", () => {
    renderDashboard([]);
    const prompt = screen.getByTestId("starter-prompt").textContent ?? "";
    expect(prompt).toContain("connect this project to Bowrain.");
  });

  it("fires the invite handler from the invite card", () => {
    const onInviteTeam = vi.fn();
    renderDashboard([], { onInviteTeam });
    fireEvent.click(screen.getByTestId("onboarding-invite-btn"));
    expect(onInviteTeam).toHaveBeenCalledTimes(1);
  });

  it("offers the hosted context scan only when the handler is wired", () => {
    const { unmount } = renderDashboard([]);
    expect(screen.queryByTestId("onboarding-scan-brand")).not.toBeInTheDocument();
    unmount();

    const onScanVoice = vi.fn();
    renderDashboard([], { onScanVoice });
    fireEvent.click(screen.getByTestId("onboarding-scan-brand"));
    expect(onScanVoice).toHaveBeenCalledTimes(1);
  });

  it("opens the create dialog from the create card", () => {
    renderDashboard([]);
    fireEvent.click(screen.getByTestId("onboarding-create-btn"));
    expect(screen.getByTestId("create-project-dialog")).toBeInTheDocument();
  });

  it("queues dropped files and opens the create dialog", () => {
    renderDashboard([]);
    const file = new File(["{}"], "en.json", { type: "application/json" });
    fireEvent.drop(screen.getByTestId("onboarding-dropzone"), {
      dataTransfer: { files: [file] },
    });
    expect(screen.getByTestId("create-project-dialog")).toBeInTheDocument();
    expect(screen.getByText(/1 file ready to import/)).toBeInTheDocument();
  });

  it("keeps the sample-project link as the low-emphasis exit", () => {
    const onCreateSampleProject = vi.fn();
    renderDashboard([], { onCreateSampleProject });
    fireEvent.click(screen.getByText(/try a sample project/i));
    expect(onCreateSampleProject).toHaveBeenCalledTimes(1);
  });
});

describe("ProjectDashboard (loop status)", () => {
  it("renders the loop-status layer from fed props on the populated state", () => {
    renderDashboard([makeProject()], {
      loopStatus: {
        latestActivity: {
          summary: "Translate flow completed for Website (fr)",
          created_at: new Date().toISOString(),
        },
        openReviewTasks: 4,
        brand: { averageScore: 82, scoredProjects: 2, driftingProjects: 1 },
      },
    });
    const row = screen.getByTestId("loop-status");
    expect(within(row).getByText("Translate flow completed for Website (fr)")).toBeInTheDocument();
    expect(within(row).getByText("4")).toBeInTheDocument();
    expect(within(row).getByText("82")).toBeInTheDocument();
  });

  it("renders no loop-status layer without the prop (first paint, desktop)", () => {
    renderDashboard([makeProject()]);
    expect(screen.queryByTestId("loop-status")).not.toBeInTheDocument();
  });

  it("never renders the loop-status layer on first run", () => {
    renderDashboard([], { loopStatus: { openReviewTasks: 1 } });
    expect(screen.queryByTestId("loop-status")).not.toBeInTheDocument();
    expect(screen.getByTestId("empty-projects")).toBeInTheDocument();
  });
});
