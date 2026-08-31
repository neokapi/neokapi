import type { SidebarsConfig } from "@docusaurus/plugin-content-docs";

// One sidebar per top-level navbar section.
//
//   gettingStartedSidebar: what the platform is, how content stays current,
//                          and the routes in; anchored to the site root
//   usingBowrainSidebar:   the product, organized by what you do; shared
//                          across the browser and desktop clients. Context
//                          governance leads; the multilingual surfaces
//                          (editor, memory) follow as the extension.
//   cliSidebar:            the developer/CI connector: kapi + the bowrain
//                          plugin. One route among the connectors, not the
//                          default way into the platform.
//   forDevelopersSidebar:  self-hosting + the developer/ engineering docs
//
// Section headings use `collapsible: false` so they render as static labels
// rather than collapsible menus, consistent with the kapi docs site pattern.
const sidebars: SidebarsConfig = {
  gettingStartedSidebar: [
    {
      type: "category",
      label: "Get Started",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "getting-started/introduction",
        "getting-started/the-context-graph",
        "getting-started/the-loop",
        "getting-started/quickstart",
        "getting-started/installation",
        "getting-started/kapi-vs-bowrain",
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
      label: "Context & voice",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/context",
        "server/context-scan",
        "server/context-voice",
        "server/terminology",
      ],
    },
    {
      type: "category",
      label: "Review",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/review",
      ],
    },
    {
      type: "category",
      label: "Workspaces & members",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/workspaces",
        "server/members-and-roles",
        "server/billing-and-credits",
      ],
    },
    {
      type: "category",
      label: "Connectors & automation",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/connectors",
        "server/connectors/wordpress",
        "server/connectors/hubspot",
        "server/connectors/figma",
        "server/connectors/github",
        "server/connectors/kapi",
        "server/publish-on-brand",
        "server/flows",
        "server/automation",
      ],
    },
    {
      type: "category",
      label: "The editor",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/translation-editor",
        "server/collaboration",
      ],
    },
    {
      type: "category",
      label: "Content memory",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/translation-memory",
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
    {
      type: "category",
      label: "Trust & support",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "server/security-and-privacy",
        "server/troubleshooting",
        "server/faq",
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
        "cli/commands/up",
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
      label: "Continuous integration",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "cli/ci/overview",
        "cli/use-cases/github-actions",
        "cli/ci/gitlab",
        "cli/ci/github-app",
        "cli/use-cases/brand-terminology-ci",
      ],
    },
    {
      type: "category",
      label: "Guides",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "cli/use-cases/website-translation",
        "cli/use-cases/source-prep",
      ],
    },
    {
      type: "category",
      label: "Walkthroughs",
      collapsible: false,
      className: "sidebar-section-heading",
      items: [
        "walkthroughs/bowrain-getting-started",
        "walkthroughs/bowrain-auth",
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
  ],
};

export default sidebars;
