/**
 * Shared fixtures for concept/terminology graph stories.
 *
 * Realistic terminology data modelled after a localization platform —
 * concepts span Security, Platform, Translation, Integration, and Brand
 * domains with multilingual terms and SKOS-aligned graph relationships.
 */

import type { ConceptInfo, ConceptHierarchyNode, GraphEdge, GraphNode } from "../../types/api";

// ---------------------------------------------------------------------------
// Concepts
// ---------------------------------------------------------------------------

export const concepts: ConceptInfo[] = [
  {
    id: "c-auth",
    domain: "Security",
    definition: "The process of verifying a user's identity before granting access to the system.",
    terms: [
      { text: "Authentication", locale: "en-US", status: "preferred" },
      { text: "Login", locale: "en-US", status: "admitted" },
      { text: "Authentifizierung", locale: "de-DE", status: "preferred" },
      { text: "Anmeldung", locale: "de-DE", status: "admitted" },
      { text: "Authentification", locale: "fr-FR", status: "preferred" },
    ],
    created_at: "2024-11-15T10:30:00Z",
    updated_at: "2025-02-20T14:22:00Z",
  },
  {
    id: "c-authz",
    domain: "Security",
    definition: "Determining what actions a verified user is permitted to perform.",
    terms: [
      { text: "Authorization", locale: "en-US", status: "preferred" },
      { text: "Permissions", locale: "en-US", status: "admitted" },
      { text: "Autorisierung", locale: "de-DE", status: "preferred" },
      { text: "Autorisation", locale: "fr-FR", status: "preferred" },
    ],
    created_at: "2024-11-15T10:35:00Z",
    updated_at: "2025-01-10T09:15:00Z",
  },
  {
    id: "c-workspace",
    domain: "Platform",
    definition: "A shared organizational context containing projects, members, and configuration.",
    terms: [
      { text: "Workspace", locale: "en-US", status: "preferred" },
      { text: "Organization", locale: "en-US", status: "deprecated" },
      { text: "Arbeitsbereich", locale: "de-DE", status: "preferred" },
      { text: "Espace de travail", locale: "fr-FR", status: "preferred" },
    ],
    created_at: "2024-10-01T08:00:00Z",
    updated_at: "2025-03-01T16:45:00Z",
  },
  {
    id: "c-project",
    domain: "Platform",
    definition:
      "A localization project containing source files, target languages, and translation workflows.",
    terms: [
      { text: "Project", locale: "en-US", status: "preferred" },
      { text: "Projekt", locale: "de-DE", status: "preferred" },
      { text: "Projet", locale: "fr-FR", status: "preferred" },
    ],
    project_id: "proj-1",
    created_at: "2024-10-05T09:00:00Z",
    updated_at: "2025-02-28T11:30:00Z",
  },
  {
    id: "c-tm",
    domain: "Translation",
    definition:
      "A database of previously translated segments used to improve consistency and speed.",
    terms: [
      { text: "Translation Memory", locale: "en-US", status: "preferred" },
      { text: "TM", locale: "en-US", status: "admitted" },
      { text: "Übersetzungsspeicher", locale: "de-DE", status: "preferred" },
      { text: "Mémoire de traduction", locale: "fr-FR", status: "preferred" },
    ],
    created_at: "2024-09-20T12:00:00Z",
    updated_at: "2025-01-15T08:20:00Z",
  },
  {
    id: "c-termbase",
    domain: "Translation",
    definition:
      "A structured glossary of approved terminology ensuring consistent use across languages.",
    terms: [
      { text: "Termbase", locale: "en-US", status: "preferred" },
      { text: "Glossary", locale: "en-US", status: "admitted" },
      { text: "Terminologiedatenbank", locale: "de-DE", status: "preferred" },
      { text: "Base terminologique", locale: "fr-FR", status: "preferred" },
    ],
    created_at: "2024-09-20T12:15:00Z",
    updated_at: "2025-03-10T10:00:00Z",
  },
  {
    id: "c-segment",
    domain: "Translation",
    definition: "A unit of text (sentence or paragraph) that is translated as a whole.",
    terms: [
      { text: "Segment", locale: "en-US", status: "preferred" },
      { text: "Block", locale: "en-US", status: "admitted" },
      { text: "Segment", locale: "de-DE", status: "preferred" },
      { text: "Segment", locale: "fr-FR", status: "preferred" },
    ],
    created_at: "2024-11-01T14:00:00Z",
    updated_at: "2024-12-20T09:30:00Z",
  },
  {
    id: "c-connector",
    domain: "Integration",
    definition:
      "A plugin that synchronizes content between Bowrain and an external system (CMS, repository).",
    terms: [
      { text: "Connector", locale: "en-US", status: "preferred" },
      { text: "Integration", locale: "en-US", status: "deprecated" },
      { text: "Konnektor", locale: "de-DE", status: "preferred" },
      { text: "Connecteur", locale: "fr-FR", status: "preferred" },
    ],
    created_at: "2025-01-05T10:00:00Z",
    updated_at: "2025-03-15T13:00:00Z",
  },
  {
    id: "c-brand-voice",
    domain: "Brand",
    definition:
      "The distinctive personality and tone expressed through language in all communications.",
    terms: [
      { text: "Brand Voice", locale: "en-US", status: "preferred" },
      { text: "Tone of Voice", locale: "en-US", status: "admitted" },
      { text: "Markenstimme", locale: "de-DE", status: "preferred" },
      { text: "Voix de marque", locale: "fr-FR", status: "preferred" },
    ],
    created_at: "2025-02-01T11:00:00Z",
    updated_at: "2025-03-20T15:30:00Z",
  },
];

