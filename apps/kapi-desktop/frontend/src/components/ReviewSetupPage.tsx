// Review in a project whose recipe declares no target language.
//
// Review holds translations to approve, so it has nothing to show until there
// is a language to translate into. The page says so and offers the source and
// target language settings from Project Settings in place, so the first
// target can be added without leaving the page. Once the saved recipe declares
// one, the same route opens the review itself (ViewSwitch).

import { PageHeader } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { KapiProject } from "../types/api";
import { LanguagesSection } from "./ProjectSettingsPage";

export interface ReviewSetupPageProps {
  project: KapiProject;
  onUpdate: (project: KapiProject) => void;
}

/** Review's empty state: one line and the languages section. */
export function ReviewSetupPage({ project, onUpdate }: ReviewSetupPageProps) {
  return (
    <div className="p-6" data-slot="review-setup-page">
      <PageHeader
        title={t("Review")}
        subtitle={t(
          "Add a language to translate into. Translations to review appear here, with content memory and coverage.",
        )}
      />
      <div className="max-w-2xl">
        <LanguagesSection project={project} onUpdate={onUpdate} untitled />
      </div>
    </div>
  );
}
