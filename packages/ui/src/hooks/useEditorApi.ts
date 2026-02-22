import { useCallback, useMemo } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type {
  BlockInfo,
  UpdateBlockRequest,
  UpdateBlockTargetCodedRequest,
  AITranslateFileRequest,
  TranslationStats,
  WordCountResult,
  TMMatchInfo,
  BlockTermMatch,
} from "../types/api";

export function useEditorApi() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const getFileBlocks = useCallback(
    async (projectId: string, fileName: string): Promise<BlockInfo[]> =>
      api.getFileBlocks(ws, projectId, fileName),
    [api, ws],
  );

  const updateBlockTarget = useCallback(
    async (req: UpdateBlockRequest): Promise<void> => api.updateBlockTarget(ws, req),
    [api, ws],
  );

  const updateBlockTargetCoded = useCallback(
    async (req: UpdateBlockTargetCodedRequest): Promise<void> =>
      api.updateBlockTargetCoded(ws, req),
    [api, ws],
  );

  const pseudoTranslateFile = useCallback(
    async (projectId: string, fileName: string, targetLocale: string): Promise<TranslationStats> =>
      api.pseudoTranslateFile(ws, projectId, fileName, targetLocale),
    [api, ws],
  );

  const aiTranslateFile = useCallback(
    async (req: AITranslateFileRequest): Promise<TranslationStats> =>
      api.aiTranslateFile(ws, req),
    [api, ws],
  );

  const tmTranslateFile = useCallback(
    async (projectId: string, fileName: string, targetLocale: string): Promise<TranslationStats> =>
      api.tmTranslateFile(ws, projectId, fileName, targetLocale),
    [api, ws],
  );

  const getWordCount = useCallback(
    async (projectId: string, fileName: string): Promise<WordCountResult> =>
      api.getWordCount(ws, projectId, fileName),
    [api, ws],
  );

  const exportTranslatedFile = useCallback(
    async (projectId: string, fileName: string, targetLocale: string): Promise<Blob> =>
      api.exportTranslatedFile(ws, projectId, fileName, targetLocale),
    [api, ws],
  );

  const lookupTMForBlock = useCallback(
    async (projectId: string, itemName: string, blockId: string, targetLocale: string): Promise<TMMatchInfo[]> =>
      api.lookupTMForBlock(ws, projectId, itemName, blockId, targetLocale),
    [api, ws],
  );

  const lookupTermsForBlock = useCallback(
    async (projectId: string, itemName: string, blockId: string, targetLocale: string): Promise<BlockTermMatch[]> =>
      api.lookupTermsForBlock(ws, projectId, itemName, blockId, targetLocale),
    [api, ws],
  );

  return useMemo(() => ({
    getFileBlocks,
    updateBlockTarget,
    updateBlockTargetCoded,
    pseudoTranslateFile,
    aiTranslateFile,
    tmTranslateFile,
    getWordCount,
    exportTranslatedFile,
    lookupTMForBlock,
    lookupTermsForBlock,
  }), [
    getFileBlocks,
    updateBlockTarget,
    updateBlockTargetCoded,
    pseudoTranslateFile,
    aiTranslateFile,
    tmTranslateFile,
    getWordCount,
    exportTranslatedFile,
    lookupTMForBlock,
    lookupTermsForBlock,
  ]);
}
