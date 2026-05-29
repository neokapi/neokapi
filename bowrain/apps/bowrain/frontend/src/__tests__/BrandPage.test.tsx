/* eslint-disable @typescript-eslint/unbound-method -- asserting on vi.fn() mock references is intentional */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import {
  ApiProvider,
  WorkspaceProvider,
  TooltipProvider,
  type ApiAdapter,
  type Workspace,
  type VoiceProfile,
  type CandidateRule,
  type ProjectInfo,
} from "@neokapi/ui";
import { BrandPage } from "../components/BrandPage";

const teamWorkspace: Workspace = {
  id: "ws1",
  name: "Acme",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "team",
  role: "owner",
};

const profile = { id: "prof-1", name: "House Voice" } as unknown as VoiceProfile;

const candidates: CandidateRule[] = [
  {
    term: "utilize",
    replacement: "use",
    correction_count: 4,
    dimension: "vocabulary",
    status: "pending",
  },
];

const projects = [{ id: "proj-1", name: "Marketing Site" } as unknown as ProjectInfo];

function makeAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return {
    listBrandProfiles: vi.fn().mockResolvedValue([profile]),
    listBrandCandidates: vi.fn().mockResolvedValue(candidates),
    promoteBrandRule: vi.fn().mockResolvedValue({ promoted: true }),
    rejectBrandRule: vi.fn().mockResolvedValue(undefined),
    evaluateBrandRule: vi.fn().mockResolvedValue({
      total_blocks: 10,
      affected_blocks: 2,
      improved_blocks: 1,
      degraded_blocks: 0,
      new_violations: 0,
      resolved_violations: 2,
      critical_count: 0,
      collections: [],
    }),
    getBrandDrift: vi.fn().mockResolvedValue({
      drifted: false,
      recent_avg: 90,
      baseline_avg: 91,
      drop: 1,
      recent_days: 30,
      recent_count: 12,
    }),
    ...overrides,
  } as unknown as ApiAdapter;
}

function renderPage(adapter: ApiAdapter, ws: Workspace = teamWorkspace) {
  return render(
    <TooltipProvider>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={ws}>
          <BrandPage projects={projects} />
        </WorkspaceProvider>
      </ApiProvider>
    </TooltipProvider>,
  );
}

describe("BrandPage", () => {
  beforeEach(() => vi.clearAllMocks());

  it("loads profiles then candidates and renders the reusable CandidateRulesList", async () => {
    const adapter = makeAdapter();
    renderPage(adapter);

    expect(adapter.listBrandProfiles).toHaveBeenCalledWith("acme");
    await waitFor(() =>
      expect(adapter.listBrandCandidates).toHaveBeenCalledWith("acme", "prof-1", { all: false }),
    );
    await waitFor(() => expect(screen.getByText("utilize")).toBeInTheDocument());
    // CandidateRulesList renders the replacement too.
    expect(screen.getByText("use")).toBeInTheDocument();
  });

  it("promotes a candidate through the adapter", async () => {
    const adapter = makeAdapter();
    renderPage(adapter);

    await waitFor(() => expect(screen.getByText("utilize")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: "Promote" }));

    await waitFor(() =>
      expect(adapter.promoteBrandRule).toHaveBeenCalledWith("acme", "prof-1", {
        term: "utilize",
        replacement: "use",
        correction_count: 4,
      }),
    );
  });

  it("evaluates blast radius once a project is selected and opens the dialog", async () => {
    const adapter = makeAdapter();
    renderPage(adapter);

    await waitFor(() => expect(screen.getByText("utilize")).toBeInTheDocument());

    // Pick a project so onEvaluate becomes available.
    fireEvent.change(screen.getByTestId("brand-project-select"), {
      target: { value: "proj-1" },
    });

    await waitFor(() => expect(adapter.getBrandDrift).toHaveBeenCalledWith("acme", "proj-1"));

    fireEvent.click(await screen.findByRole("button", { name: "Preview impact" }));

    await waitFor(() =>
      expect(adapter.evaluateBrandRule).toHaveBeenCalledWith("acme", "prof-1", {
        term: "utilize",
        replacement: "use",
        project_id: "proj-1",
      }),
    );
    // BlastRadiusSummary dialog content.
    expect(await screen.findByText(/Blast radius/)).toBeInTheDocument();
  });

  it("renders a connect prompt for a personal workspace", () => {
    const adapter = makeAdapter();
    renderPage(adapter, { ...teamWorkspace, type: "personal" });

    expect(screen.getByTestId("brand-empty")).toBeInTheDocument();
    expect(adapter.listBrandProfiles).not.toHaveBeenCalled();
  });
});
