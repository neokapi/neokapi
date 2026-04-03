import { useCallback } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type {
  BravoConversation,
  BravoMessage,
  BravoConfig,
  BravoToolInfo,
  BravoUsageSummary,
} from "../types/api";

export function useBravoApi() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const createConversation = useCallback(
    async (projectId?: string, title?: string): Promise<BravoConversation> =>
      api.bravoCreateConversation(ws, projectId, title),
    [api, ws],
  );

  const listConversations = useCallback(
    async (
      limit?: number,
      offset?: number,
    ): Promise<{ conversations: BravoConversation[]; total: number }> =>
      api.bravoListConversations(ws, limit, offset),
    [api, ws],
  );

  const getConversation = useCallback(
    async (
      conversationId: string,
    ): Promise<{ conversation: BravoConversation; messages: BravoMessage[] }> =>
      api.bravoGetConversation(ws, conversationId),
    [api, ws],
  );

  const deleteConversation = useCallback(
    async (conversationId: string): Promise<void> =>
      api.bravoDeleteConversation(ws, conversationId),
    [api, ws],
  );

  const sendMessage = useCallback(
    async (
      conversationId: string,
      content: string,
    ): Promise<{ user_message: BravoMessage; assistant_message: BravoMessage }> =>
      api.bravoSendMessage(ws, conversationId, content),
    [api, ws],
  );

  const listMessages = useCallback(
    async (
      conversationId: string,
      limit?: number,
      offset?: number,
    ): Promise<{ messages: BravoMessage[] }> =>
      api.bravoListMessages(ws, conversationId, limit, offset),
    [api, ws],
  );

  const approveToolCall = useCallback(
    async (conversationId: string, toolCallId: string): Promise<void> =>
      api.bravoApproveToolCall(ws, conversationId, toolCallId),
    [api, ws],
  );

  const denyToolCall = useCallback(
    async (conversationId: string, toolCallId: string): Promise<void> =>
      api.bravoDenyToolCall(ws, conversationId, toolCallId),
    [api, ws],
  );

  const cancelConversation = useCallback(
    async (conversationId: string): Promise<void> =>
      api.bravoCancelConversation(ws, conversationId),
    [api, ws],
  );

  const getConfig = useCallback(
    async (): Promise<BravoConfig> => api.bravoGetConfig(ws),
    [api, ws],
  );

  const updateConfig = useCallback(
    async (config: Partial<BravoConfig>): Promise<BravoConfig> => api.bravoUpdateConfig(ws, config),
    [api, ws],
  );

  const listTools = useCallback(
    async (): Promise<{ tools: BravoToolInfo[] }> => api.bravoListTools(ws),
    [api, ws],
  );

  const getUsage = useCallback(
    async (from?: string, to?: string): Promise<BravoUsageSummary> =>
      api.bravoGetUsage(ws, from, to),
    [api, ws],
  );

  return {
    createConversation,
    listConversations,
    getConversation,
    deleteConversation,
    sendMessage,
    listMessages,
    approveToolCall,
    denyToolCall,
    cancelConversation,
    getConfig,
    updateConfig,
    listTools,
    getUsage,
  };
}
