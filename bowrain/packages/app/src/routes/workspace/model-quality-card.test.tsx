import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, type ApiAdapter, type ModelRecommendationsResponse } from "@neokapi/ui";
import { ModelQualityCard } from "./model-quality-card";

const measured: ModelRecommendationsResponse = {
  enabled: true,
  locales: [
    {
      locale: "fr",
      recommended_model: "claude-sonnet",
      models: [
        {
          model: "claude-sonnet",
          adherence: 0.92,
          adherence_bare: 0.54,
          lift: 0.38,
          fixture_count: 14,
          tokens_used: 2140,
          measured_at: "2026-07-15T08:00:00Z",
          recommended: true,
        },
        {
          model: "claude-haiku",
          adherence: 0.79,
          adherence_bare: 0.57,
          lift: 0.22,
          fixture_count: 14,
          tokens_used: 1660,
          measured_at: "2026-07-15T08:00:00Z",
          recommended: false,
        },
      ],
    },
  ],
};

function setup({
  recommendations = measured,
  refresh = vi.fn().mockResolvedValue({ enqueued: 1, locales: ["fr"] }),
} = {}) {
  const api = {
    getModelRecommendations: vi.fn().mockResolvedValue(recommendations),
    refreshModelRecommendations: refresh,
  } as unknown as ApiAdapter;
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={api}>
        <ModelQualityCard ws="acme" projectId="p-1" />
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { api, refresh };
}

describe("ModelQualityCard", () => {
  it("renders the per-locale table with adherence, lift, and the recommended row marked", async () => {
    setup();

    expect(await screen.findByText("claude-sonnet")).toBeVisible();
    expect(screen.getByText("claude-haiku")).toBeVisible();
    expect(screen.getByText("92%")).toBeVisible();
    expect(screen.getByText("+38 pts")).toBeVisible();
    expect(screen.getByText("+22 pts")).toBeVisible();
    // Exactly one recommended badge — on the lift winner's row.
    expect(screen.getAllByText("Recommended")).toHaveLength(1);
    const winnerRow = screen.getByText("claude-sonnet").closest("tr");
    expect(winnerRow).toHaveTextContent("Recommended");
  });

  it("queues a sweep on refresh and confirms without polling", async () => {
    const { refresh } = setup();

    // Wait for the measurements to load: until then the button is disabled
    // (an unknown gate must not enqueue).
    await screen.findByText("claude-sonnet");
    const button = screen.getByRole("button", { name: "Refresh model measurements" });
    expect(button).toBeEnabled();
    fireEvent.click(button);

    await waitFor(() => expect(refresh).toHaveBeenCalledWith("acme", "p-1"));
    expect(await screen.findByText(/Sweep queued/)).toBeVisible();
  });

  it("disables refresh with a reason while the instance flag is off", async () => {
    setup({ recommendations: { enabled: false, locales: [] } });

    expect(await screen.findByText(/No measurements yet/)).toBeVisible();
    expect(screen.getByText(/Model sweeps are disabled on this instance/)).toBeVisible();
    expect(screen.getByRole("button", { name: "Refresh model measurements" })).toBeDisabled();
  });

  it("surfaces a failed refresh without losing the table", async () => {
    setup({ refresh: vi.fn().mockRejectedValue(new Error("403")) });

    await screen.findByText("claude-sonnet");
    fireEvent.click(screen.getByRole("button", { name: "Refresh model measurements" }));
    expect(await screen.findByText(/Refresh failed/)).toBeVisible();
    expect(screen.getByText("claude-sonnet")).toBeVisible();
  });
});