// ---------------------------------------------------------------------------
// Hierarchy (from /graph/concepts endpoint)
// ---------------------------------------------------------------------------

export const hierarchy: ConceptHierarchyNode[] = concepts.map((c) => ({
  id: c.id,
  label: "Concept",
  properties: { name: c.terms[0]?.text ?? c.id },
  children: c.id === "c-workspace" ? 1 : c.id === "c-tm" ? 1 : 0,
  parents: c.id === "c-project" ? 1 : c.id === "c-termbase" ? 1 : 0,
}));

// ---------------------------------------------------------------------------
// Graph edges (SKOS-aligned relationships)
// ---------------------------------------------------------------------------

export const graphEdges: GraphEdge[] = [
  {
    id: "e-1",
    source: "c-auth",
    target: "c-authz",
    label: "RELATED",
    properties: {},
    created_at: "2024-11-15T10:40:00Z",
    updated_at: "2024-11-15T10:40:00Z",
  },
  {
    id: "e-2",
    source: "c-workspace",
    target: "c-project",
    label: "HAS_PART",
    properties: {},
    created_at: "2024-10-05T09:10:00Z",
    updated_at: "2024-10-05T09:10:00Z",
  },
  {
    id: "e-3",
    source: "c-project",
    target: "c-workspace",
    label: "PART_OF",
    properties: {},
    created_at: "2024-10-05T09:10:00Z",
    updated_at: "2024-10-05T09:10:00Z",
  },
  {
    id: "e-4",
    source: "c-tm",
    target: "c-segment",
    label: "BROADER",
    properties: {},
    created_at: "2024-11-01T14:10:00Z",
    updated_at: "2024-11-01T14:10:00Z",
  },
  {
    id: "e-5",
    source: "c-tm",
    target: "c-termbase",
    label: "RELATED",
    properties: {},
    created_at: "2024-09-20T12:30:00Z",
    updated_at: "2024-09-20T12:30:00Z",
  },
  {
    id: "e-6",
    source: "c-brand-voice",
    target: "c-termbase",
    label: "RELATED",
    properties: {},
    created_at: "2025-02-01T11:10:00Z",
    updated_at: "2025-02-01T11:10:00Z",
  },
  {
    id: "e-7",
    source: "c-connector",
    target: "c-project",
    label: "RELATED",
    properties: {},
    created_at: "2025-01-05T10:20:00Z",
    updated_at: "2025-01-05T10:20:00Z",
  },
];

// ---------------------------------------------------------------------------
// Graph neighbor nodes (for ConceptDetailPanel)
// ---------------------------------------------------------------------------

export const neighborNodes: GraphNode[] = concepts.map((c) => ({
  id: c.id,
  label: "Concept",
  properties: { name: c.terms[0]?.text ?? c.id },
  created_at: c.created_at,
  updated_at: c.updated_at,
}));

// ---------------------------------------------------------------------------
// Edges for a specific concept (helper)
// ---------------------------------------------------------------------------

export function edgesForConcept(conceptId: string): GraphEdge[] {
  return graphEdges.filter((e) => e.source === conceptId || e.target === conceptId);
}
