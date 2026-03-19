export interface ProjectLanguage {
  locale: string;
  label: string;
  progress: number;
  blocksTotal: number;
  blocksTranslated: number;
}

export interface Project {
  name: string;
  upstream: string;
  fork: string;
  license: string;
  languages: ProjectLanguage[];
  tmReuseRate: number;
  formatTypes: string[];
}

export const projects: Project[] = [
  {
    name: "Capacitor",
    upstream: "ionic-team/capacitor",
    fork: "bowrain-test/capacitor",
    license: "MIT",
    languages: [
      { locale: "fr-FR", label: "French", progress: 78, blocksTotal: 1240, blocksTranslated: 967 },
      { locale: "de-DE", label: "German", progress: 65, blocksTotal: 1240, blocksTranslated: 806 },
      { locale: "ja-JP", label: "Japanese", progress: 42, blocksTotal: 1240, blocksTranslated: 521 },
    ],
    tmReuseRate: 34,
    formatTypes: ["JSON", "MD"],
  },
  {
    name: "Flipt",
    upstream: "flipt-io/flipt",
    fork: "bowrain-test/flipt",
    license: "GPL-3.0",
    languages: [
      { locale: "fr-FR", label: "French", progress: 85, blocksTotal: 890, blocksTranslated: 757 },
      { locale: "de-DE", label: "German", progress: 72, blocksTotal: 890, blocksTranslated: 641 },
      { locale: "ja-JP", label: "Japanese", progress: 51, blocksTotal: 890, blocksTranslated: 454 },
    ],
    tmReuseRate: 41,
    formatTypes: ["JSON", "YAML"],
  },
  {
    name: "Listmonk",
    upstream: "knadh/listmonk",
    fork: "bowrain-test/listmonk",
    license: "AGPL-3.0",
    languages: [
      { locale: "fr-FR", label: "French", progress: 91, blocksTotal: 645, blocksTranslated: 587 },
      { locale: "de-DE", label: "German", progress: 83, blocksTotal: 645, blocksTranslated: 535 },
      { locale: "ja-JP", label: "Japanese", progress: 68, blocksTotal: 645, blocksTranslated: 439 },
    ],
    tmReuseRate: 52,
    formatTypes: ["JSON"],
  },
  {
    name: "Infisical",
    upstream: "Infisical/infisical",
    fork: "bowrain-test/infisical",
    license: "MIT",
    languages: [
      { locale: "fr-FR", label: "French", progress: 62, blocksTotal: 2180, blocksTranslated: 1352 },
      { locale: "de-DE", label: "German", progress: 48, blocksTotal: 2180, blocksTranslated: 1046 },
      { locale: "ja-JP", label: "Japanese", progress: 31, blocksTotal: 2180, blocksTranslated: 676 },
    ],
    tmReuseRate: 28,
    formatTypes: ["JSON", "MD", "INI"],
  },
];
