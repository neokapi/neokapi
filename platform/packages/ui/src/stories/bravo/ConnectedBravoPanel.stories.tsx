import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider } from "../../context/ApiContext";
import { WorkspaceProvider } from "../../context/WorkspaceContext";
import { BravoProvider, useBravo } from "../../context/BravoContext";
import { BravoPanelTrigger } from "../../components/bravo/BravoPanelTrigger";
import { BravoPanel } from "../../components/bravo/BravoPanel";
import { createMockAdapter } from "../mock-adapter";
import { sampleConversations, sampleMessages, sampleConfig } from "./fixtures";
import type { Workspace, BravoConversation, BravoMessage } from "../../types/api";

// ---------------------------------------------------------------------------
// ConnectedBravo — same pattern as workspace-layout.tsx
// ---------------------------------------------------------------------------

function ConnectedBravo() {
  const { state, actions } = useBravo();
  return (
    <div className="flex items-center gap-2 p-4">
      <BravoPanelTrigger onClick={actions.togglePanel} active={state.panelOpen} />
      <BravoPanel
        open={state.panelOpen}
        onOpenChange={(open) => (open ? actions.openPanel() : actions.closePanel())}
        conversations={state.conversations}
        activeConversation={state.activeConversation}
        messages={state.messages}
        streaming={state.streaming}
        streamingContent={state.streamingContent}
        onNewConversation={() => void actions.newConversation()}
        onSelectConversation={(c) => void actions.selectConversation(c)}
        onDeleteConversation={(c) => void actions.deleteConversation(c)}
        onSendMessage={(content) => void actions.sendMessage(content)}
        onApproveToolCall={(id) => void actions.approveToolCall(id)}
        onDenyToolCall={(id) => void actions.denyToolCall(id)}
        loading={state.loading}
        sendDisabled={state.streaming}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Storybook meta
// ---------------------------------------------------------------------------

const mockWorkspace: Workspace = {
  id: "ws-1",
  name: "Demo Workspace",
  slug: "demo",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

const adapter = createMockAdapter();
// Override bravoListConversations to return sample data
adapter.bravoListConversations = async () => ({
  conversations: sampleConversations,
  total: sampleConversations.length,
});
adapter.bravoGetConversation = async () => ({
  conversation: sampleConversations[0],
  messages: sampleMessages,
});

const meta: Meta<typeof ConnectedBravo> = {
  title: "Bravo/ConnectedBravoPanel",
  component: ConnectedBravo,
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
  },
  decorators: [
    (Story) => (
      <QueryClientProvider client={queryClient}>
        <ApiProvider adapter={adapter}>
          <WorkspaceProvider initialWorkspace={mockWorkspace}>
            <BravoProvider>
              <Story />
            </BravoProvider>
          </WorkspaceProvider>
        </ApiProvider>
      </QueryClientProvider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConnectedBravo>;

/**
 * Click the @bravo trigger button to open the panel. Conversations are
 * loaded from the mock adapter.
 */
export const Default: Story = {};
