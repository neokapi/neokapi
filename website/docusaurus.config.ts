import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'gokapi',
  tagline: 'Open, AI-native localization platform in Go',
  favicon: 'img/favicon.ico',

  url: 'https://gokapi.github.io',
  baseUrl: '/',

  organizationName: 'gokapi',
  projectName: 'gokapi',
  trailingSlash: false,

  onBrokenLinks: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: 'warn',
      onBrokenMarkdownImages: 'warn',
    },
  },

  themes: ['@docusaurus/theme-mermaid'],

  plugins: [
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'ad',
        path: '../docs/ad',
        routeBasePath: 'docs/ad',
        sidebarPath: './sidebars-ad.ts',
        editUrl: 'https://github.com/gokapi/gokapi/tree/main/',
      },
    ],
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'notes',
        path: '../docs/notes',
        routeBasePath: 'docs/notes',
        sidebarPath: './sidebars-notes.ts',
        editUrl: 'https://github.com/gokapi/gokapi/tree/main/',
      },
    ],
  ],

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/gokapi/gokapi/tree/main/website/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    navbar: {
      title: 'gokapi',
      logo: {
        alt: 'gokapi',
        src: 'img/logo.png',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'userGuide',
          position: 'left',
          label: 'User Guide',
        },
        {
          type: 'docSidebar',
          sidebarId: 'developer',
          position: 'left',
          label: 'For Developers',
        },
        {
          href: '/storybook/',
          label: 'Storybook',
          position: 'left',
        },
        {
          href: 'https://github.com/gokapi/gokapi',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {
              label: 'Getting Started',
              to: '/docs/getting-started/introduction',
            },
            {
              label: 'User Guide',
              to: '/docs/kapi-cli/overview',
            },
            {
              label: 'For Developers',
              to: '/docs/developer/architecture',
            },
          ],
        },
        {
          title: 'More',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/gokapi/gokapi',
            },
            {
              label: 'Homebrew Tap',
              href: 'https://github.com/gokapi/homebrew-tap',
            },
          ],
        },
      ],
      copyright: `Copyright \u00a9 ${new Date().getFullYear()} gokapi contributors. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['go', 'protobuf', 'yaml', 'bash', 'json'],
    },
    mermaid: {
      theme: {light: 'neutral', dark: 'dark'},
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
