import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  gokapiSidebar: [
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/introduction',
        'getting-started/installation',
        'getting-started/quickstart',
      ],
    },
    {
      type: 'category',
      label: 'Kapi CLI',
      items: [
        'kapi-cli/overview',
        {
          type: 'category',
          label: 'Commands',
          items: [
            'kapi-cli/commands/flow',
            'kapi-cli/commands/formats',
            'kapi-cli/commands/tools',
            'kapi-cli/commands/plugins',
            'kapi-cli/commands/presets',
            'kapi-cli/commands/pseudo-translate',
            'kapi-cli/commands/termbase',
            'kapi-cli/commands/word-count',
          ],
        },
        'kapi-cli/mcp',
        'kapi-cli/demo-videos',
      ],
    },
    {
      type: 'category',
      label: 'Features',
      items: [
        'features/formats',
        'features/inline-formatting',
        'features/translation-memory',
        'features/terminology',
        'features/ai-translation',
        'features/mt-services',
        'features/qa-checks',
      ],
    },
    {
      type: 'category',
      label: 'Framework',
      items: [
        'developer/architecture',
        'developer/interfaces',
        'developer/formats',
        'developer/tools',
        'developer/translation-memory',
        'developer/terminology',
        'developer/plugins',
        'developer/java-bridge',
        'developer/testing',
        'developer/vocabularies',
      ],
    },
    {
      type: 'link',
      label: 'Test Results',
      href: '/test-comparison',
    },
  ],
  bowrainSidebar: [
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'bowrain-getting-started/introduction',
        'bowrain-getting-started/installation',
        'bowrain-getting-started/quickstart',
        'bowrain-getting-started/project-walkthrough',
        'bowrain-getting-started/e2e-walkthrough',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain CLI',
      items: [
        'bowrain-cli/overview',
        'bowrain-cli/project-model',
        {
          type: 'category',
          label: 'Commands',
          items: [
            'bowrain-cli/commands/init',
            'bowrain-cli/commands/config',
            'bowrain-cli/commands/status',
            'bowrain-cli/commands/diff',
            'bowrain-cli/commands/pull',
            'bowrain-cli/commands/push',
            'bowrain-cli/commands/flow',
            'bowrain-cli/commands/serve',
            'bowrain-cli/commands/auth',
            'bowrain-cli/commands/plugins',
          ],
        },
        {
          type: 'category',
          label: 'Flows',
          items: [
            'bowrain-cli/flows/overview',
            'bowrain-cli/flows/custom-flows',
            'bowrain-cli/flows/hooks',
          ],
        },
        {
          type: 'category',
          label: 'Use Cases',
          items: [
            'bowrain-cli/use-cases/website-translation',
          ],
        },
        'bowrain-cli/mcp',
        'bowrain-cli/demo-videos',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain Desktop',
      items: [
        'bowrain-desktop/overview',
        'bowrain-desktop/getting-started',
        'bowrain-desktop/walkthroughs',
        'bowrain-desktop/projects',
        'bowrain-desktop/workspaces',
        'bowrain-desktop/connectors',
        'bowrain-desktop/demo-videos',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain Server',
      items: [
        'bowrain-server/overview',
        'bowrain-server/web-overview',
        'bowrain-server/getting-started',
        'bowrain-server/translation-editor',
        'bowrain-server/translation-memory',
        'bowrain-server/terminology',
        'bowrain-server/walkthroughs',
        'bowrain-server/workspaces',
        'bowrain-server/connectors',
        'bowrain-server/automation',
        'bowrain-server/demo-videos',
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
            'bowrain-server/self-hosting',
            'bowrain-server/installation',
            'bowrain-server/configuration',
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
