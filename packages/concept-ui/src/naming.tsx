// Concept naming, shared down a view's subtree (Apache-2.0). `primaryName` stays
// pure so the rule is unit-tested directly; this carries the locale hints the
// rule needs from the app that knows them (the desktop's project source locale,
// a viewer's UI language) to every panel that draws a concept's name, without
// each panel taking a prop it only forwards.
import { createContext, useContext, useMemo, type ReactNode } from "react";
import { primaryName, type ConceptNaming } from "./concept-meta";
import type { ConceptSummary } from "./types";

const EMPTY: ConceptNaming = {};

const ConceptNamingContext = createContext<ConceptNaming>(EMPTY);

export function ConceptNamingProvider({
  naming,
  children,
}: {
  naming?: ConceptNaming;
  children: ReactNode;
}) {
  const sourceLocale = naming?.sourceLocale;
  const uiLocale = naming?.uiLocale;
  const value = useMemo<ConceptNaming>(
    () => (sourceLocale || uiLocale ? { sourceLocale, uiLocale } : EMPTY),
    [sourceLocale, uiLocale],
  );
  return <ConceptNamingContext value={value}>{children}</ConceptNamingContext>;
}

/** The locale hints in force here. Empty outside a provider. */
export function useConceptNaming(): ConceptNaming {
  return useContext(ConceptNamingContext);
}

/** The concept's display name under the locale hints in force here. */
export function useConceptName(concept: ConceptSummary): string {
  return primaryName(concept, useConceptNaming());
}
