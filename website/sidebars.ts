import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docs: [
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/introduction',
        'getting-started/installation',
        'getting-started/quickstart',
        'getting-started/project-walkthrough',
      ],
    },
    {
      type: 'category',
      label: 'Kapi CLI',
      items: [
        'kapi-cli/overview',
        'kapi-cli/installation',
        'kapi-cli/project-model',
        {
          type: 'category',
          label: 'Commands',
          items: [
            'kapi-cli/commands/init',
            'kapi-cli/commands/status',
            'kapi-cli/commands/diff',
            'kapi-cli/commands/pull',
            'kapi-cli/commands/push',
            'kapi-cli/commands/flow',
            'kapi-cli/commands/serve',
            'kapi-cli/commands/auth',
            'kapi-cli/commands/plugins',
          ],
        },
        {
          type: 'category',
          label: 'Flows',
          items: [
            'kapi-cli/flows/overview',
            'kapi-cli/flows/custom-flows',
            'kapi-cli/flows/hooks',
          ],
        },
        {
          type: 'category',
          label: 'Use Cases',
          items: [
            'kapi-cli/use-cases/website-translation',
          ],
        },
        'kapi-cli/demo-videos',
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
      label: 'Bowrain Server',
      items: [
        'bowrain-server/overview',
        'bowrain-server/installation',
        'bowrain-server/configuration',
        'bowrain-server/workspaces',
        'bowrain-server/connectors',
        'bowrain-server/automation',
        'bowrain-server/self-hosting',
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
      ],
    },
    {
      type: 'category',
      label: 'Developer Guide',
      items: [
        'developer/architecture',
        'developer/interfaces',
        'developer/formats',
        'developer/tools',
        'developer/translation-memory',
        'developer/terminology',
        'developer/content-store',
        'developer/connectors',
        'developer/events',
        'developer/server',
        'developer/plugins',
        'developer/java-bridge',
        'developer/testing',
        'developer/release',
      ],
    },
    {
      type: 'link',
      label: 'Architecture Decision Records',
      href: '/docs/adr/index',
    },
  ],
};

export default sidebars;
