/**
 * In-memory ApiAdapter for Storybook.
 *
 * Returns realistic fixture data for editor-related endpoints.
 * Mutations are applied to a mutable blocks array so the editor
 * feels interactive without needing a real server.
 */

import type { ApiAdapter } from "../api/adapter";
import type {
  BlockInfo,
  ReviewDemotion,
  SkippedFile,
  TranslationStats,
  UploadFilesResult,
  WordCountResult,
  TMMatchInfo,
  BlockTermMatch,
  BlockNote,
  BlockHistoryEntry,
  QAIssue,
  FileQAResult,
  BrandScanRequest,
  BrandScanUploadResult,
  BrandScanCheckResult,
  BrandScanDraft,
  BrandScanJob,
} from "../types/api";
import type { VoiceProfile } from "../brand/types";
import type {
  AutomationRule,
  AutomationEvent,
  AutomationHistoryEntry,
  SaveAutomationRuleRequest,
} from "../types/api";
import {
  sampleBlocks,
  sampleProject,
  sampleBlockNotes,
  sampleBlockHistory,
  sampleQAIssues,
  sampleFileQAResults,
  sampleAutomationRules,
  sampleAutomationEvents,
  sampleAutomationHistory,
  sampleRoleTemplates,
} from "./fixtures";

// ---------------------------------------------------------------------------
// Preview HTML generation — turns a blocks array into a fully interactive
// iframe document with the kat-block postMessage protocol.
// ---------------------------------------------------------------------------

/** Convert a block's coded text back to display HTML using span data. */
function sourceToDisplayHTML(b: BlockInfo): string {
  const spans = b.source_spans ?? [];
  if (!b.has_spans || !b.source_coded || spans.length === 0) {
    return b.source.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }
  let result = "";
  let spanIdx = 0;
  for (const ch of b.source_coded) {
    const code = ch.codePointAt(0) ?? 0;
    if (code === 0xe001 || code === 0xe002 || code === 0xe003) {
      const span = spans[spanIdx++];
      if (span) result += span.data;
    } else if (ch === "&") {
      result += "&amp;";
    } else if (ch === "<") {
      result += "&lt;";
    } else if (ch === ">") {
      result += "&gt;";
    } else {
      result += ch;
    }
  }
  return result;
}

