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
    tmPanel: "editor-tm-panel",
    tmEntry: "editor-tm-entry",
    termPanel: "editor-term-panel",
    termEntry: "editor-term-entry",
    contextPanel: "editor-context-panel",
    openInDesktopButton: "editor-open-in-desktop-button",
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

  // ── Brand voice (referenced in walkthroughs) ─────────────────────
  brand: {
    profilesList: "brand-profiles-list",
    newProfileButton: "brand-new-profile-button",
    starterPackPicker: "brand-starter-pack-picker",
    profileNameInput: "brand-profile-name-input",
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
