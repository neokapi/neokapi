import { describe, expect, it, vi } from "vite-plus/test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ServerReviewPlan } from "@neokapi/bowrain-client";
import { ApiProvider, WorkspaceProvider, type ApiAdapter, type PermissionName } from "@neokapi/ui";
import { ReviewPlanPanel } from "./review-plan-panel";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}));

const plan: ServerReviewPlan = {
  stream: "main",
  involvement: "context",
  next: "coordinates",
  coordinates: { source_language: "en", confirmed: false },
  context: { waiting: 0, concepts: 0 },
  exceptions: {},
  languages: [{ locale: "fr", waiting: 1, failing: 0, pages: 0, settleable: 1 }],
};

function setup(permissions: PermissionName[], stream = "main") {
  const queries = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const api = {
    getCallerPermissions: vi.fn().mockResolvedValue({ permissions, languages: [] }),
    getReviewPlan: vi.fn().mockResolvedValue(plan),
    confirmReviewCoordinates: vi.fn().mockResolvedValue(undefined),
    setReviewInvolvement: vi.fn().mockResolvedValue(undefined),
    settleReviewPlan: vi.fn().mockResolvedValue({ approved: 1, first_pass: true }),
    startReviewPlanScan: vi.fn().mockResolvedValue(undefined),
  };
  render(
    <QueryClientProvider client={queries}>
      <ApiProvider adapter={api as unknown as ApiAdapter}>
        <WorkspaceProvider
          initialWorkspace={{
            id: "acme",
            slug: "acme",
            name: "Acme",
            description: "",
            logo_url: "",
            type: "team",
            role: "member",
          }}
        >
          <ReviewPlanPanel ws="acme" projectId="p1" stream={stream} />
        </WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { queries, api };
}

describe("the review plan on Overview", () => {
  it("shows a read-only plan to a viewer", async () => {
    setup(["view_content"]);
    expect(await screen.findByRole("combobox")).toBeDisabled();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("shows a refused update and reloads the project after a successful retry", async () => {
    const { api, queries } = setup(["manage_project"]);
    queries.setQueryData(["project", "acme", "p1", "draft"], {});
    api.confirmReviewCoordinates.mockRejectedValueOnce(new Error("Coordinates changed"));
    const confirm = await screen.findByRole("button", { name: "Confirm coordinates" });
    fireEvent.click(confirm);
    expect(await screen.findByRole("alert")).toHaveTextContent("Coordinates changed");
    fireEvent.click(confirm);
    await waitFor(() => expect(api.getReviewPlan).toHaveBeenCalledTimes(2));
    expect(queries.getQueryState(["project", "acme", "p1", "draft"])?.isInvalidated).toBe(true);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("offers project settings on another stream and keeps main coordinate confirmation on main", async () => {
    setup(["manage_project", "review"], "draft");
    await waitFor(() => expect(screen.getByRole("combobox")).toBeEnabled());
    expect(screen.queryByRole("button", { name: "Confirm coordinates" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Scan again" })).toBeNull();
    expect(screen.getByRole("button", { name: /Establish/ })).toBeEnabled();
  });
});
