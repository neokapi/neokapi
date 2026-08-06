import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace } from "../types/api";
import { ProfilesView } from "../brand-hub/profiles/ProfilesView";
import { ProfileDetailView } from "../brand-hub/profiles/ProfileDetailView";
import { CoordinateReadout } from "../brand-hub/profiles/Coordinates";
import { emptyProfiles, populatedProfiles } from "../stories/contextProfileFixtures";

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function adapterFor(response: unknown): ApiAdapter {
  return {
    listContextProfiles: vi.fn().mockResolvedValue(response),
  } as unknown as ApiAdapter;
}

function renderWithProviders(ui: ReactNode, adapter: ApiAdapter) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={workspace}>{ui}</WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
}

describe("CoordinateReadout", () => {
  it("renders axes alphabetically, whatever order they arrive in", () => {
    render(<CoordinateReadout coordinates={{ product: "bowrain", channel: "docs" }} />);
    const axes = screen.getAllByRole("term").map((el) => el.textContent);
    expect(axes).toEqual(["channel", "product"]);
  });

  it("renders nothing for a point with no coordinates", () => {
    const { container } = render(<CoordinateReadout coordinates={{}} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("ProfilesView", () => {
  it("lists the default profile alongside the declared points", async () => {
    renderWithProviders(<ProfilesView onOpenProfile={vi.fn()} />, adapterFor(populatedProfiles));

    await waitFor(() => expect(screen.getByText("Brand")).toBeInTheDocument());
    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.getByText("docs-bowrain")).toBeInTheDocument();
    expect(screen.getByText("app-bowrain")).toBeInTheDocument();
  });

  it("names the voice governing a point, and says so when none is bound", async () => {
    renderWithProviders(<ProfilesView onOpenProfile={vi.fn()} />, adapterFor(populatedProfiles));

    await waitFor(() => expect(screen.getByText("Bowrain docs voice")).toBeInTheDocument());
    expect(screen.getByText("No voice bound")).toBeInTheDocument();
  });

  it("separates the voices bound to no point", async () => {
    renderWithProviders(<ProfilesView onOpenProfile={vi.fn()} />, adapterFor(populatedProfiles));

    await waitFor(() => expect(screen.getByText("Voices with no point")).toBeInTheDocument());
    // The card's heading and its voice row both name it.
    expect(screen.getAllByText("Support voice")).toHaveLength(2);
  });

  it("tells a workspace with only the default point what would declare more", async () => {
    renderWithProviders(<ProfilesView onOpenProfile={vi.fn()} />, adapterFor(emptyProfiles));

    await waitFor(() => expect(screen.getByText("One point so far")).toBeInTheDocument());
    expect(screen.getByText(/kapi push/)).toBeInTheDocument();
  });
});

describe("ProfileDetailView", () => {
  const handlers = {
    onBack: vi.fn(),
    onOpenVoice: vi.fn(),
    onOpenTerms: vi.fn(),
    onOpenChanges: vi.fn(),
  };

  it("shows a point's coordinates, voice, content and pending changes", async () => {
    renderWithProviders(
      <ProfileDetailView slug="channel~docs.product~bowrain" {...handlers} />,
      adapterFor(populatedProfiles),
    );

    await waitFor(() => expect(screen.getByText("Coordinates")).toBeInTheDocument());
    expect(screen.getByText("Bowrain docs voice")).toBeInTheDocument();
    expect(screen.getByText("bowrain-docs")).toBeInTheDocument();
    expect(screen.getByText(/2 change-sets are in review/)).toBeInTheDocument();
    expect(screen.getByText(/\.kapi\/profiles\/docs-bowrain\//)).toBeInTheDocument();
  });

  it("reports the scan on the default profile, and says what it covers", async () => {
    renderWithProviders(
      <ProfileDetailView slug="default" {...handlers} />,
      adapterFor(populatedProfiles),
    );

    await waitFor(() => expect(screen.getByText("Brand scan")).toBeInTheDocument());
    expect(screen.getByText(/covers the whole workspace/)).toBeInTheDocument();
  });

  it("keeps the scan off a point, since a scan carries no coordinates", async () => {
    renderWithProviders(
      <ProfileDetailView slug="channel~docs.product~bowrain" {...handlers} />,
      adapterFor(populatedProfiles),
    );

    await waitFor(() => expect(screen.getByText("Coordinates")).toBeInTheDocument());
    expect(screen.queryByText("Brand scan")).not.toBeInTheDocument();
  });

  it("says a scan has never run rather than showing nothing", async () => {
    renderWithProviders(
      <ProfileDetailView slug="default" {...handlers} />,
      adapterFor(emptyProfiles),
    );

    await waitFor(() => expect(screen.getByText("Brand scan")).toBeInTheDocument());
    expect(screen.getByText(/No scan yet/)).toBeInTheDocument();
  });

  it("names a slug no profile answers to rather than rendering an empty page", async () => {
    renderWithProviders(
      <ProfileDetailView slug="channel~print" {...handlers} />,
      adapterFor(populatedProfiles),
    );

    await waitFor(() => expect(screen.getByText("No such profile")).toBeInTheDocument());
  });
});
