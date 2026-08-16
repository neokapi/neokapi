import type { Config } from "@docusaurus/types";

// The i18n block a docs site needs for kapi's output to be picked up: the
// locales here are the recipe's source_language plus its target_languages, and
// the path kapi writes (i18n/<locale>/docusaurus-plugin-content-docs/current/)
// is Docusaurus's own default translation path. No plugin, no copy step.
//
// onBrokenLinks is where the source-strict / target-warn policy lives. A broken
// link in the source is an error, because somebody wrote it; a broken link in a
// target is a warning, because it is drift the loop has not caught up with yet
// and a build that fails on it turns the ordinary state of a translated site
// into an outage.
//
// DOCUSAURUS_CURRENT_LOCALE is the variable Docusaurus sets, and it is set for
// every build — including the default one, where it holds `defaultLocale`. The
// fallback below therefore never decides the policy in a Docusaurus build; it
// only keeps the expression honest if the config is read by something else.
// Reading a variable Docusaurus does not set fails in the permissive direction:
// the comparison is always false, every locale warns, and a broken link in the
// source ships.
const sourceLocale = "en-GB";
const linkIntegrity =
  (process.env.DOCUSAURUS_CURRENT_LOCALE ?? sourceLocale) === sourceLocale ? "throw" : "warn";

const config: Config = {
  title: "Tidewatch",
  tagline: "Forecast against constraint, at every berth",
  url: "https://docs.northsea.example",
  baseUrl: "/",
  onBrokenLinks: linkIntegrity,
  i18n: {
    defaultLocale: sourceLocale,
    locales: [sourceLocale, "nb", "nl"],
  },
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: linkIntegrity,
      onBrokenMarkdownImages: linkIntegrity,
    },
  },
  presets: [
    [
      "classic",
      {
        docs: { routeBasePath: "/", sidebarPath: "./sidebars.ts" },
        blog: false,
      },
    ],
  ],
};

export default config;
