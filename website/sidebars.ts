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
            'user-guide/cli/convert',
            'user-guide/cli/translate',
            'user-guide/cli/extract-merge',
            'user-guide/cli/flow',
            'user-guide/cli/pack-unpack',
            'user-guide/cli/plugins',
          ],
        },
        'user-guide/formats',
        'user-guide/translation-memory',
        'user-guide/ai-translation',
        'user-guide/connectors',
        'user-guide/configuration',
      ],
    },
    {
      type: 'category',
      label: 'Bowrain Desktop App',
      items: [
        'bowrain/overview',
        'bowrain/getting-started',
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
