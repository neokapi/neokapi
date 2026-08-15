import { describe, it, expect, vi } from "vite-plus/test";
import type { ReactNode } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import type { ApiAdapter } from "../api/adapter";
import type { Workspace } from "../types/api";
import { ProfilesView } from "../brand-hub/profiles/ProfilesView";
import { ProfileDetailView } from "../brand-hub/profiles/ProfileDetailView";
import { CoordinateReadout } from "../brand-hub/profiles/Coordinates";
import {
  emptyProfiles,
  governedButUndeclared,
  populatedProfiles,
} from "../stories/contextProfileFixtures";

const workspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function adapterFor(response: unknown, proposals: unknown = { proposals: [] }): ApiAdapter {
  return {
    listContextProfiles: vi.fn().mockResolvedValue(response),
    listChannelProposals: vi.fn().mockResolvedValue(proposals),
    judgeChannelProposal: vi.fn(),
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

  it("carries each point's check standing, and says so when a point has none", async () => {
    renderWithProviders(<ProfilesView onOpenProfile={vi.fn()} />, adapterFor(populatedProfiles));

    await waitFor(() => expect(screen.getByText("87")).toBeInTheDocument());
    expect(screen.getByText(/412 checked blocks/)).toBeInTheDocument();
    expect(screen.getByText(/23 findings/)).toBeInTheDocument();
    // The other three points have never been checked.
    expect(screen.getAllByText("Not checked yet")).toHaveLength(3);
  });

  it("carries the workspace vocabulary onto every card, with the point's own rules", async () => {
    renderWithProviders(<ProfilesView onOpenProfile={vi.fn()} />, adapterFor(populatedProfiles));

    await waitFor(() => expect(screen.getAllByText(/248 concepts/)).toHaveLength(4));
    // A point with a voice narrows the vocabulary through it.
    expect(screen.getAllByText(/23 rules here/)).toHaveLength(3);
  });

  it("offers both onboarding lanes at the front door of an empty workspace", async () => {
    renderWithProviders(
      <ProfilesView onOpenProfile={vi.fn()} onScanBrand={vi.fn()} serverUrl="https://bw.example" />,
      adapterFor(emptyProfiles),
    );

    await waitFor(() => expect(screen.getByTestId("context-onboarding")).toBeInTheDocument());
    // The assistant lane, with the same copyable prompt the blank-workspace
    // landing offers.
    expect(screen.getByText("Build your brand starter pack with your AI")).toBeInTheDocument();
    const prompt = screen.getByTestId("starter-prompt").textContent ?? "";
    expect(prompt).toContain("Install the kapi skill");
    expect(prompt).toContain("the Demo workspace");
    expect(prompt).toContain("https://bw.example");
    // The hosted lane.
    expect(screen.getByTestId("context-onboarding-scan-btn")).toBeInTheDocument();
    // The deeper guidance is not what a workspace with nothing needs first.
    expect(screen.queryByText("One point so far")).not.toBeInTheDocument();
  });

  it("fires the hosted scan from the landing empty state", async () => {
    const onScanBrand = vi.fn();
    renderWithProviders(
      <ProfilesView onOpenProfile={vi.fn()} onScanBrand={onScanBrand} />,
      adapterFor(emptyProfiles),
    );

    fireEvent.click(await screen.findByTestId("context-onboarding-scan-btn"));
    expect(onScanBrand).toHaveBeenCalledTimes(1);
  });

  it("keeps the assistant lane when the server runs no hosted scan", async () => {
    renderWithProviders(<ProfilesView onOpenProfile={vi.fn()} />, adapterFor(emptyProfiles));

    await waitFor(() => expect(screen.getByTestId("context-onboarding")).toBeInTheDocument());
    expect(screen.getByTestId("starter-prompt")).toBeInTheDocument();
    expect(screen.queryByTestId("context-onboarding-scan")).not.toBeInTheDocument();
  });

  it("tells a workspace that has a voice but no points what would declare more", async () => {
    renderWithProviders(
      <ProfilesView onOpenProfile={vi.fn()} />,
      adapterFor(governedButUndeclared),
    );

    await waitFor(() => expect(screen.getByText("One point so far")).toBeInTheDocument());
    expect(screen.getByText(/kapi push/)).toBeInTheDocument();
    expect(screen.queryByTestId("context-onboarding")).not.toBeInTheDocument();
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

  it("reports the standing of the checks that resolved through this voice", async () => {
    renderWithProviders(
      <ProfileDetailView slug="channel~docs.product~bowrain" {...handlers} />,
      adapterFor(populatedProfiles),
    );

    await waitFor(() => expect(screen.getByText("Standing")).toBeInTheDocument());
    expect(screen.getByText("Voice score")).toBeInTheDocument();
    expect(screen.getByText(/Across 412 checked blocks/)).toBeInTheDocument();
    expect(screen.getByText(/Scoped by the voice, not the point/)).toBeInTheDocument();
  });

  it("says a point has never been checked rather than showing a zero", async () => {
    renderWithProviders(
      <ProfileDetailView slug="channel~app.product~bowrain" {...handlers} />,
      adapterFor(populatedProfiles),
    );

    await waitFor(() => expect(screen.getByText("Standing")).toBeInTheDocument());
    expect(screen.getByText(/Nothing here has been checked/)).toBeInTheDocument();
  });

  it("names a slug no profile answers to rather than rendering an empty page", async () => {
    renderWithProviders(
      <ProfileDetailView slug="channel~print" {...handlers} />,
      adapterFor(populatedProfiles),
    );

    await waitFor(() => expect(screen.getByText("No such profile")).toBeInTheDocument());
  });
});
