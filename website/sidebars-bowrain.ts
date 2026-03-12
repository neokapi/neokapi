import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  bowrainSidebar: [
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/introduction',
        'getting-started/installation',
        'getting-started/quickstart',
        'getting-started/walkthrough',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain CLI',
      items: [
        'cli/overview',
        'cli/project-model',
        {
          type: 'category',
          label: 'Commands',
          items: [
            'cli/commands/init',
            'cli/commands/config',
            'cli/commands/status',
            'cli/commands/diff',
            'cli/commands/pull',
            'cli/commands/push',
            'cli/commands/sync',
            'cli/commands/flow',
            'cli/commands/serve',
            'cli/commands/auth',
            'cli/commands/plugins',
          ],
        },
        {
          type: 'category',
          label: 'Flows',
          items: [
            'cli/flows/overview',
            'cli/flows/custom-flows',
            'cli/flows/hooks',
          ],
        },
        {
          type: 'category',
          label: 'Use Cases',
          items: [
            'cli/use-cases/website-translation',
            'cli/use-cases/github-actions',
            'cli/use-cases/source-prep',
          ],
        },
        'cli/mcp',
        'cli/demo-videos',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain Desktop',
      items: [
        'desktop/overview',
        'desktop/getting-started',
        'desktop/walkthroughs',
        'desktop/projects',
        'desktop/workspaces',
        'desktop/connectors',
        'desktop/demo-videos',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain Web',
      items: [
        'server/overview',
        'server/web-overview',
        'server/getting-started',
        'server/translation-editor',
        'server/translation-memory',
        'server/terminology',
        'server/walkthroughs',
        'server/workspaces',
        'server/connectors',
        'server/automation',
        'server/flows',
        'server/demo-videos',
      ],
    },
    {
      type: 'category',
      label: 'Developer',
      items: [
        'developer/server',
        'developer/connectors',
        'developer/events',
        'developer/content-store',
        {
          type: 'category',
          label: 'Self-Hosting',
          items: [
            'server/self-hosting',
            'server/installation',
            'server/configuration',
          ],
        },
        'developer/release',
      ],
    },
    {
      type: 'link',
      label: 'Architecture Decisions',
      href: '/docs/ad/index',
    },
    {
      type: 'link',
      label: 'Implementation Notes',
      href: '/docs/notes/index',
    },
    {
      type: 'link',
      label: 'Storybook',
      href: 'pathname:///storybook/',
    },
  ],
};

export default sidebars;
