/**
 * "Acme CloudOps" story seed data constants.
 * Used by the layered seeder to populate a realistic workspace for e2e tests.
 */

import type { Concept, TMEntry } from "./api-client.js";

// ---------------------------------------------------------------------------
// Workspace
// ---------------------------------------------------------------------------

export const WORKSPACE = {
  name: "Acme CloudOps",
  slugPrefix: "acme-cloudops",
};

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

export interface ProjectDef {
  name: string;
  sourceLanguage: string;
  targetLanguages: string[];
  files: string[];
}

export const PROJECTS: ProjectDef[] = [
  {
    name: "Company Website",
    sourceLanguage: "en",
    targetLanguages: ["fr", "de", "ja"],
    files: ["about-us.html", "landing-page.html"],
  },
  {
    name: "Dashboard App",
    sourceLanguage: "en",
    targetLanguages: ["fr", "de"],
    files: ["app-strings.json", "nav-strings.json"],
  },
  {
    name: "Release Notes v3.2",
    sourceLanguage: "en",
    targetLanguages: ["fr"],
    files: ["release-notes.md"],
  },
  {
    name: "API Documentation",
    sourceLanguage: "en",
    targetLanguages: ["ja"],
    files: ["api-reference.html"],
  },
];

// ---------------------------------------------------------------------------
// TM Entries
// ---------------------------------------------------------------------------

export const TM_ENTRIES: TMEntry[] = [
  // en -> fr
  {
    source: "About Acme Inc.",
    target: "A propos d'Acme Inc.",
    source_locale: "en",
    target_locale: "fr",
  },
  { source: "Our Mission", target: "Notre mission", source_locale: "en", target_locale: "fr" },
  { source: "Our Team", target: "Notre equipe", source_locale: "en", target_locale: "fr" },
  { source: "Our Values", target: "Nos valeurs", source_locale: "en", target_locale: "fr" },
  { source: "Get in Touch", target: "Contactez-nous", source_locale: "en", target_locale: "fr" },
  { source: "Dashboard", target: "Tableau de bord", source_locale: "en", target_locale: "fr" },
  { source: "Settings", target: "Parametres", source_locale: "en", target_locale: "fr" },
  { source: "Sign Out", target: "Deconnexion", source_locale: "en", target_locale: "fr" },
  {
    source: "Something went wrong. Please try again later.",
    target: "Une erreur est survenue. Veuillez reessayer plus tard.",
    source_locale: "en",
    target_locale: "fr",
  },
  {
    source: "Your session has expired. Please sign in again.",
    target: "Votre session a expire. Veuillez vous reconnecter.",
    source_locale: "en",
    target_locale: "fr",
  },
  {
    source: "We believe every developer deserves reliable, fast, and secure infrastructure.",
    target:
      "Nous croyons que chaque developpeur merite une infrastructure fiable, rapide et securisee.",
    source_locale: "en",
    target_locale: "fr",
  },
  {
    source: "Are you sure you want to delete this project? This action cannot be undone.",
    target: "Etes-vous sur de vouloir supprimer ce projet ? Cette action est irreversible.",
    source_locale: "en",
    target_locale: "fr",
  },
  // en -> de
  { source: "About Acme Inc.", target: "Uber Acme Inc.", source_locale: "en", target_locale: "de" },
  { source: "Our Mission", target: "Unsere Mission", source_locale: "en", target_locale: "de" },
  { source: "Dashboard", target: "Dashboard", source_locale: "en", target_locale: "de" },
  { source: "Settings", target: "Einstellungen", source_locale: "en", target_locale: "de" },
];

// ---------------------------------------------------------------------------
// Terminology Concepts
// ---------------------------------------------------------------------------

