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
            'kapi-cli/commands/tm',
            'kapi-cli/commands/word-count',
          ],
        },
        {
          type: 'category',
          label: 'Use Cases',
          items: [
            'kapi-cli/use-cases/terminology-qa',
            'kapi-cli/use-cases/terminology-pretranslation',
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
      label: 'Format Reference',
      href: '/formats',
    },
    {
      type: 'link',
      label: 'Benchmarks',
      href: '/pseudobench',
    },
    {
      type: 'link',
      label: 'Test Results',
      href: '/test-comparison',
    },
  ],
};

export default sidebars;
