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
        'kapi-cli/demo-videos',
      ],
    },
    {
      type: 'category',
      label: 'Features',
      items: [
        'features/formats',
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
      ],
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
      label: 'Brain CLI',
      items: [
        'brain-cli/overview',
        'brain-cli/project-model',
        {
          type: 'category',
          label: 'Commands',
          items: [
            'brain-cli/commands/init',
            'brain-cli/commands/config',
            'brain-cli/commands/status',
            'brain-cli/commands/diff',
            'brain-cli/commands/pull',
            'brain-cli/commands/push',
            'brain-cli/commands/flow',
            'brain-cli/commands/serve',
            'brain-cli/commands/auth',
            'brain-cli/commands/plugins',
          ],
        },
        {
          type: 'category',
          label: 'Flows',
          items: [
            'brain-cli/flows/overview',
            'brain-cli/flows/custom-flows',
            'brain-cli/flows/hooks',
          ],
        },
        {
          type: 'category',
          label: 'Use Cases',
          items: [
            'brain-cli/use-cases/website-translation',
          ],
        },
        'brain-cli/demo-videos',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain Web',
      items: [
        'bowrain-web/overview',
        'bowrain-web/getting-started',
        'bowrain-web/translation-editor',
        'bowrain-web/translation-memory',
        'bowrain-web/terminology',
        'bowrain-web/walkthroughs',
        'bowrain-web/demo-videos',
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
        'bowrain-server/workspaces',
        'bowrain-server/connectors',
        'bowrain-server/automation',
        'bowrain-server/server-walkthrough',
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
