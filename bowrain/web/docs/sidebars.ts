import type { SidebarsConfig } from "@docusaurus/plugin-content-docs";

// One sidebar per top-level navbar section.
//
//   gettingStartedSidebar  — install, quickstart; anchored to the site root
//   usingBowrainSidebar    — the product, organized by what you do; shared
//                            across the browser and desktop clients
//   cliSidebar             — project sync via kapi + the bowrain plugin
//   forDevelopersSidebar   — self-hosting + the engineering docs (developer/,
//                            architecture-decisions/, notes/)
//
// Section headings use `collapsible: false` so they render as static labels
// rather than collapsible menus — consistent with the kapi docs site pattern.
const sidebars: SidebarsConfig = {
  gettingStartedSidebar: [
    {
      type: "category",
      label: "Get Started",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "getting-started/introduction",
        "getting-started/kapi-vs-bowrain",
        "getting-started/installation",
        "getting-started/quickstart",
      ],
    },
    {
      type: "category",
      label: "Walkthroughs",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "walkthroughs/bowrain-getting-started",
        "walkthroughs/bowrain-overview",
      ],
    },
  ],

  usingBowrainSidebar: [
    {
      type: "category",
      label: "Overview",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/web-overview",
        "server/getting-started",
        "server/walkthroughs",
      ],
    },
    {
      type: "category",
      label: "Workspaces & members",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/workspaces",
      ],
    },
    {
      type: "category",
      label: "The editor",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/translation-editor",
        "server/pre-process",
        "server/review",
        "server/collaboration",
      ],
    },
    {
      type: "category",
      label: "Translation memory & terminology",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/translation-memory",
        "server/terminology",
      ],
    },
    {
      type: "category",
      label: "Brand governance",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/brand-voice",
      ],
    },
    {
      type: "category",
      label: "Connectors & automation",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/connectors",
        "server/flows",
        "server/automation",
      ],
    },
    {
      type: "category",
      label: "The desktop app",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/desktop-app",
      ],
    },
  ],

  cliSidebar: [
    {
      type: "category",
      label: "Overview",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "cli/overview",
        "cli/project-model",
        "cli/mcp",
      ],
    },
    {
      type: "category",
      label: "Commands",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "cli/commands/init",
        "cli/commands/status",
        "cli/commands/diff",
        "cli/commands/pull",
        "cli/commands/push",
        "cli/commands/sync",
        "cli/commands/run",
        "cli/commands/auth",
        "cli/commands/config",
        "cli/commands/workspace",
        "cli/commands/plugins",
      ],
    },
    {
      type: "category",
      label: "Flows",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "cli/flows/overview",
        "cli/flows/custom-flows",
        "cli/flows/hooks",
      ],
    },
    {
      type: "category",
      label: "Guides",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "cli/use-cases/website-translation",
        "cli/use-cases/github-actions",
        "cli/use-cases/source-prep",
      ],
    },
    {
      type: "category",
      label: "Walkthroughs",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "walkthroughs/bowrain-init",
        "walkthroughs/bowrain-create-project",
        "walkthroughs/bowrain-auth",
        "walkthroughs/bowrain-pseudo-translate",
        "walkthroughs/bowrain-workspaces",
        "walkthroughs/bowrain-automation",
      ],
    },
  ],

  forDevelopersSidebar: [
    {
      type: "category",
      label: "Self-hosting",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/overview",
        "server/self-hosting",
        "server/installation",
        "server/configuration",
      ],
    },
    {
      type: "category",
      label: "Architecture",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "developer/server",
        "developer/connectors",
        "developer/content-store",
        "developer/events",
        "developer/local-development",
        "developer/release",
      ],
    },
    {
      type: "category",
      label: "Architecture decisions",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "architecture-decisions/README",
        "architecture-decisions/001-vision-and-modules",
        "architecture-decisions/002-authentication-and-workspaces",
        "architecture-decisions/003-permissions",
        "architecture-decisions/004-content-store",
        "architecture-decisions/005-streams",
        "architecture-decisions/006-graph-concept-storage",
        "architecture-decisions/007-media-and-blob-storage",
        "architecture-decisions/008-connector-system",
        "architecture-decisions/009-sync-protocol",
        "architecture-decisions/010-bowrain-cli-and-project-model",
        "architecture-decisions/011-rest-api",
        "architecture-decisions/012-distributed-event-bus",
        "architecture-decisions/013-automation-engine",
        "architecture-decisions/014-translator-workflow",
        "architecture-decisions/015-server-ai-operations",
        "architecture-decisions/016-bravo-agent",
        "architecture-decisions/017-bowrain-apps",
        "architecture-decisions/018-billing-and-plans",
        "architecture-decisions/019-correction-learning-loop",
        "architecture-decisions/020-governance-audit-rollback",
      ],
    },
    {
      type: "category",
      label: "Implementation notes",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "notes/README",
        "notes/admin-control-plane",
        "notes/automation-run-visibility",
        "notes/brand-voice-data-model",
        "notes/bravo-agent-implementation",
        "notes/cli-commands-reference",
        "notes/connector-interfaces",
        "notes/content-store-schema",
        "notes/entity-term-extraction",
        "notes/graph-store-schema",
        "notes/media-asset-storage",
        "notes/sync-protocol",
        "notes/translation-job-queue",
        "notes/translator-workflow",
      ],
    },
  ],
};

export default sidebars;
