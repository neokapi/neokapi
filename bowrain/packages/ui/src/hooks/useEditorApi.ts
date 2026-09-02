import { useCallback, useMemo } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import { useStream } from "../context/StreamContext";
import type {
  BlockInfo,
  BlockCounts,
  BlockQueryOptions,
  BulkApplyMemoryRequest,
  BulkApplyMemoryResult,
  BulkReviewBlocksRequest,
  BulkReviewBlocksResult,
  ReviewDemotion,
  UpdateBlockRequest,
  UpdateBlockTargetCodedRequest,
  AITranslateFileRequest,
  TranslationStats,
  WordCountResult,
  MemoryMatchInfo,
  BlockTermMatch,
  ReviewContext,
  BlockNote,
  BlockHistoryEntry,
  QAIssue,
  FileQAResult,
  CreateSourceProposalRequest,
  PendingReviewOptions,
  PendingReviewPage,
} from "../types/api";

export function useEditorApi() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const { activeStream } = useStream();

  /**
   * One page of an item's blocks. `opts` carries the server-side filters — the
   * locale's status bucket, a substring over source and target, translatability
   * and the page bounds — so a narrowed view is a narrowed query.
   */
  const getFileBlocks = useCallback(
    async (projectId: string, fileName: string, opts?: BlockQueryOptions): Promise<BlockInfo[]> =>
      (await api.getFileBlocks(ws, projectId, fileName, activeStream, opts)) ?? [],
    [api, ws, activeStream],
  );

  /**
   * One block, in the shape `getFileBlocks` returns its elements. A surface
   * that has just written a target reads back what the server now holds
   * instead of reconstructing it — the demotion an edit causes is the store's
   * decision, not the client's to predict.
   */
  const getBlock = useCallback(
    async (projectId: string, blockId: string): Promise<BlockInfo> =>
      api.getBlock(ws, projectId, blockId, activeStream),
    [api, ws, activeStream],
  );

  /** The totals and status histogram for a block query, hydrating no block. */
  const getBlockCounts = useCallback(
    async (
      projectId: string,
      item?: string,
      locale?: string,
      opts?: { q?: string; translatable?: boolean },
    ): Promise<BlockCounts> => api.getBlockCounts(ws, projectId, item, locale, activeStream, opts),
    [api, ws, activeStream],
  );

  /** One review decision across a selection of blocks, in one request. */
  const bulkReviewBlocks = useCallback(
    async (req: Omit<BulkReviewBlocksRequest, "stream">): Promise<BulkReviewBlocksResult> =>
      api.bulkReviewBlocks(ws, { ...req, stream: activeStream }),
    [api, ws, activeStream],
  );

  /** The best content-memory match written into a selection, in one request. */
  const bulkApplyMemory = useCallback(
    async (req: Omit<BulkApplyMemoryRequest, "stream">): Promise<BulkApplyMemoryResult> =>
      api.bulkApplyMemory(ws, { ...req, stream: activeStream }),
    [api, ws, activeStream],
  );

  const getPendingReview = useCallback(
    async (
      projectId: string,
      opts?: Omit<PendingReviewOptions, "stream">,
    ): Promise<PendingReviewPage> =>
      api.getPendingReview(ws, projectId, { ...opts, stream: activeStream }),
    [api, ws, activeStream],
  );

  const updateBlockTarget = useCallback(
    async (req: UpdateBlockRequest): Promise<void> =>
      api.updateBlockTarget(ws, { ...req, stream: req.stream || activeStream }),
    [api, ws, activeStream],
  );

  const updateBlockTargetCoded = useCallback(
    async (req: UpdateBlockTargetCodedRequest): Promise<void> =>
      api.updateBlockTargetCoded(ws, {
        ...req,
        stream: req.stream || activeStream,
      }),
    [api, ws, activeStream],
  );

  const aiTranslateFile = useCallback(
    async (req: AITranslateFileRequest): Promise<TranslationStats> => api.aiTranslateFile(ws, req),
    [api, ws, activeStream],
  );

  const memoryTranslateFile = useCallback(
    async (projectId: string, fileName: string, targetLocale: string): Promise<TranslationStats> =>
      api.memoryTranslateFile(ws, projectId, fileName, targetLocale, activeStream),
    [api, ws, activeStream],
  );

  const getWordCount = useCallback(
    async (projectId: string, fileName: string): Promise<WordCountResult> =>
      api.getWordCount(ws, projectId, fileName, activeStream),
    [api, ws, activeStream],
  );

  const exportTranslatedFile = useCallback(
    async (projectId: string, fileName: string, targetLocale: string): Promise<Blob> =>
      api.exportTranslatedFile(ws, projectId, fileName, targetLocale, activeStream),
    [api, ws, activeStream],
  );

  const lookupMemoryForBlock = useCallback(
    async (
      projectId: string,
      itemName: string,
      blockId: string,
      targetLocale: string,
    ): Promise<MemoryMatchInfo[]> =>
      api.lookupMemoryForBlock(ws, projectId, itemName, blockId, targetLocale, activeStream),
    [api, ws, activeStream],
  );

  const lookupTermsForBlock = useCallback(
    async (
      projectId: string,
      itemName: string,
      blockId: string,
      targetLocale: string,
    ): Promise<BlockTermMatch[]> =>
      api.lookupTermsForBlock(ws, projectId, itemName, blockId, targetLocale, activeStream),
    [api, ws, activeStream],
  );

  const getReviewContext = useCallback(
    async (
      projectId: string,
      itemName: string,
      blockId: string,
      targetLocale: string,
    ): Promise<ReviewContext> =>
      api.getReviewContext(ws, projectId, itemName, blockId, targetLocale, activeStream),
    [api, ws, activeStream],
  );

  const getBlockHistory = useCallback(
    async (
      projectId: string,
      blockId: string,
      locale: string,
      limit?: number,
    ): Promise<BlockHistoryEntry[]> =>
      api.getBlockHistory(ws, projectId, blockId, locale, limit, activeStream),
    [api, ws, activeStream],
  );

  const rollbackBlock = useCallback(
    async (projectId: string, blockId: string, toSeq: number, locale: string): Promise<void> =>
      api.rollbackBlock(ws, projectId, blockId, toSeq, locale, activeStream),
    [api, ws, activeStream],
  );

  const approvePassing = useCallback(
    async (projectId: string, locales?: string[]) =>
      api.approvePassingReview(ws, projectId, { stream: activeStream, locales }),
    [api, ws, activeStream],
  );

  const reviewBlock = useCallback(
    async (
      projectId: string,
      itemName: string,
      blockId: string,
      targetLocale: string,
      reviewed: boolean,
      demoteTo?: ReviewDemotion,
    ): Promise<void> =>
      api.reviewBlock(
        ws,
        projectId,
        itemName,
        blockId,
        targetLocale,
        reviewed,
        activeStream,
        demoteTo,
      ),
    [api, ws, activeStream],
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
      api.runQACheck(ws, projectId, blockId, locale, activeStream),
    [api, ws, activeStream],
  );

  const runFileQACheck = useCallback(
    async (projectId: string, fileName: string, locale: string): Promise<FileQAResult[]> =>
      api.runFileQACheck(ws, projectId, fileName, locale, activeStream),
    [api, ws, activeStream],
  );

  const renderDocumentPreview = useCallback(
    async (projectId: string, fileName: string, targetLocale: string): Promise<string> =>
      api.renderDocumentPreview(ws, projectId, fileName, targetLocale, activeStream),
    [api, ws, activeStream],
  );

  const renderBlockHTML = useCallback(
    async (projectId: string, blockId: string, targetLocale: string): Promise<string> =>
      api.renderBlockHTML(ws, projectId, blockId, targetLocale, activeStream),
    [api, ws, activeStream],
  );

  // Back-to-source review (RV-F): propose a source-text change, list the open
  // proposals, decide them, and promote a marked entity to a concept.
  const createSourceProposal = useCallback(
    async (projectId: string, req: Omit<CreateSourceProposalRequest, "stream">) =>
      api.createSourceProposal(ws, projectId, { ...req, stream: activeStream }),
    [api, ws, activeStream],
  );

  const listSourceProposals = useCallback(
    async (projectId: string) => api.listSourceProposals(ws, projectId),
    [api, ws],
  );

  const decideSourceProposal = useCallback(
    async (
      projectId: string,
      proposalId: string,
      decision: "approve" | "reject",
      reason?: string,
    ) => api.decideSourceProposal(ws, projectId, proposalId, decision, reason),
    [api, ws],
  );

  const promoteEntityToConcept = useCallback(
    async (projectId: string, itemName: string, blockId: string, entityKey: string) =>
      api.promoteEntityToConcept(ws, projectId, itemName, blockId, entityKey, activeStream),
    [api, ws, activeStream],
  );

  return useMemo(
    () => ({
      getFileBlocks,
      getBlockCounts,
      getBlock,
      bulkReviewBlocks,
      bulkApplyMemory,
      getPendingReview,
      updateBlockTarget,
      updateBlockTargetCoded,
      aiTranslateFile,
      memoryTranslateFile,
      getWordCount,
      exportTranslatedFile,
      lookupMemoryForBlock,
      lookupTermsForBlock,
      getReviewContext,
      getBlockHistory,
      rollbackBlock,
      reviewBlock,
      approvePassing,
      addBlockNote,
      listBlockNotes,
      deleteBlockNote,
      runQACheck,
      runFileQACheck,
      renderDocumentPreview,
      renderBlockHTML,
      createSourceProposal,
      listSourceProposals,
      decideSourceProposal,
      promoteEntityToConcept,
    }),
    [
      getFileBlocks,
      getBlockCounts,
      getBlock,
      bulkReviewBlocks,
      bulkApplyMemory,
      getPendingReview,
      updateBlockTarget,
      updateBlockTargetCoded,
      aiTranslateFile,
      memoryTranslateFile,
      getWordCount,
      exportTranslatedFile,
      lookupMemoryForBlock,
      lookupTermsForBlock,
      getReviewContext,
      getBlockHistory,
      rollbackBlock,
      reviewBlock,
      approvePassing,
      addBlockNote,
      listBlockNotes,
      deleteBlockNote,
      runQACheck,
      runFileQACheck,
      renderDocumentPreview,
      renderBlockHTML,
      createSourceProposal,
      listSourceProposals,
      decideSourceProposal,
      promoteEntityToConcept,
    ],
  );
}
