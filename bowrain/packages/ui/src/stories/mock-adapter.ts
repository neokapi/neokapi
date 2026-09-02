/**
 * In-memory ApiAdapter for Storybook.
 *
 * Returns realistic fixture data for editor-related endpoints.
 * Mutations are applied to a mutable blocks array so the editor
 * feels interactive without needing a real server.
 */

import type { ApiAdapter } from "../api/adapter";
import { getBlockStatus, getTargetText } from "../components/editor/blockStatus";
import type {
  ApprovePassingRequest,
  ApprovePassingResult,
  SourceProposal,
  CreateSourceProposalRequest,
  BlockInfo,
  ReviewDemotion,
  SkippedFile,
  TranslationStats,
  UploadFilesResult,
  WordCountResult,
  MemoryMatchInfo,
  BlockTermMatch,
  ReviewContext,
  ReviewVoiceProfile,
  BlockNote,
  BlockHistoryEntry,
  QAIssue,
  FileQAResult,
  ContextScanRequest,
  ContextScanUploadResult,
  ContextScanCheckResult,
  ContextScanDraft,
  ContextScanJob,
  ModelRecommendationsResponse,
  BlockQueryOptions,
  BlockStatusCounts,
  AutomationHistoryPage,
  ContextScanApproveResult,
  ApproveAxisRequest,
  PendingRecipeChange,
  CollectionInfo,
  PendingReviewOptions,
  TermCompliance,
} from "../types/api";
import type { VoiceProfile, VoiceCorrectionRequest } from "../voice/types";
import type {
  ChangeSet,
  ChangeSetImpact,
  ChangeSetStatus,
  Market,
  MarketRequest,
} from "../types/brand-graph";
import type {
  AutomationRule,
  AutomationEvent,
  AutomationHistoryEntry,
  SaveAutomationRuleRequest,
} from "../types/api";
import { ALL_PERMISSIONS } from "../types/api";
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
  sampleReviewVoiceProfile,
} from "./fixtures";

/**
 * The block filters the server applies, run over the fixture blocks: a
 * locale's status bucket, a case-insensitive substring over source and that
 * locale's target, and translatability. Paging is the caller's slice.
 */
function queryBlocks(blocks: BlockInfo[], opts?: BlockQueryOptions): BlockInfo[] {
  const needle = opts?.q?.toLowerCase();
  return blocks.filter((b) => {
    if (opts?.translatable !== undefined && b.translatable !== opts.translatable) return false;
    if (opts?.status && opts.locale && getBlockStatus(b, opts.locale) !== opts.status) return false;
    if (needle) {
      const target = opts?.locale ? getTargetText(b, opts.locale) : "";
      if (!b.source.toLowerCase().includes(needle) && !target.toLowerCase().includes(needle)) {
        return false;
      }
    }
    return true;
  });
}

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
  /** `approvePassingReview` invocations in call order. */
  approvePassingReviewCalls: ApprovePassingRequest[];
  /** Overrides the computed `approvePassingReview` result when set. */
  approvePassingResult?: ApprovePassingResult;
  /** `createSourceProposal` invocations in call order. */
  createSourceProposalCalls: CreateSourceProposalRequest[];
  /** `decideSourceProposal` invocations in call order. */
  decideSourceProposalCalls: { proposalId: string; decision: string; reason?: string }[];
  /** `promoteEntityToConcept` invocations in call order. */
  promoteEntityToConceptCalls: { itemName: string; blockId: string; entityKey: string }[];
  /** Backing store for source proposals (seed to preload the review surface). */
  sourceProposals: SourceProposal[];
  /** `recordVoiceCorrection` invocations in call order. */
  recordVoiceCorrectionCalls: VoiceCorrectionRequest[];
  /** `startContextScan` invocations in call order. */
  startContextScanCalls: ContextScanRequest[];
  /** `uploadContextScanSources` invocations — filenames per call. */
  uploadContextScanSourcesCalls: string[][];
  /** `checkVoiceDraft` invocations in call order. */
  checkVoiceDraftCalls: { profileName: string; text: string }[];
  /** `approveAxis` invocations in call order. */
  approveAxisCalls: ApproveAxisRequest[];
  /**
   * States returned by successive `getContextScan` calls: each call consumes
   * the next entry; the last entry repeats. Tests overwrite this to simulate
   * queued → processing → completed/failed progressions.
   */
  contextScanJobStates: ContextScanJob[];
  /**
   * Which item each block belongs to (block id → item name). A block absent
   * from the map is in "messages.json", the single-item default.
   */
  itemNames: Record<string, string>;
  /**
   * Which collection each item belongs to (item name → collection id), the
   * pairing the server makes by joining items. An item absent from the map is
   * in no collection, the "" scope.
   */
  itemCollections: Record<string, string>;
  /** `getPendingReview` option objects in call order. */
  pendingReviewCalls: (PendingReviewOptions | undefined)[];
  /**
   * Per-block terminology and voice evidence the review queue carries (block id
   * → the fields the server stamps). A block absent from the map has no
   * terminology governance applied and has never been scored.
   */
  blockEvidence: Record<
    string,
    { term_compliance?: TermCompliance; voice_score?: number; voice_bar?: number }
  >;
}

