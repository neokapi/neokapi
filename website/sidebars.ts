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
      ],
    },
    {
      type: 'category',
      label: 'User Guide',
      items: [
        {
          type: 'category',
          label: 'CLI Reference',
          items: [
            'user-guide/cli/flow',
            'user-guide/cli/plugins',
            'user-guide/cli/connect',
            'user-guide/cli/store',
            'user-guide/cli/auth',
            'user-guide/cli/serve',
          ],
        },
        {
          type: 'category',
          label: 'Use Cases',
          items: [
            'user-guide/use-cases/website-translation',
          ],
        },
        'cli/demo-videos',
        'user-guide/formats',
        'user-guide/translation-memory',
        'user-guide/terminology',
        'user-guide/ai-translation',
        'user-guide/mt-services',
        'user-guide/content-store',
        'user-guide/workspaces',
        'user-guide/server-walkthrough',
        'user-guide/self-hosting',
        'user-guide/automation',
        'user-guide/configuration',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain Desktop App',
      items: [
        'bowrain/overview',
        'bowrain/getting-started',
        'bowrain/walkthroughs',
        'bowrain/projects',
        'bowrain/workspaces',
        'bowrain/connectors',
        'bowrain/demo-videos',
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
