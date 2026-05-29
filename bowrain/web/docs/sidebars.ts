import type { SidebarsConfig } from "@docusaurus/plugin-content-docs";

// One sidebar per top-level navbar section.
//
//   gettingStartedSidebar  — install, quickstart; anchored to the site root
//   cliSidebar             — project sync via kapi + the bowrain plugin
//   webSidebar             — Bowrain web app (browser editor)
//   desktopSidebar         — Bowrain desktop app
//   selfHostingSidebar     — run your own server
//
// Developer-only material (developer/, architecture-decisions/, notes/,
// brand-voice strategy docs) is intentionally excluded from all sidebars so
// it does not appear in the navigation. The files remain on disk so existing
// deep-links do not 404.
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

  webSidebar: [
    {
      type: "category",
      label: "Overview",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/web-overview",
        "server/getting-started",
        "server/workspaces",
      ],
    },
    {
      type: "category",
      label: "Editing & Translation",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/translation-editor",
        "server/translation-memory",
        "server/terminology",
        "server/flows",
      ],
    },
    {
      type: "category",
      label: "Brand Governance",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/brand-voice",
      ],
    },
    {
      type: "category",
      label: "Workspace & Automation",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/connectors",
        "server/automation",
      ],
    },
    {
      type: "category",
      label: "Walkthroughs",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/walkthroughs",
        "walkthroughs/bowrain-web-login-and-workspace",
        "walkthroughs/bowrain-web-claim-project",
        "walkthroughs/bowrain-web-translation-editor",
        "walkthroughs/bowrain-web-focus-view",
        "walkthroughs/bowrain-web-context-panel",
        "walkthroughs/bowrain-web-tm-explorer",
        "walkthroughs/bowrain-web-term-explorer",
        "walkthroughs/bowrain-web-pseudo-translation",
        "walkthroughs/bowrain-web-settings",
        "walkthroughs/bowrain-web-invite-workflow",
      ],
    },
  ],

  desktopSidebar: [
    {
      type: "category",
      label: "Overview",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "desktop/overview",
        "desktop/getting-started",
        "desktop/workspaces",
      ],
    },
    {
      type: "category",
      label: "Projects & Files",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "desktop/projects",
        "desktop/connectors",
      ],
    },
    {
      type: "category",
      label: "Walkthroughs",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "desktop/walkthroughs",
      ],
    },
  ],

  selfHostingSidebar: [
    {
      type: "category",
      label: "Self-Hosting",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/overview",
        "server/self-hosting",
        "server/installation",
        "server/configuration",
      ],
    },
  ],
};

export default sidebars;
