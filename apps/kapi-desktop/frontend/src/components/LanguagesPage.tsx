// The Languages door of a project whose recipe declares no target language.
//
// The rail's "Languages +" opens it: the add-a-language part of Project
// Settings, alone, under one line naming what the first target language
// brings. Once the saved recipe declares one, the same entry opens the
// Project page's language standing instead (ViewSwitch).

import { PageHeader } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { KapiProject } from "../types/api";
import { LanguagesSection } from "./ProjectSettingsPage";

export interface LanguagesPageProps {
  project: KapiProject;
  onUpdate: (project: KapiProject) => void;
}

/** A settings-and-setup page: one section, the source and target languages. */
export function LanguagesPage({ project, onUpdate }: LanguagesPageProps) {
  return (
    <div className="p-6" data-slot="languages-page">
      <PageHeader
        title={t("Languages")}
        subtitle={t(
          "Add a language to translate into. Review, content memory and coverage appear with the first one.",
        )}
      />
      <div className="max-w-2xl">
        <LanguagesSection project={project} onUpdate={onUpdate} untitled />
      </div>
    </div>
  );
}
