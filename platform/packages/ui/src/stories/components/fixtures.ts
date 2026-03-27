/**
 * Shared fixtures for component Storybook stories.
 */

import type {
  User,
  Workspace,
  StreamInfo,
  StreamDiffResult,
  StreamMergeResult,
  CollectionInfo,
  CollectionTranslationStats,
  ItemTranslationStats,
  LocaleTranslationStats,
  ArchivedProject,
  NotificationInfo,
  ActivityInfo,
  TaskInfo,
  RoleTemplate,
} from "../../types/api";

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

export const sampleUser: User = {
  id: "user-1",
  email: "alice@example.com",
  name: "Alice Chen",
  avatar_url: "",
};

export const anotherUser: User = {
  id: "user-2",
  email: "bob@example.com",
  name: "Bob Martinez",
  avatar_url: "",
};

// ---------------------------------------------------------------------------
// Workspaces
// ---------------------------------------------------------------------------

export const sampleWorkspace: Workspace = {
  id: "ws-1",
  name: "Acme Corp",
  slug: "acme",
  description: "Main workspace",
  logo_url: "",
  type: "team",
  role: "owner",
  languages: ["en", "fr", "de", "ja", "es"],
};

export const personalWorkspace: Workspace = {
  id: "ws-2",
  name: "Personal",
  slug: "personal",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

export const viewerWorkspace: Workspace = {
  ...sampleWorkspace,
  id: "ws-3",
  name: "Client Project",
  slug: "client",
  role: "viewer",
};

// ---------------------------------------------------------------------------
// Streams
// ---------------------------------------------------------------------------

export const mainStream: StreamInfo = {
  name: "main",
  parent: "",
  base_cursor: 0,
  archived: false,
  locked: false,
  visibility: "public",
  description: "Primary content stream",
  created_at: "2026-01-01T10:00:00Z",
  created_by: "user-1",
};

export const featureStream: StreamInfo = {
  name: "feature/translations",
  parent: "main",
  base_cursor: 5,
  archived: false,
  locked: false,
  visibility: "private",
  description: "Q1 translations",
  created_at: "2026-03-01T10:00:00Z",
  created_by: "user-1",
};

export const sharedStream: StreamInfo = {
  name: "review/q1",
  parent: "main",
  base_cursor: 3,
  archived: false,
  locked: false,
  visibility: "shared",
  description: "Review branch",
  created_at: "2026-02-15T10:00:00Z",
  created_by: "user-2",
  shared_with: ["user-1"],
};

export const lockedStream: StreamInfo = {
  name: "release/v2.0",
  parent: "main",
  base_cursor: 10,
  archived: false,
  locked: true,
  locked_by: "user-1",
  locked_at: "2026-03-15T14:00:00Z",
  visibility: "public",
  description: "Release v2.0 — locked for deployment",
  created_at: "2026-03-10T10:00:00Z",
  created_by: "user-1",
};

export const archivedStream: StreamInfo = {
  name: "old/release-1",
  parent: "main",
  base_cursor: 1,
  archived: true,
  locked: false,
  visibility: "public",
  description: "Archived release",
  created_at: "2025-12-01T10:00:00Z",
  created_by: "user-1",
};

export const sampleStreams: StreamInfo[] = [mainStream, featureStream, sharedStream];

// ---------------------------------------------------------------------------
// Stream Tags
// ---------------------------------------------------------------------------

import type { StreamTag } from "../../types/api";

export const mergeTag: StreamTag = {
  id: "tag-1",
  project_id: "proj-1",
  stream: "feature/translations",
  name: "merged-main-20260315-140000",
  kind: "merge",
  cursor: 42,
  metadata: {
    target_stream: "main",
    merged_blocks: "12",
    added_blocks: "5",
    modified_blocks: "6",
    removed_blocks: "1",
  },
  created_by: "user-1",
  created_at: "2026-03-15T14:00:00Z",
};

export const releaseTag: StreamTag = {
  id: "tag-2",
  project_id: "proj-1",
  stream: "release/v2.0",
  name: "v2.0",
  kind: "release",
  cursor: 50,
  metadata: { version: "2.0.0", notes: "GA release" },
  created_by: "user-1",
  created_at: "2026-03-20T10:00:00Z",
};

export const milestoneTag: StreamTag = {
  id: "tag-3",
  project_id: "proj-1",
  stream: "main",
  name: "translations-complete-fr",
  kind: "milestone",
  cursor: 30,
  metadata: { description: "French translations 100%" },
  created_by: "user-2",
  created_at: "2026-03-18T09:00:00Z",
};

export const customTag: StreamTag = {
  id: "tag-4",
  project_id: "proj-1",
  stream: "main",
  name: "qa-approved",
  kind: "custom",
  cursor: 35,
  created_by: "user-1",
  created_at: "2026-03-19T16:00:00Z",
};

export const sampleTags: StreamTag[] = [mergeTag, releaseTag, milestoneTag, customTag];

// ---------------------------------------------------------------------------
// Stream Diff & Merge
// ---------------------------------------------------------------------------

export const sampleDiff: StreamDiffResult = {
  stream_name: "feature/translations",
  parent_name: "main",
  changes: [
    { block_id: "blk-hero", change_type: "modified", old_hash: "abc1234", new_hash: "def5678" },
    { block_id: "blk-cta", change_type: "modified", old_hash: "111aaaa", new_hash: "222bbbb" },
    { block_id: "blk-banner", change_type: "added", old_hash: "", new_hash: "333cccc" },
    { block_id: "blk-footer-old", change_type: "removed", old_hash: "444dddd", new_hash: "" },
  ],
};

export const emptyDiff: StreamDiffResult = {
  stream_name: "feature/translations",
  parent_name: "main",
  changes: [],
};

export const sampleMergeResult: StreamMergeResult = {
  merged_blocks: 12,
  added_blocks: 3,
  modified_blocks: 7,
  removed_blocks: 2,
};

export const emptyMergeResult: StreamMergeResult = {
  merged_blocks: 0,
  added_blocks: 0,
  modified_blocks: 0,
  removed_blocks: 0,
};

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

export const defaultCollection: CollectionInfo = {
  id: "coll-default",
  project_id: "proj-1",
  name: "All Items",
  kind: "uploaded",
  item_label: "item",
  is_default: true,
  item_count: 42,
  created_at: "2026-01-01T10:00:00Z",
  updated_at: "2026-03-10T10:00:00Z",
};

export const docsCollection: CollectionInfo = {
  id: "coll-docs",
  project_id: "proj-1",
  name: "Documentation",
  kind: "uploaded",
  item_label: "document",
  is_default: false,
  item_count: 15,
  created_at: "2026-02-01T10:00:00Z",
  updated_at: "2026-03-10T10:00:00Z",
};

export const connectedCollection: CollectionInfo = {
  id: "coll-cms",
  project_id: "proj-1",
  name: "CMS Content",
  kind: "connected",
  item_label: "page",
  is_default: false,
  item_count: 28,
  created_at: "2026-02-15T10:00:00Z",
  updated_at: "2026-03-12T10:00:00Z",
};

export const sampleCollections: CollectionInfo[] = [
  defaultCollection,
  docsCollection,
  connectedCollection,
];

// ---------------------------------------------------------------------------
// Translation Stats
// ---------------------------------------------------------------------------

export const sampleLocaleStats: LocaleTranslationStats[] = [
  {
    locale: "fr-FR",
    display_name: "French (France)",
    percentage: 85.2,
    translated_words: 4260,
    total_words: 5000,
    translated_blocks: 170,
    total_blocks: 200,
  },
  {
    locale: "de-DE",
    display_name: "German (Germany)",
    percentage: 62.0,
    translated_words: 3100,
    total_words: 5000,
    translated_blocks: 124,
    total_blocks: 200,
  },
  {
    locale: "ja-JP",
    display_name: "Japanese (Japan)",
    percentage: 45.5,
    translated_words: 2275,
    total_words: 5000,
    translated_blocks: 91,
    total_blocks: 200,
  },
  {
    locale: "es-ES",
    display_name: "Spanish (Spain)",
    percentage: 92.0,
    translated_words: 4600,
    total_words: 5000,
    translated_blocks: 184,
    total_blocks: 200,
  },
];

export const sampleItemStats: ItemTranslationStats[] = [
  {
    item_id: "itm-1",
    item_name: "landing.html",
    format: "html",
    word_count: 1320,
    collection_id: "coll-default",
    block_count: 48,
    locales: [
      {
        locale: "fr-FR",
        percentage: 95,
        translated_words: 1254,
        total_words: 1320,
        translated_blocks: 46,
        total_blocks: 48,
      },
      {
        locale: "de-DE",
        percentage: 70,
        translated_words: 924,
        total_words: 1320,
        translated_blocks: 34,
        total_blocks: 48,
      },
    ],
  },
  {
    item_id: "itm-2",
    item_name: "about.json",
    format: "json",
    word_count: 640,
    collection_id: "coll-default",
    block_count: 20,
    locales: [
      {
        locale: "fr-FR",
        percentage: 100,
        translated_words: 640,
        total_words: 640,
        translated_blocks: 20,
        total_blocks: 20,
      },
      {
        locale: "de-DE",
        percentage: 50,
        translated_words: 320,
        total_words: 640,
        translated_blocks: 10,
        total_blocks: 20,
      },
    ],
  },
  {
    item_id: "itm-3",
    item_name: "api-reference.md",
    format: "md",
    word_count: 3040,
    collection_id: "coll-docs",
    block_count: 80,
    locales: [
      {
        locale: "fr-FR",
        percentage: 78,
        translated_words: 2371,
        total_words: 3040,
        translated_blocks: 62,
        total_blocks: 80,
      },
      {
        locale: "de-DE",
        percentage: 30,
        translated_words: 912,
        total_words: 3040,
        translated_blocks: 24,
        total_blocks: 80,
      },
    ],
  },
];

export const sampleCollectionStats: CollectionTranslationStats[] = [
  {
    collection_id: "coll-default",
    collection_name: "Default",
    item_count: 3,
    block_count: 148,
    word_count: 5000,
    locales: [
      {
        locale: "fr-FR",
        percentage: 85,
        translated_words: 4250,
        total_words: 5000,
        translated_blocks: 128,
        total_blocks: 148,
      },
      {
        locale: "de-DE",
        percentage: 52,
        translated_words: 2600,
        total_words: 5000,
        translated_blocks: 68,
        total_blocks: 148,
      },
    ],
  },
  {
    collection_id: "coll-docs",
    collection_name: "Documentation",
    item_count: 8,
    block_count: 120,
    word_count: 12000,
    locales: [
      {
        locale: "fr-FR",
        percentage: 60,
        translated_words: 7200,
        total_words: 12000,
        translated_blocks: 72,
        total_blocks: 120,
      },
      {
        locale: "de-DE",
        percentage: 25,
        translated_words: 3000,
        total_words: 12000,
        translated_blocks: 30,
        total_blocks: 120,
      },
    ],
  },
];

// ---------------------------------------------------------------------------
// Archived Projects (Recycle Bin)
// ---------------------------------------------------------------------------

export const sampleArchivedProjects: ArchivedProject[] = [
  {
    id: "proj-old-1",
    name: "Legacy Mobile App",
    default_source_language: "en",
    target_languages: ["fr", "de"],
    archived: true,
    archived_at: "2026-03-10T10:00:00Z",
    created_at: "2025-06-01T10:00:00Z",
    updated_at: "2026-03-10T10:00:00Z",
  },
  {
    id: "proj-old-2",
    name: "Old Marketing Site",
    default_source_language: "en-US",
    target_languages: ["ja-JP", "ko-KR", "zh-CN"],
    archived: true,
    archived_at: "2026-03-14T10:00:00Z",
    created_at: "2025-09-01T10:00:00Z",
    updated_at: "2026-03-14T10:00:00Z",
  },
];

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

export const sampleNotifications: NotificationInfo[] = [
  {
    id: "notif-1",
    user_id: "user-1",
    type: "review.assigned",
    title: "Review assigned",
    body: "You have been assigned to review translations for landing.html",
    read: false,
    created_at: new Date(Date.now() - 5 * 60000).toISOString(),
  },
  {
    id: "notif-2",
    user_id: "user-1",
    type: "review.completed",
    title: "Review completed",
    body: "Bob approved all translations for about.json (fr-FR)",
    read: false,
    created_at: new Date(Date.now() - 2 * 3600000).toISOString(),
  },
  {
    id: "notif-3",
    user_id: "user-1",
    type: "extraction.completed",
    title: "Extraction completed",
    body: "48 blocks extracted from landing.html",
    read: true,
    created_at: new Date(Date.now() - 24 * 3600000).toISOString(),
  },
  {
    id: "notif-4",
    user_id: "user-1",
    type: "content.available",
    title: "New content available",
    body: "3 items pushed with 47 blocks ready for translation in fr-FR",
    read: false,
    created_at: new Date(Date.now() - 30 * 60000).toISOString(),
  },
  {
    id: "notif-5",
    user_id: "user-1",
    type: "progress.milestone",
    title: "Progress milestone reached",
    body: "fr-FR reached 75% translated for landing.html",
    read: false,
    created_at: new Date(Date.now() - 4 * 3600000).toISOString(),
  },
  {
    id: "notif-6",
    user_id: "user-1",
    type: "deadline.approaching",
    title: "Deadline approaching",
    body: 'Task "Review fr-FR translations" is due in 24 hours',
    read: false,
    priority: "high",
    created_at: new Date(Date.now() - 15 * 60000).toISOString(),
  },
  {
    id: "notif-7",
    user_id: "user-1",
    type: "mention",
    title: "Alice mentioned you",
    body: "Hey @user-1 can you check the terminology in block 42?",
    read: false,
    created_at: new Date(Date.now() - 45 * 60000).toISOString(),
  },
  {
    id: "notif-8",
    user_id: "user-1",
    type: "task.assigned",
    title: "New review task: fr-FR",
    body: "Content is ready for review in fr-FR.",
    read: false,
    task_id: "task-1",
    created_at: new Date(Date.now() - 10 * 60000).toISOString(),
  },
  {
    id: "notif-9",
    user_id: "user-1",
    type: "content.ready",
    title: "Content ready for review",
    body: "AI translation and extraction completed — content is ready for human review",
    read: false,
    created_at: new Date(Date.now() - 12 * 60000).toISOString(),
  },
];

// ---------------------------------------------------------------------------
// Activities
// ---------------------------------------------------------------------------

export const sampleActivities: ActivityInfo[] = [
  {
    id: "act-1",
    workspace_id: "ws-1",
    actor_id: "user-1",
    type: "translation.completed",
    summary: "completed translation of about.json to fr-FR",
    actor_name: "Alice Chen",
    created_at: new Date(Date.now() - 10 * 60000).toISOString(),
  },
  {
    id: "act-2",
    workspace_id: "ws-1",
    actor_id: "user-2",
    type: "review.passed",
    summary: "approved translations for landing.html (de-DE)",
    actor_name: "Bob Martinez",
    created_at: new Date(Date.now() - 3600000).toISOString(),
  },
  {
    id: "act-3",
    workspace_id: "ws-1",
    actor_id: "user-1",
    type: "stream.created",
    summary: "created stream feature/translations",
    actor_name: "Alice Chen",
    created_at: new Date(Date.now() - 86400000).toISOString(),
  },
];

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

export const sampleTasks: TaskInfo[] = [
  {
    id: "task-1",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "review",
    title: "Review fr-FR translations for landing.html",
    status: "open",
    priority: "high",
    created_by: "user-1",
    created_at: new Date(Date.now() - 3600000).toISOString(),
    updated_at: new Date(Date.now() - 3600000).toISOString(),
    due_at: new Date(Date.now() + 86400000).toISOString(),
  },
  {
    id: "task-2",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "translate",
    title: "Translate api-reference.md to ja-JP",
    status: "in_progress",
    priority: "normal",
    created_by: "user-1",
    created_at: new Date(Date.now() - 86400000).toISOString(),
    updated_at: new Date(Date.now() - 86400000).toISOString(),
  },
  {
    id: "task-3",
    workspace_id: "ws-1",
    project_id: "proj-1",
    type: "fix_quality",
    title: "Fix terminology in about.json",
    status: "open",
    priority: "urgent",
    created_by: "user-2",
    created_at: new Date(Date.now() - 7200000).toISOString(),
    updated_at: new Date(Date.now() - 7200000).toISOString(),
    due_at: new Date(Date.now() - 3600000).toISOString(), // overdue
  },
];

// ---------------------------------------------------------------------------
// Role Templates
// ---------------------------------------------------------------------------

export const sampleRoleTemplates: RoleTemplate[] = [
  {
    id: "role-1",
    workspace_id: "ws-1",
    name: "translator",
    display_name: "Translator",
    description: "Can view content and submit translations",
    permissions: 0,
    permission_names: ["view_content", "translate"],
    is_builtin: true,
    position: 1,
    created_at: "2026-01-01T10:00:00Z",
    updated_at: "2026-01-01T10:00:00Z",
  },
  {
    id: "role-2",
    workspace_id: "ws-1",
    name: "reviewer",
    display_name: "Reviewer",
    description: "Can review and approve translations",
    permissions: 0,
    permission_names: ["view_content", "translate", "review", "manage_terms"],
    is_builtin: true,
    position: 2,
    created_at: "2026-01-01T10:00:00Z",
    updated_at: "2026-01-01T10:00:00Z",
  },
  {
    id: "role-3",
    workspace_id: "ws-1",
    name: "project-manager",
    display_name: "Project Manager",
    description: "Full project management access",
    permissions: 0,
    permission_names: [
      "view_content",
      "edit_source",
      "translate",
      "review",
      "manage_terms",
      "manage_tm",
      "run_flows",
      "manage_files",
      "manage_streams",
      "manage_members",
      "manage_project",
    ],
    is_builtin: false,
    position: 3,
    created_at: "2026-02-15T10:00:00Z",
    updated_at: "2026-03-01T10:00:00Z",
  },
];