function generatePreviewHTML(blocks: BlockInfo[]): string {
  let isFirstHeading = true;
  const bodyLines = blocks.map((b) => {
    const displayHTML = sourceToDisplayHTML(b);
    const inner = `<kat-block data-block-id="${b.id}">${displayHTML}</kat-block>`;
    const isShort = b.source.length < 45 && !b.has_spans;
    if (isShort) {
      if (isFirstHeading) {
        isFirstHeading = false;
        return `  <h1>${inner}</h1>`;
      }
      return `  <h2>${inner}</h2>`;
    }
    return `  <p>${inner}</p>`;
  });

  // The script handles the kat-block postMessage protocol:
  // - kat-iframe-ready: signals the iframe is ready
  // - kat-select-block: highlights the selected block
  // - kat-insert-spacer: inserts a gap after the block for the editor card
  // - kat-remove-spacer: removes the spacer
  // - kat-update-block: updates block content (target preview)
  // - kat-content-height / kat-spacer-position: reports dimensions
  // - kat-block-click: notifies parent of block clicks
  const script = `<script>
(function(){
var spacer=null;
function rh(){parent.postMessage({type:"kat-content-height",height:document.documentElement.scrollHeight},"*")}
function sel(id){document.querySelectorAll("kat-block").forEach(function(el){el.classList.toggle("kat-selected",el.dataset.blockId===id)});var ow=document.querySelector(".kat-active-line");if(ow)ow.classList.remove("kat-active-line");var sb=document.querySelector('kat-block[data-block-id="'+id+'"]');if(sb){var w=sb.closest("h1,h2,h3,p,div,li,td,th")||sb.parentElement;if(w)w.classList.add("kat-active-line")}}
function ins(bid,h){
  rem();
  var b=document.querySelector('kat-block[data-block-id="'+bid+'"]');
  if(!b)return;
  var w=b.closest("h1,h2,h3,p,div,li,td,th")||b.parentElement;
  spacer=document.createElement("div");spacer.className="kat-spacer";spacer.style.height=h+"px";
  w.parentNode.insertBefore(spacer,w.nextSibling);
  var r=spacer.getBoundingClientRect();
  parent.postMessage({type:"kat-spacer-position",y:r.top+window.scrollY,contentHeight:document.documentElement.scrollHeight},"*");
}
function rem(){if(spacer&&spacer.parentNode){spacer.parentNode.removeChild(spacer);spacer=null}rh()}
document.querySelectorAll("kat-block").forEach(function(el){var w=el.closest("h1,h2,h3,p,div,li,td,th")||el.parentElement;if(w)w.classList.add("kat-wrapper")});
document.addEventListener("click",function(e){e.preventDefault();var b=e.target.closest?e.target.closest("kat-block"):null;if(!b){var w=e.target.closest?e.target.closest(".kat-wrapper"):null;if(w)b=w.querySelector("kat-block")}if(b)parent.postMessage({type:"kat-block-click",blockId:b.dataset.blockId},"*")});
window.addEventListener("message",function(e){
  var d=e.data;if(!d||!d.type)return;
  if(d.type==="kat-select-block")sel(d.blockId);
  if(d.type==="kat-insert-spacer")ins(d.blockId,d.height);
  if(d.type==="kat-remove-spacer")rem();
  if(d.type==="kat-update-block"){
    var el=document.querySelector('kat-block[data-block-id="'+d.blockId+'"]');
    if(el){if(d.html)el.innerHTML=d.html;else el.textContent=d.text||"";rh()}
  }
});
parent.postMessage({type:"kat-iframe-ready"},"*");setTimeout(rh,0);
})();
</script>`;

  return `<!DOCTYPE html>
<html>
<head>
<style>
*{box-sizing:border-box}
body{font-family:system-ui,-apple-system,BlinkMacSystemFont,sans-serif;margin:0;padding:40px 16px 80px;line-height:1.7;color:#1e293b;background:#fff}
kat-block{display:inline;border-radius:3px}
.kat-wrapper{cursor:pointer;position:relative;border-radius:6px;padding:6px 8px;margin:-6px -8px;transition:background .15s ease}
.kat-wrapper:hover:not(.kat-active-line){background:rgba(59,130,246,0.04)}
@keyframes kat-fade-in{from{opacity:0}to{opacity:1}}
.kat-active-line{position:relative;background:rgba(59,130,246,0.08);border-radius:6px;padding:6px 8px;margin:-6px -8px;animation:kat-fade-in .15s ease}
.kat-active-line::before{content:'';position:absolute;left:0;top:0;bottom:0;width:4px;background:#3b82f6;border-radius:2px 0 0 2px}
.kat-spacer{display:block;transition:height .25s ease}
h1{font-size:32px;font-weight:700;margin:0 0 12px;line-height:1.3}
h2{font-size:22px;font-weight:600;margin:32px 0 8px;line-height:1.3;color:#334155}
p{margin:0 0 16px;font-size:16px}
code{background:#e2e8f0;padding:1px 5px;border-radius:4px;font-size:0.9em;font-family:ui-monospace,monospace}
b{font-weight:600}
a{color:#6366f1;text-decoration:underline}
</style>
</head>
<body>
${bodyLines.join("\n")}
${script}
</body>
</html>`;
}

/** One recorded `reviewBlock` invocation (for component-test assertions). */
export interface ReviewBlockCall {
  workspaceSlug: string;
  projectId: string;
  itemName: string;
  blockId: string;
  targetLocale: string;
  reviewed: boolean;
  stream?: string;
  demoteTo?: ReviewDemotion;
}

/**
 * The mock adapter plus test hooks: recorded review calls and a failure
 * toggle so tests can assert the optimistic-update rollback path.
 */
export interface MockAdapter extends ApiAdapter {
  /** `reviewBlock` invocations in call order. */
  reviewBlockCalls: ReviewBlockCall[];
  /** When true, `reviewBlock` rejects instead of applying. */
  failReviewBlock: boolean;
  /** `startBrandScan` invocations in call order. */
  startBrandScanCalls: BrandScanRequest[];
  /** `uploadBrandScanSources` invocations — filenames per call. */
  uploadBrandScanSourcesCalls: string[][];
  /** `checkBrandDraft` invocations in call order. */
  checkBrandDraftCalls: { profileName: string; text: string }[];
  /**
   * States returned by successive `getBrandScan` calls: each call consumes
   * the next entry; the last entry repeats. Tests overwrite this to simulate
   * queued → processing → completed/failed progressions.
   */
  brandScanJobStates: BrandScanJob[];
}

// ---------------------------------------------------------------------------
// Brand-scan fixtures (deterministic — stories and tests assert on these)
// ---------------------------------------------------------------------------

/** Deterministic drafted profile for the brand-scan review fixtures. */
export const sampleBrandScanProfile: VoiceProfile = {
  id: "",
  name: "Acme Brand Voice",
  description: "Drafted from 3 sources by the brand scan.",
  tone: {
    personality: ["precise", "helpful", "direct"],
    formality: "neutral",
    emotion: "warm",
    humor: "none",
  },
  style: {
    active_voice: true,
    sentence_length: "medium",
    person_pov: "second",
    contractions: "sometimes",
  },
  vocabulary: {
    preferred_terms: [{ term: "workspace", note: "Preferred over 'account'." }],
    forbidden_terms: [{ term: "synergy", severity: "major" }],
    competitor_terms: [{ term: "QuickPay" }],
  },
  examples: [
    {
      before: "Leverage our synergistic platform.",
      after: "Use the workspace to publish in every language.",
      category: "vocabulary",
    },
  ],
  workspace_id: "ws-1",
  version: 0,
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
};

