/**
 * Tests for the back-to-source review affordances (RV-F): proposing a source
 * change, the source-owner review surface, the FocusedReviewer propose button,
 * and the entity→concept promote wiring.
 */
import { describe, it, expect } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { ProposeSourceChangeDialog } from "../components/review/ProposeSourceChangeDialog";
import { SourceProposalsDialog } from "../components/review/SourceProposalsDialog";
import { FocusedReviewer } from "../components/review/FocusedReviewer";
import type { ReviewEntry } from "../components/review/reviewQueue";
import { ApiProvider } from "../context/ApiContext";
import { WorkspaceProvider } from "../context/WorkspaceContext";
import { createMockAdapter, type MockAdapter } from "../stories/mock-adapter";
import type { BlockInfo, SourceProposal, Workspace } from "../types/api";

const mockWorkspace: Workspace = {
  id: "ws-1",
  name: "Demo",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function withProviders(adapter: MockAdapter, ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ApiProvider adapter={adapter}>
        <WorkspaceProvider initialWorkspace={mockWorkspace}>{ui}</WorkspaceProvider>
      </ApiProvider>
    </QueryClientProvider>,
  );
}

function block(over: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b1",
    source: "Reset your password",
    source_coded: "Reset your password",
    source_spans: [],
    targets: { "fr-FR": { text: "Réinitialisez", status: "translated" } },
    translatable: true,
    has_spans: false,
    properties: {},
    entities: [
      { key: "entity:0", text: "password", type: "entity:product", start: 11, end: 19, dnt: false },
    ],
    ...over,
  };
}

function entry(over: Partial<ReviewEntry> = {}): ReviewEntry {
  return {
    id: "itm-1::b1::fr-FR",
    itemId: "itm-1",
    itemName: "auth.json",
    locale: "fr-FR",
    block: block(),
    issues: [],
    ...over,
  };
}

describe("ProposeSourceChangeDialog", () => {
  it("creates a source proposal on submit", async () => {
    const user = userEvent.setup();
    const adapter = createMockAdapter([]);
    withProviders(
      adapter,
      <ProposeSourceChangeDialog
        open
        onOpenChange={() => {}}
        projectId="proj-1"
        blockId="b1"
        itemName="ui.json"
        sourceLocale="en-US"
        foundInLocale="fr-FR"
        initialSource="Colour picker"
        localeCount={3}
      />,
    );

    const textarea = screen.getByTestId("propose-source-text");
    await user.clear(textarea);
    await user.type(textarea, "Color picker");
    await user.click(screen.getByTestId("propose-source-submit"));

    await waitFor(() => expect(adapter.createSourceProposalCalls).toHaveLength(1));
    expect(adapter.createSourceProposalCalls[0]).toMatchObject({
      block_id: "b1",
      item_name: "ui.json",
      proposed_source: "Color picker",
      kind: "text-fix",
      found_in_locale: "fr-FR",
    });
  });

  it("disables submit when the source is unchanged (text-fix)", async () => {
    const adapter = createMockAdapter([]);
    withProviders(
      adapter,
      <ProposeSourceChangeDialog
        open
        onOpenChange={() => {}}
        projectId="proj-1"
        blockId="b1"
        itemName="ui.json"
        sourceLocale="en-US"
        initialSource="Colour picker"
      />,
    );
    expect(screen.getByTestId("propose-source-submit")).toBeDisabled();
  });

  it("makes the wider blast radius legible (re-drafts all locales)", () => {
    const adapter = createMockAdapter([]);
    withProviders(
      adapter,
      <ProposeSourceChangeDialog
        open
        onOpenChange={() => {}}
        projectId="proj-1"
        blockId="b1"
        itemName="ui.json"
        sourceLocale="en-US"
        initialSource="Colour picker"
        localeCount={3}
      />,
    );
    expect(screen.getByTestId("propose-source-impact").textContent).toContain("all 3 locales");
  });
});

describe("SourceProposalsDialog", () => {
  const proposal: SourceProposal = {
    id: "sp-1",
    workspace_id: "ws",
    project_id: "proj-1",
    item_name: "ui.json",
    block_id: "b1",
    kind: "text-fix",
    original_source: "Colour picker",
    proposed_source: "Color picker",
    rationale: "US English",
    found_in_locale: "fr-FR",
    status: "open",
    created_at: new Date(0).toISOString(),
    updated_at: new Date(0).toISOString(),
  };

  it("lists open proposals and approves one", async () => {
    const user = userEvent.setup();
    const adapter = createMockAdapter([]);
    adapter.sourceProposals.push({ ...proposal });

    withProviders(
      adapter,
      <SourceProposalsDialog open onOpenChange={() => {}} projectId="proj-1" />,
    );

    await waitFor(() => expect(screen.getByTestId("source-proposal-item")).toBeInTheDocument());
    expect(screen.getByText("Color picker")).toBeInTheDocument();

    await user.click(screen.getByTestId("source-proposal-approve"));
    await waitFor(() => expect(adapter.decideSourceProposalCalls).toHaveLength(1));
    expect(adapter.decideSourceProposalCalls[0]).toMatchObject({
      proposalId: "sp-1",
      decision: "approve",
    });
  });

  it("shows an empty state when there are no proposals", async () => {
    const adapter = createMockAdapter([]);
    withProviders(
      adapter,
      <SourceProposalsDialog open onOpenChange={() => {}} projectId="proj-1" />,
    );
    await waitFor(() => expect(screen.getByTestId("source-proposals-empty")).toBeInTheDocument());
  });
});

describe("FocusedReviewer back-to-source affordances", () => {
  const baseProps = {
    entry: entry(),
    sourceLocale: "en-US",
    position: { index: 1, total: 3 },
    editing: false,
    onApprove: () => {},
    onSignOff: () => {},
    onReject: () => {},
    onEditToggle: () => {},
    onSaveEdit: () => {},
    onCancelEdit: () => {},
    onReCheck: () => {},
    onMarkTerm: () => {},
    onSuggestVoiceRule: () => {},
    onMakeRule: () => {},
  };

  it("proposes a source change for the whole block when nothing is selected", async () => {
    const user = userEvent.setup();
    const calls: string[] = [];
    render(<FocusedReviewer {...baseProps} onProposeSourceChange={(t) => calls.push(t)} />);
    await user.click(screen.getByTestId("source-propose-change"));
    expect(calls).toEqual(["Reset your password"]);
  });

  it("lights the entity Promote button and wires onEntityPromote", async () => {
    const user = userEvent.setup();
    const promoted: string[] = [];
    render(<FocusedReviewer {...baseProps} onEntityPromote={(k) => promoted.push(k)} />);
    // The document marks the entity where it occurs; the lane beneath it is
    // where the entity can be acted on.
    await user.click(screen.getByTestId("promote-entity-entity:0"));
    expect(promoted).toEqual(["entity:0"]);
  });
});
