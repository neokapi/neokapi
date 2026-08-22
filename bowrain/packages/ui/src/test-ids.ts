/**
 * Typed registry of all `data-testid` values used by the bowrain UI.
 *
 * Convention: every element a Playwright spec needs to target gets an entry
 * here, and BOTH the React component AND the spec import the symbol from
 * this file. Renaming a testid is a single-line change.
 *
 * Why: the bowrain web app has historically had 0 `data-testid` attributes
 * — Playwright specs leaned on `getByText`/`getByRole`, which silently
 * break whenever copy changes or seed data drifts (see issue #425, run
 * 24921022592 — `GET /workspaces/.../editor/projects → 404` was a
 * seed-data + getByText failure that the registry pattern prevents).
 *
 * Usage:
 *
 *   // In a React component:
 *   import { TEST_IDS } from "@neokapi/ui/test-ids";
 *   <button data-testid={TEST_IDS.editor.saveButton}>Save</button>
 *
 *   // In a Playwright spec:
 *   import { TEST_IDS } from "@neokapi/ui/test-ids";
 *   await page.getByTestId(TEST_IDS.editor.saveButton).click();
 *
 * Add new entries here as scenes/specs need them. Group by feature area;
 * keep nesting shallow (one level only) so refactors stay simple.
 */

export const TEST_IDS = {
  // ── Auth ──────────────────────────────────────────────────────────
  auth: {
    loginSsoButton: "auth-login-sso-button",
    joinForm: "auth-join-form",
    joinEmailInput: "auth-join-email-input",
    joinSubmit: "auth-join-submit",
    claimForm: "auth-claim-form",
    claimWorkspaceInput: "auth-claim-workspace-input",
    claimSubmit: "auth-claim-submit",
  },

  // ── Workspace rail / dashboard ───────────────────────────────────
  workspace: {
    rail: "workspace-rail",
    switcher: "workspace-switcher",
    newProjectButton: "workspace-new-project-button",
    projectCard: "workspace-project-card",
    projectNameInput: "workspace-project-name-input",
    projectCreateSubmit: "workspace-project-create-submit",
    inviteMemberButton: "workspace-invite-member-button",
    inviteMemberEmailInput: "workspace-invite-member-email-input",
    inviteMemberSubmit: "workspace-invite-member-submit",
  },

  // ── Translation editor ────────────────────────────────────────────
  editor: {
    container: "editor-container",
    sourceColumn: "editor-source-column",
    targetColumn: "editor-target-column",
    block: "editor-block",
    blockSource: "editor-block-source",
    blockTarget: "editor-block-target",
    saveButton: "editor-save-button",
    discardButton: "editor-discard-button",
    splitViewToggle: "editor-split-view-toggle",
    previewToggle: "editor-preview-toggle",
    focusViewToggle: "editor-focus-view-toggle",
    memoryPanel: "editor-tm-panel",
    memoryEntry: "editor-tm-entry",
    termPanel: "editor-term-panel",
    termEntry: "editor-term-entry",
    contextPanel: "editor-context-panel",
    openInDesktopBanner: "open-in-desktop-banner",
    openInDesktopButton: "open-in-desktop-btn",
    openInDesktopDismiss: "dismiss-open-in-desktop",
    desktopNotFoundMessage: "editor-desktop-not-found",
    desktopDownloadLink: "editor-desktop-download-link",
  },

  // ── @bravo agent panel ───────────────────────────────────────────
  bravo: {
    trigger: "bravo-trigger",
    panel: "bravo-panel",
    newConversationHeader: "bravo-new-conversation-header",
    closeButton: "bravo-close-button",
  },

  // ── Project view ──────────────────────────────────────────────────
  project: {
    header: "project-header",
    fileList: "project-file-list",
    fileRow: "project-file-row",
    targetLangsInput: "project-target-langs-input",
    settingsTab: "project-settings-tab",
  },

  // ── Voice (referenced in walkthroughs) ─────────────────────
  brand: {
    profilesList: "voice-profiles-list",
    newProfileButton: "voice-new-profile-button",
    starterPackPicker: "voice-starter-pack-picker",
    profileNameInput: "voice profile-name-input",
  },

  // ── Blank-slate onboarding (the assistant and hosted lanes) ──────
  onboarding: {
    emptyProjects: "empty-projects",
    starterPromptCard: "starter-prompt-card",
    starterPrompt: "starter-prompt",
    copyStarterPrompt: "copy-starter-prompt",
    contextLanes: "context-onboarding",
    contextScanCard: "context-onboarding-scan",
    contextScanButton: "context-onboarding-scan-btn",
    githubConnected: "github-setup-connected",
    githubOpenProject: "github-setup-open-project",
  },

  // ── Context scan (AI context onboarding — epic 016) ──────────────────
  contextScan: {
    input: "context-scan-input",
    urlInput: "context-scan-url-input",
    fileDrop: "context-scan-file-drop",
    start: "context-scan-start",
    progress: "context-scan-progress",
    review: "context-scan-review",
    fieldConfidence: "context-scan-field-confidence",
    termRow: "context-scan-term-row",
    liveTester: "context-scan-live-tester",
    axes: "context-scan-axes",
    approve: "context-scan-approve",
  },

  // ── Settings ──────────────────────────────────────────────────────
  settings: {
    nav: "settings-nav",
    section: "settings-section",
    heading: "settings-heading",
    saveButton: "settings-save-button",
  },
} as const;

/** Helper for places that need the raw string set (e.g. an allowlist check). */
export function flattenTestIds(): readonly string[] {
  const ids: string[] = [];
  const walk = (obj: Record<string, unknown>) => {
    for (const v of Object.values(obj)) {
      if (typeof v === "string") ids.push(v);
      else if (v && typeof v === "object") walk(v as Record<string, unknown>);
    }
  };
  walk(TEST_IDS as Record<string, unknown>);
  return ids;
}