/** Deterministic completed brand-scan draft (profile + evidence + terms). */
export const sampleBrandScanDraft: BrandScanDraft = {
  profile: sampleBrandScanProfile,
  evidence: {
    fields: {
      tone: { confidence: 0.82, source: "Consistent register across brand-guide.docx" },
      style: { confidence: 0.71, source: "Short imperative sentences on the landing pages" },
      vocabulary: { confidence: 0.9, source: "Glossary section in brand-guide.docx" },
      examples: { confidence: 0.55, source: "Rewrites derived from the blog posts" },
    },
  },
  terms: [
    {
      term: "workspace",
      definition: "The shared container for projects and members.",
      domain: "product",
    },
    {
      term: "stream",
      definition: "A named line of content development within a project.",
      domain: "product",
    },
    {
      term: "convergence",
      definition: "The process that settles translations against checks.",
      domain: "product",
    },
  ],
  sources: [
    { kind: "upload", label: "brand-guide.docx", runes: 48210 },
    { kind: "url", label: "https://acme.example/about", runes: 9120 },
    { kind: "paste", label: "pasted text", runes: 1834 },
  ],
  truncated: false,
};

/** A completed brand-scan job carrying the sample draft. */
export const sampleBrandScanJob: BrandScanJob = {
  id: "scan-1",
  status: "completed",
  progress: 100,
  message: "done",
  tokens_used: 12840,
  draft: sampleBrandScanDraft,
};

/** File extensions the mock brand-scan upload endpoint accepts. */
const BRAND_SCAN_KNOWN_EXTENSIONS = new Set([
  "md",
  "markdown",
  "html",
  "htm",
  "xhtml",
  "txt",
  "text",
  "csv",
  "tsv",
  "docx",
  "xlsx",
  "odt",
  "ods",
  "odp",
  "epub",
  "xml",
  "json",
  "yaml",
  "yml",
]);

/** Extensions recognized but deferred (need a pdf/pptx text extractor). */
const BRAND_SCAN_DEFERRED_EXTENSIONS = new Set(["pdf", "ppt", "pptx", "pptm", "ppsx", "potx"]);

/** File extensions the mock "server" pretends to have format readers for. */
const MOCK_KNOWN_EXTENSIONS = new Set([
  "json",
  "html",
  "htm",
  "md",
  "mdx",
  "xliff",
  "xlf",
  "po",
  "yaml",
  "yml",
]);

