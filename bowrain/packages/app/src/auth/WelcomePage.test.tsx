import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, type ApiAdapter, type User, type Workspace } from "@neokapi/ui";
import { WelcomePage } from "./WelcomePage";

const newWorkspace: Workspace = {
  id: "ws-1",
  name: "Ada Lovelace",
  slug: "ada",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

const staleUser: User = {
  id: "user-1",
  email: "ada@example.com",
  name: "Ada Lovelace",
  avatar_url: "",
  onboarded_at: null,
};

function setup(overrides: Partial<ApiAdapter> = {}) {
  const api = {
    getOnboardingStatus: vi.fn().mockResolvedValue({
      needs_onboarding: true,
      email: "ada@example.com",
      display_name: "Ada Lovelace",
      suggested_slug: "ada",
    }),
    checkSlug: vi.fn().mockResolvedValue({ available: true }),
    completeOnboarding: vi.fn().mockResolvedValue(newWorkspace),
    ...overrides,
  } as unknown as ApiAdapter;
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  queryClient.setQueryData<User>(["currentUser"], staleUser);
  queryClient.setQueryData<Workspace[]>(["workspaces"], []);
  const onComplete = vi.fn();
  render(
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={api}>
        <WelcomePage onComplete={onComplete} />
      </ApiProvider>
    </QueryClientProvider>,
  );
  return { api, queryClient, onComplete };
}

describe("WelcomePage", () => {
  it("seeds the workspaces and currentUser caches before completing", async () => {
    const { api, queryClient, onComplete } = setup();

    await screen.findByText("Available");
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() => expect(onComplete).toHaveBeenCalledWith("ada"));
    expect(api.completeOnboarding).toHaveBeenCalledWith("ada", "Ada Lovelace");
    expect(queryClient.getQueryData<Workspace[]>(["workspaces"])).toEqual([newWorkspace]);
    expect(queryClient.getQueryData<User>(["currentUser"])?.onboarded_at).toBeTruthy();
  });

  it("does not duplicate an already-cached workspace", async () => {
    const { queryClient, onComplete } = setup();
    queryClient.setQueryData<Workspace[]>(["workspaces"], [newWorkspace]);

    await screen.findByText("Available");
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() => expect(onComplete).toHaveBeenCalledWith("ada"));
    expect(queryClient.getQueryData<Workspace[]>(["workspaces"])).toEqual([newWorkspace]);
  });

  it("shows an error and re-enables submit when onboarding fails", async () => {
    const { queryClient, onComplete } = setup({
      completeOnboarding: vi
        .fn()
        .mockRejectedValue(new Error('403: {"error":"signups are closed"}')),
    });

    await screen.findByText("Available");
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));

    await screen.findByText("Couldn't complete onboarding");
    expect(onComplete).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Continue" })).toBeEnabled();
    expect(queryClient.getQueryData<Workspace[]>(["workspaces"])).toEqual([]);
    expect(queryClient.getQueryData<User>(["currentUser"])?.onboarded_at).toBeNull();
  });
});
