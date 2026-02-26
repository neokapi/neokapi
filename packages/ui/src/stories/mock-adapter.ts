/**
 * In-memory ApiAdapter for Storybook.
 *
 * Returns realistic fixture data for editor-related endpoints.
 * Mutations are applied to a mutable blocks array so the editor
 * feels interactive without needing a real server.
 */

import type { ApiAdapter } from "../api/adapter";
import type {
  BlockInfo, TranslationStats, WordCountResult,
  TMMatchInfo, BlockTermMatch,
} from "../types/api";
import { sampleBlocks, sampleProject } from "./fixtures";

export function createMockAdapter(blocks?: BlockInfo[]): ApiAdapter {
  // Mutable copy so updates are reflected in subsequent reads
  const _blocks: BlockInfo[] = blocks
    ? blocks.map((b) => ({ ...b, targets: { ...b.targets }, targets_coded: { ...b.targets_coded } }))
    : sampleBlocks.map((b) => ({ ...b, targets: { ...b.targets }, targets_coded: { ...b.targets_coded } }));

  const noop = async () => {};
  const notImpl = () => { throw new Error("Not implemented in mock"); };

  return {
    // --- Config ---------------------------------------------------------
    getConfig: async () => ({ mode: "standalone", version: "0.0.0-storybook" }),

    // --- Auth -----------------------------------------------------------
    getCurrentUser: async () => ({
      id: "user-1",
      email: "translator@example.com",
      name: "Demo User",
      avatar_url: "",
    }),

    // --- Workspaces -----------------------------------------------------
    listWorkspaces: async () => [{
      id: "ws-1", name: "Demo Workspace", slug: "demo", description: "", logo_url: "", type: "personal", role: "owner",
    }],
    createWorkspace: notImpl,
    getWorkspace: async () => ({
      id: "ws-1", name: "Demo Workspace", slug: "demo", description: "", logo_url: "", type: "personal", role: "owner",
    }),
    updateWorkspace: notImpl,
    deleteWorkspace: notImpl,

    // --- Members --------------------------------------------------------
    listMembers: async () => [],
    addMember: noop,
    updateMemberRole: noop,
    removeMember: noop,

    // --- Invites --------------------------------------------------------
    listInvites: async () => [],
    createInvite: notImpl,
    deleteInvite: noop,
    acceptInvite: notImpl,

    // --- Claim ----------------------------------------------------------
    claimProject: notImpl,

    // --- Projects -------------------------------------------------------
    listProjects: async () => [sampleProject],
    createProject: notImpl,
    getProject: async () => sampleProject,
    deleteProject: noop,
    uploadFiles: notImpl,
    removeFile: notImpl,

    // --- Editor ---------------------------------------------------------
    getFileBlocks: async () => _blocks,

    updateBlockTarget: async (_ws, req) => {
      const blk = _blocks.find((b) => b.id === req.block_id);
      if (blk) {
        blk.targets[req.target_locale] = req.text;
        blk.targets_coded = blk.targets_coded ?? {};
        blk.targets_coded[req.target_locale] = req.text;
      }
    },

    updateBlockTargetCoded: async (_ws, req) => {
      const blk = _blocks.find((b) => b.id === req.block_id);
      if (blk) {
        blk.targets_coded = blk.targets_coded ?? {};
        blk.targets_coded[req.target_locale] = req.coded_text;
        // Also write plain text (strip Unicode markers)
        blk.targets[req.target_locale] = req.coded_text
          .replace(/[\uE001\uE002\uE003]/g, "");
      }
    },

    pseudoTranslateFile: async (): Promise<TranslationStats> => ({
      total_blocks: _blocks.length,
      translated_blocks: _blocks.length,
      word_count: 42,
    }),

    aiTranslateFile: async (): Promise<TranslationStats> => ({
      total_blocks: _blocks.length,
      translated_blocks: _blocks.length,
      word_count: 42,
    }),

    tmTranslateFile: async (): Promise<TranslationStats> => ({
      total_blocks: _blocks.length,
      translated_blocks: Math.floor(_blocks.length * 0.7),
      word_count: 30,
    }),

    getWordCount: async (): Promise<WordCountResult> => ({
      source_words: 42,
      source_chars: 220,
      target_words: { "fr-FR": 38, "de-DE": 12 },
      target_chars: { "fr-FR": 200, "de-DE": 60 },
    }),

    exportTranslatedFile: async () => new Blob(["mock export"], { type: "application/octet-stream" }),

    lookupTMForBlock: async (): Promise<TMMatchInfo[]> => [
      { source: "Welcome to Gokapi", target: "Bienvenue sur Gokapi", score: 100, match_type: "exact" },
      { source: "Welcome to the app", target: "Bienvenue dans l'application", score: 85, match_type: "fuzzy" },
    ],

    lookupTermsForBlock: async (): Promise<BlockTermMatch[]> => [
      { source_term: "localization", target_terms: ["localisation"], domain: "i18n", status: "preferred", start: 0, end: 12 },
    ],

    // --- TM -------------------------------------------------------------
    getTMEntries: async () => ({ entries: [], total_count: 0 }),
    getTMCount: async () => 0,
    addTMEntry: notImpl,
    updateTMEntry: noop,
    deleteTMEntry: noop,

    // --- Terms ----------------------------------------------------------
    getTerms: async () => ({ concepts: [], total_count: 0 }),
    getTermCount: async () => 0,
    addConcept: notImpl,
    updateConcept: noop,
    deleteConcept: noop,
    importTermsCSV: async () => 0,
    importTermsJSON: async () => 0,
    exportTermsJSON: async () => "{}",

    // --- Providers ------------------------------------------------------
    listProviderConfigs: async () => [
      { id: "prov-1", name: "Claude", provider_type: "anthropic", model: "claude-sonnet-4-20250514", base_url: "" },
    ],
    saveProviderConfig: notImpl,
    deleteProviderConfig: noop,
    testProviderConfig: noop,

    // --- Utility --------------------------------------------------------
    getKnownLocales: async () => [
      { code: "en-US", display_name: "English (United States)" },
      { code: "fr-FR", display_name: "French (France)" },
      { code: "de-DE", display_name: "German (Germany)" },
      { code: "ja-JP", display_name: "Japanese (Japan)" },
      { code: "es-ES", display_name: "Spanish (Spain)" },
      { code: "zh-CN", display_name: "Chinese (Simplified)" },
    ],
    listFormats: async () => [],
    listTools: async () => [],
  };
}
