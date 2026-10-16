// What an assistant is told about one Fernwell file: the files the picker
// offers, and the brief for the help center's interface strings.

import type { AgentContextView } from "../../types/api";

/** The file the brief opens on when Ask or Checks hand the panel a path. */
export const BRIEF_PATH = "src/en/app-ui.json";

/** The files the picker offers, each with the collection that claims it. */
export const BRIEF_FILES = [
  { value: "docs/getting-started.md", label: "docs" },
  { value: BRIEF_PATH, label: "app-ui" },
  { value: "src/en/errors.json", label: "app-ui" },
];

/** The brief for the interface strings: a voice, two terms, and the body as an agent reads it. */
export const BRIEF_VIEW: AgentContextView = {
  path: BRIEF_PATH,
  answer: {
    point: {
      path: BRIEF_PATH,
      collection: "app-ui",
      ref: "app-ui@ui",
      coordinates: { product: "fernwell", channel: "ui" },
      default: false,
    },
    scope: "project",
    voice: { name: "Fernwell help center", source: "voice/help-center.yaml" },
    terms: [
      {
        concept_id: "c-studio",
        term: "business",
        discouraged: true,
        replacement: "studio",
      },
      {
        concept_id: "c-sign-in",
        term: "log in",
        discouraged: true,
        replacement: "sign in",
      },
    ],
    terms_total: 2,
  },
  text: `# Writing src/en/app-ui.json

Voice: Fernwell help center. Plain, warm, second person (you).

Say this, not that:
- studio, not "business"
- sign in, not "log in"`,
};