// ---------------------------------------------------------------------------
// Measured-steerability fixtures (deterministic — stories and tests assert on
// these): two candidate models on fr, the lift winner marked recommended.
// ---------------------------------------------------------------------------

/** Deterministic model recommendation results for stories and tests. */
export const sampleModelRecommendations: ModelRecommendationsResponse = {
  enabled: true,
  locales: [
    {
      locale: "fr",
      recommended_model: "claude-sonnet",
      models: [
        {
          model: "claude-sonnet",
          adherence: 0.92,
          adherence_bare: 0.54,
          lift: 0.38,
          fixture_count: 14,
          tokens_used: 2140,
          measured_at: "2026-07-15T08:00:00Z",
          recommended: true,
        },
        {
          model: "claude-haiku",
          adherence: 0.79,
          adherence_bare: 0.57,
          lift: 0.22,
          fixture_count: 14,
          tokens_used: 1660,
          measured_at: "2026-07-15T08:00:00Z",
          recommended: false,
        },
      ],
    },
  ],
};

// ---------------------------------------------------------------------------
// Brand-scan fixtures (deterministic — stories and tests assert on these)
// ---------------------------------------------------------------------------

/** Deterministic drafted profile for the brand-scan review fixtures. */
export const sampleContextScanProfile: VoiceProfile = {
  id: "",
  name: "Acme Voice",
  description: "Drafted from 3 sources by the context scan.",
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

/**
 * Deterministic completed scan: a voice and a vocabulary, both proposed at the
 * default point — the empty `at` a single-corpus scan produces.
 */
export const sampleContextScanDraft: ContextScanDraft = {
  artefacts: [
    {
      kind: "voice",
      voice: sampleContextScanProfile,
      evidence: {
        fields: {
          tone: { confidence: 0.82, source: "Consistent register across brand-guide.docx" },
          style: { confidence: 0.71, source: "Short imperative sentences on the landing pages" },
          vocabulary: { confidence: 0.9, source: "Terminology section in brand-guide.docx" },
          examples: { confidence: 0.55, source: "Rewrites derived from the blog posts" },
        },
      },
    },
    {
      kind: "terms",
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
    },
  ],
  // One structural axis and one declared axis, because the two are approved
  // differently: `product` is derived from a collection's channel and needs one
  // named, `audience` sits on the project's default point and refuses a
  // collection.
  axes: [
    {
      axis: "product",
      values: ["kapi", "bowrain"],
      evidence: ["Two distinct product names across brand-guide.docx and the about page"],
      confidence: 0.84,
    },
    {
      axis: "audience",
      values: ["developer", "buyer"],
      evidence: ["Reference pages address a developer; the landing pages address a buyer"],
      confidence: 0.61,
    },
  ],
  sources: [
    { kind: "upload", label: "brand-guide.docx", runes: 48210 },
    { kind: "url", label: "https://acme.example/about", runes: 9120 },
    { kind: "paste", label: "pasted text", runes: 1834 },
  ],
  truncated: false,
};

/** One collection, so a structural axis has somewhere to be approved for. */
export const sampleCollection: CollectionInfo = {
  id: "col-docs",
  project_id: "proj-1",
  name: "docs",
  kind: "uploaded",
  item_label: "file",
  is_default: false,
  item_count: 12,
  created_at: "2026-08-01T09:00:00Z",
  updated_at: "2026-08-01T09:00:00Z",
};

/** A completed brand-scan job carrying the sample draft. */
export const sampleContextScanJob: ContextScanJob = {
  id: "scan-1",
  status: "completed",
  progress: 100,
  message: "done",
  tokens_used: 12840,
  draft: sampleContextScanDraft,
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

/**
 * A fixed timestamp. The brand-graph endpoints below stamp records they invent,
 * and a story that renders a relative time must not re-render differently on
 * every reload.
 */
const MOCK_NOW = "2026-01-01T00:00:00Z";

/** The shape every change-set endpoint returns, at whatever status it returns. */
function mockChangeset(id: string, status: ChangeSetStatus): ChangeSet {
  return {
    id,
    workspace_id: "ws-1",
    name: "Mock change-set",
    status,
    created_by: "storybook",
    created_at: MOCK_NOW,
    updated_at: MOCK_NOW,
  };
}

/** A market as the create/update endpoints echo one back. */
function mockMarket(id: string, req: MarketRequest): Market {
  return {
    id,
    workspace_id: "ws-1",
    name: req.name,
    description: req.description,
    locales: req.locales ?? [],
    created_at: MOCK_NOW,
    updated_at: MOCK_NOW,
  };
}

/** An empty blast radius — nothing affected, nothing to inspect. */
function mockImpact(): ChangeSetImpact {
  return {
    total_blocks: 0,
    affected_blocks: 0,
    new_violations: 0,
    resolved: 0,
    words: 0,
    projects: [],
    samples: [],
  };
}

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
  const approvePassingReviewCalls: ApprovePassingRequest[] = [];
  const createSourceProposalCalls: CreateSourceProposalRequest[] = [];
  const decideSourceProposalCalls: { proposalId: string; decision: string; reason?: string }[] = [];
  const promoteEntityToConceptCalls: { itemName: string; blockId: string; entityKey: string }[] =
    [];
  const recordVoiceCorrectionCalls: VoiceCorrectionRequest[] = [];
  const startContextScanCalls: ContextScanRequest[] = [];
  const uploadContextScanSourcesCalls: string[][] = [];
  const checkVoiceDraftCalls: { profileName: string; text: string }[] = [];
  const approveAxisCalls: ApproveAxisRequest[] = [];
  const pendingReviewCalls: (PendingReviewOptions | undefined)[] = [];
  let contextScanPollIndex = 0;

  const adapter: MockAdapter = {
    // --- Test hooks -------------------------------------------------------
    reviewBlockCalls,
    failReviewBlock: false,
    approvePassingReviewCalls,
    createSourceProposalCalls,
    decideSourceProposalCalls,
    promoteEntityToConceptCalls,
    sourceProposals: [],
    recordVoiceCorrectionCalls,
    startContextScanCalls,
    uploadContextScanSourcesCalls,
    checkVoiceDraftCalls,
    approveAxisCalls,
    contextScanJobStates: [sampleContextScanJob],
    itemNames: {},
    itemCollections: {},
    blockEvidence: {},
    pendingReviewCalls,

    // --- Config ---------------------------------------------------------
    getConfig: async () => ({
      mode: "standalone",
      version: "0.0.0-storybook",
      commit: "storybook",
      build_date: "unknown",
      // The mock backend answers every brand-scan call, so the capability is on.
      features: { context_scan: true },
    }),
    getPublicPlatformConfig: async () => ({
      signups_open: true,
      maintenance: { enabled: false },
      ai_customer_choice: false,
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
    // Stories exercise the full surface, so the mock caller holds every
    // permission unless a story overrides it.
    getCallerPermissions: async () => ({ permissions: [...ALL_PERMISSIONS], languages: [] }),
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
    listCollections: async () => [sampleCollection],
    createCollection: notImpl,
    getCollection: notImpl,
    updateCollection: notImpl,
    deleteCollection: noop,
    uploadToCollection: notImpl,

    // --- Connectors -----------------------------------------------------
    // The mock exposes no connectors, so the batch answers every id as one
    // it cannot resolve rather than inventing a status.
    getConnectorStatuses: async (_ws, ids) => ({ statuses: {}, unknown: [...ids] }),

    // --- Editor ---------------------------------------------------------
    getFileBlocks: async (_ws, _projectId, _fileName, _stream, opts) =>
      queryBlocks(_blocks, opts).slice(
        opts?.offset ?? 0,
        (opts?.offset ?? 0) + (opts?.limit ?? _blocks.length),
      ),

    getBlockCounts: async (_ws, _projectId, _item, locale, _stream, opts) => {
      const matching = queryBlocks(_blocks, { locale, ...opts });
      const translatable = matching.filter((b) => b.translatable !== false);
      const status: BlockStatusCounts = {
        "not-started": 0,
        draft: 0,
        translated: 0,
        reviewed: 0,
      };
      if (locale) for (const b of translatable) status[getBlockStatus(b, locale)]++;
      return { total: matching.length, translatable: translatable.length, locale, status };
    },

    getBlock: async (_ws, _projectId, blockId) => {
      const block = _blocks.find((b) => b.id === blockId);
      if (!block) throw new Error(`block not found: ${blockId}`);
      return block;
    },

    getItem: async (_ws, _projectId, itemName) => ({
      id: itemName,
      name: itemName,
      format: "json",
      type: "file",
      block_count: _blocks.length,
      translatable: _blocks.filter((b) => b.translatable !== false).length,
    }),

    bulkReviewBlocks: async (_ws, req) => {
      const results = req.block_ids.map((id) => {
        const blk = _blocks.find((b) => b.id === id);
        if (!blk) return { block_id: id, ok: false, error: "block not found" };
        const status = req.approve ? "reviewed" : (req.status ?? "translated");
        const entry = blk.targets[req.target_locale];
        blk.targets[req.target_locale] = {
          text: typeof entry === "string" ? entry : (entry?.text ?? ""),
          status,
        };
        return { block_id: id, ok: true, status };
      });
      const succeeded = results.filter((r) => r.ok).length;
      return {
        results,
        succeeded,
        failed: results.length - succeeded,
        review_completed: _blocks.every(
          (b) => getBlockStatus(b, req.target_locale) === "reviewed" || b.translatable === false,
        ),
      };
    },

    // The mock carries no content memory, so every selected block is skipped
    // for the reason the server gives when nothing clears the threshold.
    bulkApplyMemory: async (_ws, req) => ({
      applied: [],
      skipped: req.block_ids.map((id) => ({ block_id: id, reason: "no match above threshold" })),
    }),

    // The server-side review queue: every (block, locale) pair with target
    // text still below reviewed — the same predicate the server runs. The
    // collection scope is applied here, as the server applies it, so a caller
    // that passes one is answered its queue and its total.
    getPendingReview: async (_ws, _projectId, opts) => {
      pendingReviewCalls.push(opts);
      const entries = _blocks
        .flatMap((b) =>
          Object.entries(b.targets ?? {})
            .filter(([, t]) => {
              // A target entry is either bare text or {text, status}.
              const tt =
                typeof t === "string"
                  ? { text: t, status: "" }
                  : (t as { text?: string; status?: string });
              return (
                b.translatable !== false &&
                (tt.text ?? "").trim() !== "" &&
                ["draft", "translated", ""].includes(tt.status ?? "")
              );
            })
            .map(([loc]) => {
              const itemName = adapter.itemNames[b.id] ?? "messages.json";
              const evidence = adapter.blockEvidence[b.id];
              return {
                block_id: b.id,
                item_name: itemName,
                locale: loc,
                block: b,
                collection_id: adapter.itemCollections[itemName] ?? "",
                // The two bars beyond the checks the server judges on, absent unless a
                // test seeds them — the same shape the real payload carries.
                term_compliance: evidence?.term_compliance,
                voice_score: evidence?.voice_score,
                voice_bar: evidence?.voice_bar,
              };
            }),
        )
        .filter((e) => opts?.collectionId === undefined || e.collection_id === opts.collectionId);
      const offset = opts?.offset ?? 0;
      const limit = opts?.limit ?? 200;
      return {
        entries: entries.slice(offset, offset + limit),
        total: entries.length,
        limit,
        offset,
      };
    },

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

    memoryTranslateFile: async (): Promise<TranslationStats> => ({
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

    lookupMemoryForBlock: async (): Promise<MemoryMatchInfo[]> => [
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

    // The five layers of context one unit is decided in. The document surface
    // asks for it when a block is opened; the queue asks for the unit under the
    // cursor. `blk-3` is the ungoverned, undecided, unmatched block, so a story
    // can show the empty states without a second adapter.
    getReviewContext: async (
      _ws,
      _projectId,
      itemName,
      blockId,
      targetLocale,
    ): Promise<ReviewContext> =>
      blockId === "blk-3"
        ? {
            block_id: blockId,
            item_name: itemName,
            locale: targetLocale,
            terms: [],
            collection_id: "",
            notes: [],
            voice_findings: [],
          }
        : {
            block_id: blockId,
            item_name: itemName,
            locale: targetLocale,
            voice_profile: sampleReviewVoiceProfile,
            terms: [
              {
                source_term: "app",
                target_terms: ["application"],
                domain: "product",
                status: "preferred",
                start: 0,
                end: 3,
              },
            ],
            collection_id: "col-1",
            collection_name: "Marketing site",
            coordinates: { product: "kapi", channel: "web" },
            previous: { block_id: "blk-0", source_runs: [{ text: "Sign in to continue" }] },
            next: { block_id: "blk-2", source_runs: [{ text: "Click here to continue" }] },
            memory_match: {
              source: "Welcome to Neokapi",
              target: "Bienvenue sur Neokapi",
              score: 1,
              match_type: "exact",
            },
            decision: {
              state: "rejected",
              status: "draft",
              by: "maria@bowrain.test",
              at: "2026-08-30T09:12:00Z",
              note: "Reads as machine output; soften the imperative.",
            },
            notes: [
              {
                id: "note-1",
                blockId,
                author: "maria@bowrain.test",
                text: "Legal asked us to keep the product name unchanged.",
                createdAt: "2026-08-30T09:15:00Z",
              },
            ],
            voice_findings: [
              {
                category: "compliance",
                severity: "major",
                message: "Uses a term the profile forbids.",
                original_text: "Neokapi",
                suggestion: "the platform",
                position: {
                  kind: "range",
                  start: { run: 0, offset: 12 },
                  end: { run: 0, offset: 19 },
                },
              },
            ],
            voice_score: 62,
            voice_bar: 90,
            origin: {
              kind: "ai",
              engine: "claude-sonnet",
              tool: "translate",
              timestamp: "2026-08-29T18:40:00Z",
              profile: "vp-1",
            },
          },

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

    approvePassingReview: async (_ws, _projectId, req = {}) => {
      approvePassingReviewCalls.push(req);
      if (adapter.approvePassingResult) return adapter.approvePassingResult;
      // Default: promote every pending block passing checks (the mock runs none),
      // marking them reviewed so a re-read reflects the emptied queue.
      const locales = req.locales;
      let approved = 0;
      for (const blk of _blocks) {
        if (!blk.translatable) continue;
        for (const [loc, entry] of Object.entries(blk.targets)) {
          if (locales && !locales.includes(loc)) continue;
          const text = typeof entry === "string" ? entry : (entry?.text ?? "");
          const status = typeof entry === "string" ? "" : (entry?.status ?? "");
          if (text.trim() && status !== "reviewed" && status !== "signed-off") {
            blk.targets[loc] = { text, status: "reviewed" };
            approved++;
          }
        }
      }
      return {
        approved,
        skipped: 0,
        skipped_failing_checks: 0,
        skipped_term_violations: 0,
        skipped_below_voice_bar: 0,
        skipped_self_authored: 0,
        remaining_pending: 0,
        review_completed: true,
      };
    },

    recordVoiceCorrection: async (_ws, _projectId, req) => {
      recordVoiceCorrectionCalls.push(req);
      return { auto_promoted: false };
    },

    createSourceProposal: async (_ws, projectId, req) => {
      createSourceProposalCalls.push(req);
      const now = new Date(0).toISOString();
      const proposal: SourceProposal = {
        id: `sp-${createSourceProposalCalls.length}`,
        workspace_id: "ws",
        project_id: projectId,
        stream: req.stream,
        item_name: req.item_name,
        block_id: req.block_id,
        kind: req.kind ?? "text-fix",
        original_source: "",
        proposed_source: req.proposed_source,
        rationale: req.rationale,
        found_in_locale: req.found_in_locale,
        status: "open",
        created_at: now,
        updated_at: now,
      };
      adapter.sourceProposals.push(proposal);
      return proposal;
    },
    listSourceProposals: async () => adapter.sourceProposals.filter((p) => p.status === "open"),
    decideSourceProposal: async (_ws, _projectId, proposalId, decision, reason) => {
      decideSourceProposalCalls.push({ proposalId, decision, reason });
      const p = adapter.sourceProposals.find((x) => x.id === proposalId);
      if (p) p.status = decision === "approve" ? "approved" : "rejected";
      return {
        ok: true,
        status: p?.status ?? decision,
        applied: decision === "approve",
        run_started: decision === "approve",
      };
    },
    promoteEntityToConcept: async (_ws, _projectId, itemName, blockId, entityKey) => {
      promoteEntityToConceptCalls.push({ itemName, blockId, entityKey });
      const now = new Date(0).toISOString();
      return {
        ok: true,
        concept: {
          id: `concept-${promoteEntityToConceptCalls.length}`,
          domain: "",
          definition: "",
          terms: [],
          created_at: now,
          updated_at: now,
        },
      };
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
    demoteVoiceRule: async () => {},

    // --- Checks ----------------------------------------------------------
    runQACheck: async (): Promise<QAIssue[]> => sampleQAIssues,
    runFileQACheck: async (): Promise<FileQAResult[]> => sampleFileQAResults,

    // --- Preview ---------------------------------------------------------
    renderDocumentPreview: async (): Promise<string> => generatePreviewHTML(_blocks),
    renderBlockHTML: async (_ws, _projectId, _blockId): Promise<string> =>
      "<span>rendered block</span>",

    // --- content memory -------------------------------------------------------------
    getMemoryEntries: async () => ({ entries: [], total_count: 0 }),
    getMemoryCount: async () => 0,
    addMemoryEntry: notImpl,
    updateMemoryEntry: noop,
    deleteMemoryEntry: noop,
    bulkDeleteMemoryEntries: async (_ws, ids) => ({
      results: ids.map((id) => ({ id, deleted: true })),
      deleted: ids.length,
      failed: 0,
    }),

    // --- Terms ----------------------------------------------------------
    getTerms: async () => ({ concepts: [], total_count: 0 }),
    getTermCount: async () => 0,
    addConcept: notImpl,
    updateConcept: noop,
    deleteConcept: noop,
    // Deleting a concept is governed, so the batch is refused the way the
    // server refuses it.
    bulkDeleteConcepts: async () => {
      throw new Error("governed change requires a change-set");
    },
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
    listAutomationHistory: async (_ws, _projectId, opts): Promise<AutomationHistoryPage> => {
      const limit = opts?.limit ?? sampleAutomationHistory.length;
      const start = opts?.cursor
        ? sampleAutomationHistory.findIndex((e) => e.id === opts.cursor) + 1
        : 0;
      const entries = sampleAutomationHistory.slice(start, start + limit);
      const last = entries[entries.length - 1];
      const more = last ? start + entries.length < sampleAutomationHistory.length : false;
      return more && last ? { entries, next_cursor: last.id } : { entries };
    },

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

    // --- Voice -------------------------------------------------------
    listVoiceProfiles: async () => [],
    getVoiceProfile: notImpl,
    createVoiceProfile: notImpl,
    updateVoiceProfile: notImpl,
    deleteVoiceProfile: noop,
    getVoiceScores: async () => [],
    getVoiceTrends: async () => [],
    getVoiceRollup: async () => ({ projects: [], total: 0, limit: 50, offset: 0 }),
    getLoopRollup: async () => ({}),
    listVoiceCandidates: async () => [],
    promoteVoiceRule: async () => ({ promoted: true }),
    rejectVoiceRule: noop,
    evaluateVoiceRule: async () => ({
      total_blocks: 0,
      affected_blocks: 0,
      improved_blocks: 0,
      degraded_blocks: 0,
      new_violations: 0,
      resolved_violations: 0,
      critical_count: 0,
      collections: [],
    }),
    getVoiceDrift: async () => ({
      drifted: false,
      recent_avg: 0,
      baseline_avg: 0,
      drop: 0,
      recent_days: 7,
      recent_count: 0,
    }),
    createProfileFromStarter: notImpl,

    // --- Context scan (epic 016) -------------------------------------------
    uploadContextScanSources: async (_ws, files): Promise<ContextScanUploadResult> => {
      uploadContextScanSourcesCalls.push(files.map((f) => f.name));
      const uploads: ContextScanUploadResult["uploads"] = [];
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
    startContextScan: async (_ws, req) => {
      startContextScanCalls.push(req);
      contextScanPollIndex = 0;
      return { job_id: sampleContextScanJob.id };
    },
    approveContextScan: async (_ws, _scanId, req): Promise<ContextScanApproveResult> => ({
      profile: req.profile,
      profile_action: "created",
      concepts_created: req.terms?.length ?? 0,
      concepts_existing: 0,
      concept_ids: (req.terms ?? []).map((t) => `concept-${t.term}`),
    }),
    approveAxis: async (_ws, projectId, req): Promise<PendingRecipeChange> => {
      approveAxisCalls.push(req);
      // The server refuses a structural axis with no collection, so the mock
      // does too — a story that forgets it should see the message a reviewer
      // would, not a success.
      const structural = req.axis === "product" || req.axis === "channel";
      if (structural && !req.collection) {
        throw new Error(
          `${req.axis} is derived from a collection's channel, not declared: name the collection this applies to`,
        );
      }
      return {
        id: `change-${approveAxisCalls.length}`,
        workspace_id: "ws-1",
        project_id: projectId,
        path: structural
          ? `collections.${req.collection}.channel`
          : `defaults.coordinates.${req.axis}`,
        value: structural ? `${req.value}/web` : req.value,
        status: "pending",
        created_at: "2026-08-22T09:00:00Z",
      };
    },
    getContextScan: async (_ws, jobId): Promise<ContextScanJob> => {
      const states = adapter.contextScanJobStates;
      const state = states[Math.min(contextScanPollIndex, states.length - 1)];
      contextScanPollIndex++;
      return { ...state, id: jobId };
    },
    checkVoiceDraft: async (_ws, profile, text): Promise<ContextScanCheckResult> => {
      checkVoiceDraftCalls.push({ profileName: profile.name, text });
      // Deterministic: forbidden/competitor terms in the sample text lower
      // the score and surface findings, mirroring the vocabulary matcher.
      const rules = [
        ...(profile.vocabulary.forbidden_terms ?? []),
        ...(profile.vocabulary.competitor_terms ?? []),
      ];
      const findings = rules
        .filter((r) => text.toLowerCase().includes(r.term.toLowerCase()))
        .map((r) => ({
          // Server shape: core/check.Finding — the grouping field is
          // `category`, and the position is a run range, not character offsets.
          category: "vocabulary",
          severity: r.severity ?? "major",
          message: `Uses the term "${r.term}"`,
          position: { kind: "range", start: { run: 0 }, end: { run: 0, offset: r.term.length } },
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

    // --- Measured steerability (model recommendation sweeps) ---------------------
    getModelRecommendations: async () => sampleModelRecommendations,
    refreshModelRecommendations: async () => ({ enqueued: 1, locales: ["fr"] }),

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
    getTaskCounts: async () => ({
      by_status: { open: 0, in_progress: 0, completed: 0, cancelled: 0 },
      total: 0,
    }),
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
        creditsTotal: 2_000_000,
        creditsUsed: 490_000,
        creditsRemaining: 1_510_000,
        resetsAt: new Date(Date.now() + 12 * 24 * 60 * 60 * 1000).toISOString(),
      },
      spendableCredits: 1_510_000,
      stripeCustomerId: "cus_mock",
    }),
    billingGetPlans: async () => ({
      plans: [
        {
          id: "free" as const,
          name: "Free",
          monthly_credits: 0, // no recurring allowance — one-time trial grant instead
          max_markets: 2,
          max_brands: 1,
          max_custodians: 0,
          per_seat: false,
          purchasable: false,
          current: false,
        },
        {
          id: "pro" as const,
          name: "Pro",
          monthly_credits: 2_000_000,
          max_markets: 5,
          max_brands: 1,
          max_custodians: 1,
          per_seat: false,
          purchasable: true,
          current: true,
        },
        {
          id: "team" as const,
          name: "Team",
          monthly_credits: 8_000_000,
          max_markets: 25,
          max_brands: 3,
          max_custodians: 5,
          per_seat: true,
          purchasable: true,
          current: false,
        },
        {
          id: "enterprise" as const,
          name: "Enterprise",
          monthly_credits: -1,
          max_markets: -1,
          max_brands: -1,
          max_custodians: -1,
          per_seat: false,
          purchasable: false,
          current: false,
        },
      ],
      credit_pack: { credits: 200_000, purchasable: true },
    }),
    billingBuyCredits: async () => ({ url: "#" }),
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
    billingGetLedgerPage: async (_ws, query) => ({
      entries: [],
      total: 0,
      limit: query?.limit ?? 50,
      offset: query?.offset ?? 0,
      usage_by_operation: {},
      net_by_operation: {},
      from: query?.from ?? new Date(Date.now() - 30 * 86400_000).toISOString(),
      to: query?.to ?? new Date().toISOString(),
    }),

    // --- Account security ---------------------------------------------------
    refreshSession: async () => true,
    getAccountSecurity: async () => ({ in_app: true, account_url: "" }),
    listPasskeys: async () => ({ passkeys: [] }),
    passkeyRegisterStart: async () => ({ options: {}, nonce: "mock-nonce" }),
    passkeyRegisterFinish: async () => {},
    deletePasskey: async () => {},

    // --- Connectors ---------------------------------------------------------
    listConnectors: async () => [],
    addConnector: async (_ws, type) => ({ id: "conn-1", name: "", category: type }),
    removeConnector: async () => {},
    getConnectorStatus: async (_ws, connectorId) => ({
      connectorId,
      lastSync: "",
      itemCount: 0,
      fileCount: 0,
      wordCount: 0,
      pendingPull: 0,
      pendingPush: 0,
      errors: [],
    }),
    fetchConnector: async () => ({ items_fetched: 0 }),
    publishConnector: async () => ({ status: "ok" }),
    listConnectorContent: async () => [],

    // --- GitHub App ---------------------------------------------------------
    githubSetupState: async () => ({ state: "mock-state", expires_in: 600 }),
    claimInstallation: async () => ({ installation_id: 1, account: "acme" }),
    listInstallationRepos: async () => [],
    detectInstallationRepo: async () => ({
      monorepo_markers: null,
      signals: [],
      proposed_patterns: "",
      match_count: 0,
      match_preview: [],
      truncated: false,
    }),
    bindInstallationRepo: async (_ws, _installationId, req) => ({
      connector_id: "conn-1",
      repository: req.repository,
      project_id: req.project_id,
      branch: req.branch ?? "main",
    }),

    // --- PostHog ------------------------------------------------------------
    getPostHogConnector: async () => ({ configured: false }),
    savePostHogConnector: async () => ({ configured: true }),
    deletePostHogConnector: async () => {},
    getPostHogDemand: async (_ws, _project, range) => ({
      range,
      generated_at: MOCK_NOW,
      total_sessions: 0,
      countries: null,
      languages: null,
      source: { kind: "posthog", host_label: "mock", posthog_project_id: "0" },
      cached_at: MOCK_NOW,
      cached: false,
    }),

    // --- Context profiles ---------------------------------------------------
    listContextProfiles: async () => ({
      profiles: [],
      axes: [],
      terms: { concept_count: 0 },
      scan_scope: "workspace",
    }),

    // --- Channel-slug equivalence -------------------------------------------
    listChannelProposals: async () => ({ proposals: [] }),
    judgeChannelProposal: async (_ws: string, judgement) => ({
      ...judgement,
      judged_at: MOCK_NOW,
      updated_at: MOCK_NOW,
    }),

    // --- Convergence --------------------------------------------------------
    estimateConvergence: async () => ({
      source: { gate: "none", total: 0, ready: 0, held: 0 },
      totals: { pending: 0, via_tm: 0, via_ai: 0, token_estimate: 0 },
    }),

    // --- Concepts -----------------------------------------------------------
    // The context-hub stories drive these from `voiceHubOverrides`, which carries
    // the fixture graph. Here they answer emptily but truthfully, so a story
    // that renders the surface without opting into that graph gets an empty
    // state rather than a crash.
    listConcepts: async () => ({ concepts: [], total_count: 0 }),
    getConceptStatusCounts: async () => ({ total: 0, by_status: {} }),
    getConceptLocaleCoverage: async () => ({ total: 0, locales: [] }),
    getConcept: async (_ws, conceptId) => ({
      id: conceptId,
      domain: "",
      definition: "",
      terms: [],
      created_at: MOCK_NOW,
      updated_at: MOCK_NOW,
    }),
    createConcept: async (_ws, req) => ({
      id: "c-new",
      domain: req.domain,
      definition: req.definition,
      terms: req.terms,
      created_at: MOCK_NOW,
      updated_at: MOCK_NOW,
    }),
    getConceptStory: async (_ws, conceptId) => ({ concept_id: conceptId, entries: [] }),
    listConceptRelations: async () => [],
    addConceptRelation: async (_ws, conceptId, req) => ({
      id: "r-new",
      source_id: conceptId,
      target_id: req.target_id,
      relation_type: req.relation_type,
      note: req.note,
      created_at: MOCK_NOW,
    }),
    deleteConceptRelation: async () => {},
    getConceptBlastRadius: async (_ws, conceptId) => ({
      concept_id: conceptId,
      total_blocks: 0,
      blocks: 0,
      occurrences: 0,
      words: 0,
      projects: [],
      samples: [],
    }),
    getConceptProjects: async (_ws, conceptId) => ({
      concept_id: conceptId,
      source: "graph",
      at: MOCK_NOW,
      projects: [],
      uses: [],
      blocks: 0,
      occurrences: 0,
      uses_total: 0,
    }),
    listObservations: async () => [],
    addObservation: async (_ws, conceptId, req) => ({
      id: "o-new",
      workspace_id: "ws-1",
      concept_id: conceptId,
      created_by: "storybook",
      created_at: MOCK_NOW,
      ...req,
    }),
    deleteObservation: async () => {},
    listConceptComments: async () => [],
    addConceptComment: async (_ws, conceptId, req) => ({
      id: "cm-new",
      workspace_id: "ws-1",
      concept_id: conceptId,
      body: req.body,
      author: "storybook",
      created_at: MOCK_NOW,
      resolved: false,
    }),
    resolveConceptComment: async () => {},
    deleteConceptComment: async () => {},

    // --- Markets ------------------------------------------------------------
    listMarkets: async () => [],
    createMarket: async (_ws, req) => mockMarket("m-new", req),
    updateMarket: async (_ws, marketId, req) => mockMarket(marketId, req),
    deleteMarket: async () => {},

    // --- Change-sets --------------------------------------------------------
    listChangesets: async () => [],
    getChangesetCounts: async () => ({ total: 0, by_status: {} }),
    getChangeset: async (_ws, changesetId) => ({
      ...mockChangeset(changesetId, "draft"),
      governed: false,
      ops: [],
      reviews: [],
      pilots: [],
      solo_review: false,
    }),
    createChangeset: async (_ws, req) => ({
      ...mockChangeset("cs-new", "draft"),
      name: req.name,
      description: req.description,
    }),
    patchChangeset: async (_ws, changesetId, req) => ({
      ...mockChangeset(changesetId, "draft"),
      ...req,
    }),
    appendChangesetOp: async (_ws, changesetId, req) => ({
      workspace_id: "ws-1",
      changeset_id: changesetId,
      seq: 1,
      base_rev: req.base_rev ?? 0,
      created_by: "storybook",
      created_at: MOCK_NOW,
      ...req,
    }),
    removeChangesetOp: async () => {},
    submitChangeset: async (_ws, changesetId) => mockChangeset(changesetId, "in_review"),
    approveChangeset: async (_ws, changesetId) => mockChangeset(changesetId, "approved"),
    rejectChangeset: async (_ws, changesetId) => mockChangeset(changesetId, "draft"),
    mergeChangeset: async (_ws, changesetId) => ({
      changeset_id: changesetId,
      revisions_created: 0,
      pilots_stopped: 0,
    }),
    abandonChangeset: async (_ws, changesetId) => mockChangeset(changesetId, "abandoned"),
    getChangesetBlastRadius: async () => mockImpact(),
    refreshChangesetBlastRadius: async () => mockImpact(),
    addPilot: async (_ws, changesetId, req) => ({
      workspace_id: "ws-1",
      changeset_id: changesetId,
      project_id: req.project_id,
      stream: req.stream,
      created_by: "storybook",
      created_at: MOCK_NOW,
    }),
    removePilot: async () => {},
    trialFindings: async (_ws, changesetId, projectId, stream) => ({
      changeset_id: changesetId,
      project_id: projectId,
      stream,
      total_blocks: 42,
      changed_blocks: 2,
      raised: [
        {
          kind: "term",
          rule: "utilise",
          replacement: "use",
          concept_id: "c-utilise",
          block_id: "b-1",
          item_name: "pricing.md",
          collection_name: "Docs",
          locale: "en-US",
          text: "You can utilise the API to fetch a quote.",
        },
      ],
      cleared: [
        {
          kind: "voice",
          rule: "synergy",
          severity: "major",
          block_id: "b-2",
          item_name: "home.json",
          collection_name: "Pages",
          locale: "en-US",
          text: "Embrace synergy across teams.",
        },
      ],
      raised_total: 1,
      cleared_total: 1,
      terms_computed: true,
      computed_at: MOCK_NOW,
    }),
  };
  return adapter;
}