export const CONCEPTS: Concept[] = [
  {
    domain: "cloud",
    definition: "A graphical overview of key metrics and status indicators",
    terms: [
      { text: "dashboard", locale: "en", status: "preferred" },
      { text: "tableau de bord", locale: "fr", status: "preferred" },
      { text: "Dashboard", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "cloud",
    definition:
      "The process of releasing a new version of an application to a server or hosting environment",
    terms: [
      { text: "deployment", locale: "en", status: "preferred" },
      { text: "deploiement", locale: "fr", status: "preferred" },
      { text: "Bereitstellung", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "cloud",
    definition: "Physical or virtual compute capacity in a cloud environment",
    terms: [
      { text: "instance", locale: "en", status: "preferred" },
      { text: "instance", locale: "fr", status: "preferred" },
      { text: "Instanz", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "cloud",
    definition: "Automatic adjustment of compute capacity based on demand",
    terms: [
      { text: "auto-scaling", locale: "en", status: "preferred" },
      { text: "mise a l'echelle automatique", locale: "fr", status: "approved" },
      { text: "Auto-Skalierung", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "cloud",
    definition: "A geographic area containing one or more data centers",
    terms: [
      { text: "region", locale: "en", status: "preferred" },
      { text: "region", locale: "fr", status: "preferred" },
      { text: "Region", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "cloud",
    definition: "The ability to automatically redirect traffic when a primary system fails",
    terms: [
      { text: "failover", locale: "en", status: "preferred" },
      { text: "basculement", locale: "fr", status: "preferred" },
      { text: "Failover", locale: "de", status: "approved" },
    ],
  },
  {
    domain: "security",
    definition: "A chronological record of system activities for security review",
    terms: [
      { text: "audit log", locale: "en", status: "preferred" },
      { text: "journal d'audit", locale: "fr", status: "preferred" },
      { text: "Audit-Protokoll", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "ui",
    definition: "Application configuration and customization options",
    terms: [
      { text: "settings", locale: "en", status: "preferred" },
      { text: "parametres", locale: "fr", status: "preferred" },
      { text: "Einstellungen", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "cloud",
    definition: "Runtime configuration values provided to an application",
    terms: [
      { text: "environment variable", locale: "en", status: "preferred" },
      { text: "variable d'environnement", locale: "fr", status: "preferred" },
      { text: "Umgebungsvariable", locale: "de", status: "preferred" },
    ],
  },
  {
    domain: "cloud",
    definition:
      "A lightweight, standalone executable package that includes everything needed to run software",
    terms: [
      { text: "container", locale: "en", status: "preferred" },
      { text: "conteneur", locale: "fr", status: "preferred" },
      { text: "Container", locale: "de", status: "preferred" },
    ],
  },
];

// ---------------------------------------------------------------------------
// Brand Profile
// ---------------------------------------------------------------------------

export const BRAND_PROFILE = {
  pack: "professional",
  name: "Acme CloudOps Voice",
};

// ---------------------------------------------------------------------------
// Task Definitions
// ---------------------------------------------------------------------------

export interface TaskDef {
  title: string;
  description?: string;
  type?: string;
  priority?: string;
  /** Index into PROJECTS array to associate with a project */
  projectIndex?: number;
}

export const TASKS: TaskDef[] = [
  {
    title: "Review French translations for Company Website",
    description:
      "Check all French translations for the about-us and landing pages. Pay attention to brand terminology.",
    type: "review",
    priority: "high",
    projectIndex: 0,
  },
  {
    title: "Translate Dashboard App to German",
    description: "Complete the German translations for all app strings and navigation.",
    type: "translation",
    priority: "medium",
    projectIndex: 1,
  },
  {
    title: "Verify release notes terminology",
    description: "Ensure release notes use approved terminology from the termbase.",
    type: "review",
    priority: "low",
    projectIndex: 2,
  },
  {
    title: "Translate API docs to Japanese",
    description: "First pass translation of the API reference documentation into Japanese.",
    type: "translation",
    priority: "high",
    projectIndex: 3,
  },
];

// ---------------------------------------------------------------------------
// Automation Rules
// ---------------------------------------------------------------------------

export interface AutomationRuleDef {
  name: string;
  trigger: string;
  conditions: unknown[];
  actions: unknown[];
  enabled: boolean;
  /** Index into PROJECTS array */
  projectIndex: number;
}

export const AUTOMATION_RULES: AutomationRuleDef[] = [
  {
    name: "Auto-pseudo on upload",
    trigger: "file.uploaded",
    conditions: [],
    actions: [{ type: "pseudo_translate" }],
    enabled: true,
    projectIndex: 0,
  },
  {
    name: "TM pre-translate on upload",
    trigger: "file.uploaded",
    conditions: [],
    actions: [{ type: "tm_pretranslate", min_match: 80 }],
    enabled: true,
    projectIndex: 1,
  },
];

// ---------------------------------------------------------------------------
// Stream Definition
// ---------------------------------------------------------------------------

export const STREAM = {
  name: "feature/french-review",
  description: "Stream for reviewing and updating French translations",
};
