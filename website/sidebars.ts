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
      type: 'category',
      label: 'Architecture Decision Records',
      items: [
        'adr/index',
        'adr/001-go-reimagining-of-okapi',
        'adr/002-content-model',
        'adr/003-streaming-pipeline-and-flow-execution',
        'adr/004-format-system',
        'adr/005-plugin-system',
        'adr/006-java-bridge',
        'adr/007-tool-system',
        'adr/008-configuration',
        'adr/009-ai-integration',
        'adr/010-translation-memory',
        'adr/011-kaz-archive-format',
        'adr/012-bowrain-desktop-app',
      ],
    },
  ],
};

export default sidebars;
