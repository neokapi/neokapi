import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  userGuide: [
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/introduction',
        'getting-started/installation',
        'getting-started/quickstart',
        'getting-started/project-walkthrough',
        'getting-started/e2e-walkthrough',
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
      label: 'Bowrain',
      items: [
        'bowrain-web/overview',
        'bowrain-web/getting-started',
        'bowrain-web/translation-editor',
        'bowrain-web/translation-memory',
        'bowrain-web/terminology',
        'bowrain-server/workspaces',
        'bowrain-server/connectors',
        'bowrain-server/automation',
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
  ],
  developer: [
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
        'developer/content-store',
        'developer/plugins',
        'developer/java-bridge',
        'developer/testing',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain',
      items: [
        'developer/server',
        'developer/connectors',
        'developer/events',
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
      label: 'Architecture Decision Records',
      href: '/docs/adr/index',
    },
  ],
};

export default sidebars;
