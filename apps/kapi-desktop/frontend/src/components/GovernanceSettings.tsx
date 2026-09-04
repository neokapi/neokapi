// The governance half of Project Settings: what governs the project's content,
// and where that content sits.
//
// Recipe keys nothing else could reach live here: `defaults.voice`,
// `defaults.coordinates`, `defaults.flow` and `exclude`. The point and the
// voice binding are the shared forms from `@neokapi/ui-primitives`, fed the
// recipe's vocabulary; the structural axes (product, channel) are refused with
// the refusal the recipe itself gives, served by the backend from
// project.DeclarableAxis so this editor cannot offer an axis `kapi apply`
// would reject.

import { useQuery } from "@tanstack/react-query";
import { Compass, Workflow } from "lucide-react";
import {
  Card,
  CardContent,
  CoordinatesEditor,
  Label,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  SectionHeading,
  TagInput,
  VoiceBindingSelect,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { call } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { ChannelMap } from "./channels/ChannelMap";
import { decodeVoiceBinding, encodeVoiceBinding, voiceBindingOptions } from "./voiceBinding";
import type { KapiProject, ProjectDefaults } from "../types/api";

/** One axis a recipe can put a coordinate on. */
export interface RecipeAxis {
  axis: string;
  declarable: boolean;
  refusal?: string;
  values?: string[];
  used?: string;
}

/** What the recipe can say about where content sits and what governs it. */
export interface RecipeGovernance {
  axes: RecipeAxis[];
  channels: string[];
  profiles: string[];
  voice_files: string[];
  packs: string[];
}

export interface GovernanceSettingsProps {
  tabID: string;
  project: KapiProject;
  onUpdate: (project: KapiProject) => void;
  /** Injected in tests and stories; production reads the Wails backend. */
  governance?: RecipeGovernance;
}

const NONE = "__none__";

export function GovernanceSettings({
  tabID,
  project,
  onUpdate,
  governance: injected,
}: GovernanceSettingsProps) {
  const query = useQuery({
    queryKey: qk.recipeGovernance(tabID),
    queryFn: () => call<RecipeGovernance>("RecipeGovernance", tabID),
    enabled: !injected && !!tabID,
  });
  const governance: RecipeGovernance = injected ??
    query.data ?? { axes: [], channels: [], profiles: [], voice_files: [], packs: [] };

  const defaults = project.defaults ?? {};
  const updateDefaults = (patch: Partial<ProjectDefaults>) =>
    onUpdate({ ...project, defaults: { ...defaults, ...patch } });

  const flows = Object.keys(project.flows ?? {});
  const structural = governance.axes.filter((a) => !a.declarable);

  return (
    <>
      {/* Where content sits: the project's own point, and the channel map. */}
      <section>
        <SectionHeading className="mb-3" icon={<Compass size={14} />}>
          Where content sits
        </SectionHeading>
        <Card>
          <CardContent className="space-y-4 p-4">
            <CoordinatesEditor
              value={defaults.coordinates ?? {}}
              axes={governance.axes}
              allowNewAxis
              label={t("Default point")}
              emptyText={t("Every collection sits at the project's own point.")}
              note={t("{axes} come from a collection's channel.", {
                axes: structural.map((a) => a.axis).join(" and "),
              })}
              onChange={(coordinates) => updateDefaults({ coordinates })}
            />

            <VoiceBindingSelect
              value={encodeVoiceBinding(defaults.voice)}
              options={voiceBindingOptions(governance)}
              inheritLabel={t("None bound")}
              label={t("Voice profile")}
              help={t("A file is edited on the Voice page. A starter pack is read-only.")}
              onChange={(key) => updateDefaults({ voice: decodeVoiceBinding(key) })}
            />

            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">{t("Channels")}</Label>
              <ChannelMap tabID={tabID} onUpdate={onUpdate} />
            </div>
          </CardContent>
        </Card>
      </section>

      {/* What runs, and what is skipped. */}
      <section>
        <SectionHeading className="mb-3" icon={<Workflow size={14} />}>
          What runs, and what is skipped
        </SectionHeading>
        <Card>
          <CardContent className="space-y-4 p-4">
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">
                {t("Default flow")}
              </Label>
              <Select
                value={defaults.flow || NONE}
                onValueChange={(v) => updateDefaults({ flow: v === NONE ? undefined : v })}
              >
                <SelectTrigger className="max-w-xs" aria-label={t("Default flow")}>
                  <SelectValue placeholder={t("Chosen per run")} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>{t("Chosen per run")}</SelectItem>
                  {flows.map((f) => (
                    <SelectItem key={f} value={f}>
                      {f}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="mt-1 text-[10px] text-muted-foreground">
                {t("The default flow a run uses when it names no other one.")}
              </p>
            </div>

            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">
                {t("Skip these paths")}
              </Label>
              <TagInput
                value={defaults.exclude ?? []}
                onChange={(exclude) =>
                  updateDefaults({ exclude: exclude.length ? exclude : undefined })
                }
                placeholder={t("Add a pattern, e.g. **/vendor/**")}
              />
              <p className="mt-1 text-[10px] text-muted-foreground">
                {t("Glob patterns a content scan passes over.")}
              </p>
            </div>
          </CardContent>
        </Card>
      </section>
    </>
  );
}
