// The locale hints the Brand hub names concepts by (AGPL-3.0). Both the list and
// one concept's dashboard read them from here, so a concept keeps its heading
// when you open it.
import { useMemo } from "react";
import { useNeokapi } from "@neokapi/i18n-react/runtime";
import type { ConceptNaming } from "@neokapi/concept-ui";
import { useProjects } from "../../hooks/useProjectApi";
import { workspaceSourceLocale } from "./workspace-source-locale";

/**
 * Name a concept in the language the workspace authors in, or in the language
 * the viewer reads Bowrain in. With neither available, English wins, and after
 * English a fixed order over the concept's terms.
 */
export function useWorkspaceConceptNaming(): ConceptNaming {
  const { data: projects } = useProjects();
  const { locale } = useNeokapi();
  const sourceLocale = useMemo(() => workspaceSourceLocale(projects), [projects]);
  return useMemo(
    () => ({ sourceLocale: sourceLocale || undefined, uiLocale: locale }),
    [sourceLocale, locale],
  );
}
