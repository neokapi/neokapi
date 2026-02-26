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
  BlockNote,
  BlockHistoryEntry,
  QAIssue,
  FileQAResult,
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

  const getBlockHistory = useCallback(
    async (projectId: string, blockId: string, locale: string, limit?: number): Promise<BlockHistoryEntry[]> =>
      api.getBlockHistory(ws, projectId, blockId, locale, limit),
    [api, ws],
  );

  const addBlockNote = useCallback(
    async (projectId: string, blockId: string, text: string): Promise<BlockNote> =>
      api.addBlockNote(ws, projectId, blockId, text),
    [api, ws],
  );

  const listBlockNotes = useCallback(
    async (projectId: string, blockId: string): Promise<BlockNote[]> =>
      api.listBlockNotes(ws, projectId, blockId),
    [api, ws],
  );

  const deleteBlockNote = useCallback(
    async (projectId: string, noteId: string): Promise<void> =>
      api.deleteBlockNote(ws, projectId, noteId),
    [api, ws],
  );

  const runQACheck = useCallback(
    async (projectId: string, blockId: string, locale: string): Promise<QAIssue[]> =>
      api.runQACheck(ws, projectId, blockId, locale),
    [api, ws],
  );

  const runFileQACheck = useCallback(
    async (projectId: string, fileName: string, locale: string): Promise<FileQAResult[]> =>
      api.runFileQACheck(ws, projectId, fileName, locale),
    [api, ws],
  );

  const renderDocumentPreview = useCallback(
    async (projectId: string, fileName: string, targetLocale: string): Promise<string> =>
      api.renderDocumentPreview(ws, projectId, fileName, targetLocale),
    [api, ws],
  );

  const renderBlockHTML = useCallback(
    async (projectId: string, blockId: string, targetLocale: string): Promise<string> =>
      api.renderBlockHTML(ws, projectId, blockId, targetLocale),
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
    getBlockHistory,
    addBlockNote,
    listBlockNotes,
    deleteBlockNote,
    runQACheck,
    runFileQACheck,
    renderDocumentPreview,
    renderBlockHTML,
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
    getBlockHistory,
    addBlockNote,
    listBlockNotes,
    deleteBlockNote,
    runQACheck,
    runFileQACheck,
    renderDocumentPreview,
    renderBlockHTML,
  ]);
}