export function createMockAdapter(blocks?: BlockInfo[]): MockAdapter {
  // Mutable copy so updates are reflected in subsequent reads
  const _blocks: BlockInfo[] = blocks
    ? blocks.map((b) => ({
        ...b,
        targets: { ...b.targets },
        targets_coded: { ...b.targets_coded },
      }))
    : sampleBlocks.map((b) => ({
        ...b,
        targets: { ...b.targets },
        targets_coded: { ...b.targets_coded },
      }));

  const _automationRules: AutomationRule[] = sampleAutomationRules.map((r) => ({
    ...r,
    conditions: [...r.conditions],
    actions: [...r.actions],
  }));

  const noop = async () => {};
  const notImpl = () => {
    throw new Error("Not implemented in mock");
  };

  // Replace a target's text while preserving its per-locale review status —
  // the server's SetTargetText/SetTargetRuns preserve Target.Status, so the
  // mock must not clobber a {text, status} entry with a bare string.
  const setTargetPreservingStatus = (blk: BlockInfo, locale: string, text: string) => {
    const entry = blk.targets[locale];
    const status = entry != null && typeof entry === "object" ? entry.status : undefined;
    blk.targets[locale] = status ? { text, status } : text;
  };

  const reviewBlockCalls: ReviewBlockCall[] = [];
  const startBrandScanCalls: BrandScanRequest[] = [];
  const uploadBrandScanSourcesCalls: string[][] = [];
  const checkBrandDraftCalls: { profileName: string; text: string }[] = [];
  let brandScanPollIndex = 0;

  const adapter: MockAdapter = {
    // --- Test hooks -------------------------------------------------------
    reviewBlockCalls,
    failReviewBlock: false,
    startBrandScanCalls,
    uploadBrandScanSourcesCalls,
    checkBrandDraftCalls,
    brandScanJobStates: [sampleBrandScanJob],

    // --- Config ---------------------------------------------------------
    getConfig: async () => ({
      mode: "standalone",
      version: "0.0.0-storybook",
      commit: "storybook",
      build_date: "unknown",
      // The mock backend answers every brand-scan call, so the capability is on.
      features: { brand_scan: true },
    }),

    // --- Auth -----------------------------------------------------------
    getCurrentUser: async () => ({
      id: "user-1",
      email: "translator@example.com",
      name: "Demo User",
      avatar_url: "",
      onboarded_at: "2024-01-01T00:00:00Z",
    }),

    // --- Account management --------------------------------------------
    getOnboardingStatus: async () => ({
      needs_onboarding: false,
      email: "translator@example.com",
      display_name: "Demo User",
    }),
    completeOnboarding: async () => ({
      id: "ws-1",
      name: "Demo",
      slug: "demo",
      description: "",
      logo_url: "",
      type: "personal",
      role: "owner",
    }),
    checkSlug: async () => ({ available: true }),
    requestEmailChange: async () => ({
      status: "verification sent",
      new_email: "new@example.com",
      expires_at: new Date(Date.now() + 86_400_000).toISOString(),
    }),
    confirmEmailChange: async () => ({
      status: "email updated",
      new_email: "new@example.com",
    }),
    adminListSlugReservations: async () => [],
    adminReleaseSlugReservation: notImpl,

    // --- Workspaces -----------------------------------------------------
    listWorkspaces: async () => [
      {
        id: "ws-1",
        name: "Demo Workspace",
        slug: "demo",
        description: "",
        logo_url: "",
        type: "personal",
        role: "owner",
      },
    ],
    createWorkspace: notImpl,
    getWorkspace: async () => ({
      id: "ws-1",
      name: "Demo Workspace",
      slug: "demo",
      description: "",
      logo_url: "",
      type: "personal",
      role: "owner",
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

    // --- Role Templates --------------------------------------------------
    listRoleTemplates: async () => [...sampleRoleTemplates],
    createRoleTemplate: notImpl,
    updateRoleTemplate: notImpl,
    deleteRoleTemplate: noop,

    // --- Project Members -------------------------------------------------
    listProjectMembers: async () => [],
    addProjectMember: notImpl,
    updateProjectMember: notImpl,
    removeProjectMember: noop,

    // --- API Tokens -----------------------------------------------------
    listApiTokens: async () => [],
    createApiToken: notImpl,
    deleteApiToken: noop,

    // --- Claim ----------------------------------------------------------
    claimProject: notImpl,

    // --- Projects -------------------------------------------------------
    listProjects: async () => [sampleProject],
    createProject: notImpl,
    getProject: async () => sampleProject,
    updateProject: async () => sampleProject,
    deleteProject: noop,
    uploadFiles: async (_ws, _projectId, files): Promise<UploadFilesResult> => {
      // Simulate the server's per-file skip behaviour: files without a known
      // format extension are reported in `skipped` instead of imported.
      const skipped: SkippedFile[] = files
        .filter((f) => !MOCK_KNOWN_EXTENSIONS.has(f.name.split(".").pop()?.toLowerCase() ?? ""))
        .map((f) => ({
          name: f.name,
          reason: `no format reader for ".${f.name.split(".").pop() ?? ""}"`,
        }));
      return skipped.length > 0 ? { ...sampleProject, skipped } : { ...sampleProject };
    },
    removeFile: notImpl,

    // --- Archive / Recycle Bin ----------------------------------------------------
    restoreProject: noop,
    permanentlyDeleteProject: noop,
    listArchivedProjects: async () => [],
    restoreStream: noop,

    // --- Audit Log -------------------------------------------------------
    listWorkspaceAuditLog: async () => [],
    verifyWorkspaceAuditChain: async () => ({
      chain_key: "",
      rows: 0,
      valid: true,
    }),

    // --- Collections ----------------------------------------------------
    listCollections: async () => [],
    createCollection: notImpl,
    getCollection: notImpl,
    updateCollection: notImpl,
    deleteCollection: noop,
    uploadToCollection: notImpl,

    // --- Editor ---------------------------------------------------------
    getFileBlocks: async () => _blocks,

    updateBlockTarget: async (_ws, req) => {
      const blk = _blocks.find((b) => b.id === req.block_id);
      if (blk) {
        setTargetPreservingStatus(blk, req.target_locale, req.text);
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
        setTargetPreservingStatus(
          blk,
          req.target_locale,
          req.coded_text.replace(/[\uE001\uE002\uE003]/g, ""),
        );
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

    exportTranslatedFile: async () =>
      new Blob(["mock export"], { type: "application/octet-stream" }),

    lookupTMForBlock: async (): Promise<TMMatchInfo[]> => [
      {
        source: "Welcome to Neokapi",
        target: "Bienvenue sur Neokapi",
        score: 100,
        match_type: "exact",
      },
      {
        source: "Welcome to the app",
        target: "Bienvenue dans l'application",
        score: 85,
        match_type: "fuzzy",
      },
    ],

    lookupTermsForBlock: async (): Promise<BlockTermMatch[]> => [
      {
        source_term: "localization",
        target_terms: ["localisation"],
        domain: "i18n",
        status: "preferred",
        start: 0,
        end: 12,
      },
    ],

    // --- Block notes ----------------------------------------------------
    addBlockNote: async (_ws, _projectId, blockId, text): Promise<BlockNote> => ({
      id: `note-${Date.now()}`,
      blockId,
      author: "translator@example.com",
      text,
      createdAt: new Date().toISOString(),
    }),
    listBlockNotes: async (): Promise<BlockNote[]> => sampleBlockNotes,
    deleteBlockNote: async () => {},

    // --- Block history ---------------------------------------------------
    getBlockHistory: async (): Promise<BlockHistoryEntry[]> => sampleBlockHistory,

    // --- Rollback / restore (#778) ---------------------------------------
    rollbackBlock: async () => {},
    revertBatch: async () => ({ reverted: 0 }),
    restoreToPoint: async () => ({ restored: 0 }),
    setBlockStatus: async () => {},

    // --- Per-locale review (Target.Status ladder) -------------------------
    reviewBlock: async (
      workspaceSlug,
      projectId,
      itemName,
      blockId,
      targetLocale,
      reviewed,
      stream,
      demoteTo,
    ) => {
      reviewBlockCalls.push({
        workspaceSlug,
        projectId,
        itemName,
        blockId,
        targetLocale,
        reviewed,
        stream,
        demoteTo,
      });
      if (adapter.failReviewBlock) throw new Error("reviewBlock failed (mock)");
      const blk = _blocks.find((b) => b.id === blockId);
      if (blk) {
        const entry = blk.targets[targetLocale];
        const text = typeof entry === "string" ? entry : (entry?.text ?? "");
        blk.targets[targetLocale] = {
          text,
          // A rejection (reviewed=false + demoteTo "draft") lands on draft;
          // a plain un-review on translated (mirrors HandleReviewBlock).
          status: reviewed ? "reviewed" : demoteTo === "draft" ? "draft" : "translated",
        };
      }
    },

    // --- Governance (#778) -----------------------------------------------
    listGroups: async () => [],
    createGroup: async (_ws: string, name: string) => ({
      id: "g1",
      workspace_id: "ws",
      name,
      description: "",
      created_at: new Date(0).toISOString(),
    }),
    deleteGroup: async () => {},
    listGroupMembers: async () => [],
    addGroupMember: async () => {},
    removeGroupMember: async () => {},
    listGroupBindings: async () => [],
    addGroupBinding: async (_ws: string, groupId: string, projectId: string, roleId: string) => ({
      id: "b1",
      group_id: groupId,
      workspace_id: "ws",
      project_id: projectId,
      role_id: roleId,
      languages: [],
      created_at: new Date(0).toISOString(),
    }),
    removeGroupBinding: async () => {},
    listDenyRules: async () => [],
    createDenyRule: async (_ws: string, rule) => ({
      id: "d1",
      workspace_id: "ws",
      subject_type: rule.subject_type,
      subject_id: rule.subject_id,
      project_id: rule.project_id ?? "",
      denied_perms: 0,
      reason: rule.reason ?? "",
      created_at: new Date(0).toISOString(),
    }),
    deleteDenyRule: async () => {},
    getSoDMode: async () => ({ mode: "warn" as const }),
    setSoDMode: async () => {},
    listRoleOverrides: async () => ({}),
    setRoleOverride: async () => {},
    demoteBrandRule: async () => {},

    // --- QA --------------------------------------------------------------
    runQACheck: async (): Promise<QAIssue[]> => sampleQAIssues,
    runFileQACheck: async (): Promise<FileQAResult[]> => sampleFileQAResults,

    // --- Preview ---------------------------------------------------------
    renderDocumentPreview: async (): Promise<string> => generatePreviewHTML(_blocks),
    renderBlockHTML: async (_ws, _projectId, _blockId): Promise<string> =>
      "<span>rendered block</span>",

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

    // --- Automations -----------------------------------------------------
    listAutomationRules: async (): Promise<AutomationRule[]> => [..._automationRules],
    createAutomationRule: async (
      _ws: string,
      _pid: string,
      data: SaveAutomationRuleRequest,
    ): Promise<AutomationRule> => {
      const rule: AutomationRule = {
        id: `rule-${Date.now()}`,
        project_id: "proj-demo-1",
        ...data,
        builtin: false,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      _automationRules.push(rule);
      return rule;
    },
    updateAutomationRule: async (
      _ws: string,
      _pid: string,
      ruleId: string,
      data: SaveAutomationRuleRequest,
    ): Promise<AutomationRule> => {
      const idx = _automationRules.findIndex((r) => r.id === ruleId);
      if (idx >= 0) {
        _automationRules[idx] = {
          ..._automationRules[idx],
          ...data,
          updated_at: new Date().toISOString(),
        };
        return _automationRules[idx];
      }
      throw new Error("Rule not found");
    },
    deleteAutomationRule: async (_ws: string, _pid: string, ruleId: string): Promise<void> => {
      const idx = _automationRules.findIndex((r) => r.id === ruleId);
      if (idx >= 0) _automationRules.splice(idx, 1);
    },
    toggleAutomationRule: async (
      _ws: string,
      _pid: string,
      ruleId: string,
    ): Promise<AutomationRule> => {
      const rule = _automationRules.find((r) => r.id === ruleId);
      if (rule) {
        rule.enabled = !rule.enabled;
        rule.updated_at = new Date().toISOString();
        return rule;
      }
      throw new Error("Rule not found");
    },
    listAutomationEvents: async (): Promise<AutomationEvent[]> => sampleAutomationEvents,
    listAutomationHistory: async (): Promise<AutomationHistoryEntry[]> => sampleAutomationHistory,

    // --- Automation Runs ------------------------------------------------
    listAutomationRuns: async () => [],
    getAutomationRun: async () => ({ run: {} as any, steps: [] }),
    listStepLogs: async () => [],
    cancelAutomationRun: async () => {},

    // --- Flow definitions -----------------------------------------------
    listFlowDefinitions: async () => [
      {
        id: "translate",
        name: "AI Translate",
        description: "Translate content using AI/LLM",
        source: "built-in",
        nodes: [
          {
            id: "reader",
            type: "reader",
            name: "auto",
            position: { x: 0, y: 100 },
          },
          {
            id: "translate",
            type: "tool",
            name: "translate",
            position: { x: 250, y: 100 },
          },
          {
            id: "writer",
            type: "writer",
            name: "auto",
            position: { x: 500, y: 100 },
          },
        ],
        edges: [
          { id: "e1", source: "reader", target: "translate" },
          { id: "e2", source: "translate", target: "writer" },
        ],
      },
    ],
    getFlowDefinition: async (_ws, _pid, flowId) => ({
      id: flowId,
      name: flowId,
      source: "project",
      nodes: [],
      edges: [],
    }),
    createFlowDefinition: async (_ws, _pid, def) => def,
    updateFlowDefinition: async (_ws, _pid, _flowId, def) => def,
    deleteFlowDefinition: async () => {},

    // --- Providers ------------------------------------------------------
    listProviderConfigs: async () => [
      {
        id: "prov-1",
        name: "Claude",
        provider_type: "anthropic",
        model: "claude-sonnet-4-20250514",
        base_url: "",
      },
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

    // --- Notifications ---------------------------------------------------
    listNotifications: async () => ({ notifications: [], unread_count: 0 }),
    markNotificationRead: noop,
    markAllNotificationsRead: noop,
    deleteNotification: noop,

    // --- Digest Settings ---------------------------------------------------
    getDigestSettings: async () => ({
      frequency: "daily" as const,
      quiet_start: "",
      quiet_end: "",
      timezone: "UTC",
    }),
    updateDigestSettings: async (_ws, settings) => settings,

    // --- Entities ---------------------------------------------------------
    createEntity: async (_ws, _pid, _item, _bid, entity) => ({
      key: `entity-${Date.now()}`,
      text: "",
      type: "generic",
      start: 0,
      end: 0,
      dnt: false,
      ...entity,
    }),
    updateEntity: async (_ws, _pid, _item, _bid, entityKey, entity) => ({
      key: entityKey,
      text: "",
      type: "generic",
      start: 0,
      end: 0,
      dnt: false,
      ...entity,
    }),
    deleteEntity: noop,
    promoteEntity: noop,
    listStreams: async () => [],
    createStream: async () => {
      throw new Error("Not implemented");
    },
    getStream: async () => {
      throw new Error("Not implemented");
    },
    updateStream: async () => ({
      name: "main",
      parent: "",
      base_cursor: 0,
      archived: false,
      locked: false,
      visibility: "public" as const,
      description: "",
      created_at: "",
      created_by: "",
    }),
    deleteStream: noop,
    diffStream: async () => ({ stream_name: "", parent_name: "", changes: [] }),
    mergeStream: async () => ({
      merged_blocks: 0,
      added_blocks: 0,
      modified_blocks: 0,
      removed_blocks: 0,
    }),
    lockStream: async () => ({
      name: "main",
      parent: "",
      base_cursor: 0,
      archived: false,
      locked: true,
      visibility: "public" as const,
      description: "",
      created_at: "",
      created_by: "",
    }),
    unlockStream: async () => ({
      name: "main",
      parent: "",
      base_cursor: 0,
      archived: false,
      locked: false,
      visibility: "public" as const,
      description: "",
      created_at: "",
      created_by: "",
    }),
    listStreamTags: async () => [],
    createStreamTag: notImpl,
    getStreamTag: notImpl,
    deleteStreamTag: noop,
    listProjectTags: async () => [],

    // --- Brand Voice -------------------------------------------------------
    listBrandProfiles: async () => [],
    getBrandProfile: notImpl,
    createBrandProfile: notImpl,
    updateBrandProfile: notImpl,
    deleteBrandProfile: noop,
    getBrandScores: async () => [],
    getBrandTrends: async () => [],
    listBrandCandidates: async () => [],
    promoteBrandRule: async () => ({ promoted: true }),
    rejectBrandRule: noop,
    evaluateBrandRule: async () => ({
      total_blocks: 0,
      affected_blocks: 0,
      improved_blocks: 0,
      degraded_blocks: 0,
      new_violations: 0,
      resolved_violations: 0,
      critical_count: 0,
      collections: [],
    }),
    getBrandDrift: async () => ({
      drifted: false,
      recent_avg: 0,
      baseline_avg: 0,
      drop: 0,
      recent_days: 7,
      recent_count: 0,
    }),
    listStarterPacks: async () => [
      {
        name: "professional-b2b",
        description: "Formal, authoritative voice for B2B",
      },
      {
        name: "friendly-dtc",
        description: "Casual, warm voice for DTC brands",
      },
      { name: "marketing-blog", description: "Conversational voice for blogs" },
      { name: "customer-support", description: "Empathetic voice for support" },
      {
        name: "technical-docs",
        description: "Precise voice for documentation",
      },
    ],
    createProfileFromStarter: notImpl,

    // --- Brand scan (epic 016) -------------------------------------------
    uploadBrandScanSources: async (_ws, files): Promise<BrandScanUploadResult> => {
      uploadBrandScanSourcesCalls.push(files.map((f) => f.name));
      const uploads: BrandScanUploadResult["uploads"] = [];
      const skipped: SkippedFile[] = [];
      for (const f of files) {
        const ext = f.name.split(".").pop()?.toLowerCase() ?? "";
        if (BRAND_SCAN_DEFERRED_EXTENSIONS.has(ext)) {
          skipped.push({
            name: f.name,
            reason: "unsupported (deferred: needs pdf/pptx text extractor)",
          });
        } else if (!BRAND_SCAN_KNOWN_EXTENSIONS.has(ext)) {
          skipped.push({ name: f.name, reason: `no text extractor for ".${ext}"` });
        } else {
          uploads.push({ key: `blob-${f.name}`, filename: f.name, size: f.size });
        }
      }
      return skipped.length > 0 ? { uploads, skipped } : { uploads };
    },
    startBrandScan: async (_ws, req) => {
      startBrandScanCalls.push(req);
      brandScanPollIndex = 0;
      return { job_id: sampleBrandScanJob.id };
    },
    getBrandScan: async (_ws, jobId): Promise<BrandScanJob> => {
      const states = adapter.brandScanJobStates;
      const state = states[Math.min(brandScanPollIndex, states.length - 1)];
      brandScanPollIndex++;
      return { ...state, id: jobId };
    },
    checkBrandDraft: async (_ws, profile, text): Promise<BrandScanCheckResult> => {
      checkBrandDraftCalls.push({ profileName: profile.name, text });
      // Deterministic: forbidden/competitor terms in the sample text lower
      // the score and surface findings, mirroring the vocabulary matcher.
      const rules = [
        ...(profile.vocabulary.forbidden_terms ?? []),
        ...(profile.vocabulary.competitor_terms ?? []),
      ];
      const findings = rules
        .filter((r) => text.toLowerCase().includes(r.term.toLowerCase()))
        .map((r) => ({
          // Server shape: core/check.Finding serializes the grouping field as
          // `category` (NOT the UI's `dimension`) — the live tester normalizes.
          category: "vocabulary",
          severity: r.severity ?? "major",
          message: `Uses the term "${r.term}"`,
          position: { start: 0, end: r.term.length },
          original_text: r.term,
        }));
      return { score: { overall: findings.length > 0 ? 62 : 96 }, findings };
    },
    getTranslationDashboard: async () => ({
      locale_stats: [],
      item_stats: [],
      collection_stats: [],
      total_blocks: 0,
      translatable_blocks: 0,
      total_source_words: 0,
    }),

    // --- Activities (Bowrain AD-014) ------------------------------------------------
    listActivities: async () => ({
      activities: [],
      next_cursor: "",
      new_count: 0,
    }),
    markActivitiesSeen: async () => {},

    // --- Convergence runs (Bowrain AD-022) ------------------------------------------
    listConvergenceRuns: async () => [],
    getConvergenceRun: async (_ws, projectId, runId) => ({
      id: runId,
      project_id: projectId,
      trigger: "manual",
      state: "converged" as const,
      passes: 1,
      locales: [],
      failing_checks: 0,
    }),
    startConvergenceRun: async (_ws, projectId) => ({
      id: `run-${Date.now()}`,
      project_id: projectId,
      trigger: "manual",
      state: "running" as const,
      passes: 0,
      locales: [],
      failing_checks: 0,
      created_at: new Date().toISOString(),
    }),
    cancelConvergenceRun: async () => {},

    // --- Tasks (Bowrain AD-014) -----------------------------------------------------
    listTasks: async () => ({ tasks: [], next_cursor: "" }),
    createTask: async (_ws, task) => ({
      id: `task-${Date.now()}`,
      workspace_id: "ws-1",
      project_id: task.project_id,
      type: task.type ?? "custom",
      status: "open" as const,
      priority: task.priority ?? "normal",
      title: task.title,
      description: task.description ?? "",
      assignee_id: task.assignee_id ?? "",
      created_by: "user-1",
      completed_by: "",
      data: {},
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }),
    getTask: notImpl,
    updateTask: notImpl,
    deleteTask: noop,
    assignTask: noop,
    completeTask: noop,
    cancelTask: noop,
    listMyTasks: async () => ({ tasks: [], next_cursor: "" }),

    // --- Notification Preferences (Bowrain AD-014) ----------------------------------
    getNotificationPreferences: async () => ({ preferences: [] }),
    updateNotificationPreferences: noop,

    // --- @bravo Agent (Bowrain AD-016) ----------------------------------------------
    bravoCreateConversation: async () => ({
      id: "conv-mock",
      workspace_id: "ws-1",
      user_id: "user-1",
      project_id: "",
      title: "New conversation",
      status: "active" as const,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    }),
    bravoListConversations: async () => ({ conversations: [], total: 0 }),
    bravoGetConversation: async () => ({
      conversation: {
        id: "conv-mock",
        workspace_id: "ws-1",
        user_id: "user-1",
        project_id: "",
        title: "Mock",
        status: "active" as const,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
      messages: [],
    }),
    bravoDeleteConversation: noop,
    bravoSendMessage: async () => ({
      user_message: {
        id: "msg-u",
        conversation_id: "conv-mock",
        role: "user" as const,
        content: "hello",
        created_at: new Date().toISOString(),
      },
      assistant_message: {
        id: "msg-a",
        conversation_id: "conv-mock",
        role: "assistant" as const,
        content: "Mock response",
        created_at: new Date().toISOString(),
      },
    }),
    bravoListMessages: async () => ({ messages: [] }),
    bravoApproveToolCall: noop,
    bravoDenyToolCall: noop,
    bravoCancelConversation: noop,
    bravoGetConfig: async () => ({
      workspace_id: "ws-1",
      enabled: false,
      code_exec_enabled: false,
      max_concurrent: 3,
    }),
    bravoUpdateConfig: async () => ({
      workspace_id: "ws-1",
      enabled: false,
      code_exec_enabled: false,
      max_concurrent: 3,
    }),
    bravoListTools: async () => ({ tools: [] }),
    bravoGetUsage: async () => ({
      workspace_id: "ws-1",
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_container_sec: 0,
      message_count: 0,
    }),
    bravoUpdateMode: async (_ws: string, _id: string, mode: string) => ({
      mode,
      permissions: ["view_content"],
    }),
    bravoSendMessageSSE: () => new AbortController(),

    // --- Billing (Bowrain AD-018) ---------------------------------------------------
    billingGetOverview: async () => ({
      subscription: {
        plan: "pro" as const,
        status: "active" as const,
        seatCount: 3,
        currentPeriodStart: new Date().toISOString(),
        currentPeriodEnd: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(),
      },
      credits: {
        creditsTotal: 500_000,
        creditsUsed: 123_000,
        weekStart: new Date().toISOString(),
        weekEnd: new Date(Date.now() + 4 * 24 * 60 * 60 * 1000).toISOString(),
        source: "plan",
      },
    }),
    billingGetUsage: async () => ({
      aiTranslation: 80_000,
      aiQualityCheck: 15_000,
      bravoMessages: 25_000,
      bravoContainer: 3_000,
      total: 123_000,
    }),
    billingGetModelUsage: async () => ({
      model_usage: [
        {
          model: "claude-sonnet-4-20250514",
          operation: "translate",
          prompt_tokens: 60_000,
          output_tokens: 20_000,
          total_tokens: 80_000,
          call_count: 150,
        },
        {
          model: "gpt-4o",
          operation: "translate",
          prompt_tokens: 10_000,
          output_tokens: 5_000,
          total_tokens: 15_000,
          call_count: 30,
        },
        {
          model: "claude-sonnet-4-20250514",
          operation: "qa_check",
          prompt_tokens: 12_000,
          output_tokens: 3_000,
          total_tokens: 15_000,
          call_count: 25,
        },
      ],
      runner_usage: [
        { operation: "bravo_container", total_seconds: 1_842, count: 47 },
        { operation: "auto_translate", total_seconds: 623, count: 12 },
      ],
      from: new Date(Date.now() - 7 * 86400_000).toISOString(),
      to: new Date().toISOString(),
    }),
    billingCreateCheckout: async () => ({ url: "#" }),
    billingCreatePortal: async () => ({ url: "#" }),
    billingGetLedger: async () => [],
  };
  return adapter;
}
