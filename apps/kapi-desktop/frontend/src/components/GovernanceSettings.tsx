// The governance half of Project Settings: what governs the project's content,
// and where that content sits.
//
// Recipe keys nothing else could reach live here — `defaults.voice`,
// `defaults.coordinates`, `defaults.flow` and `exclude`. The structural axes
// (product, channel) are refused with the refusal the recipe itself gives,
// served by the backend from project.DeclarableAxis so this editor cannot offer
// an axis `kapi apply` would reject.

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Compass, Plus, Trash2, Workflow } from "lucide-react";
import {
  Button,
  Card,
  CardContent,
  Input,
  Label,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  SectionHeading,
  TagInput,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { call } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { ChannelMap } from "./channels/ChannelMap";
import type { KapiProject, ProjectDefaults, VoiceBindingSpec } from "../types/api";

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

/** The three forms a voice binding takes, as one picker. */
function VoiceBindingField({
  binding,
  onChange,
  governance,
}: {
  binding?: VoiceBindingSpec;
  onChange: (next: VoiceBindingSpec | undefined) => void;
  governance: RecipeGovernance;
}) {
  const current = binding?.profile_file
    ? `file:${binding.profile_file}`
    : binding?.pack
      ? `pack:${binding.pack}`
      : binding?.profile
        ? `store:${binding.profile}`
        : NONE;

  return (
    <div className="space-y-1">
      <Label className="mb-1 block text-xs text-muted-foreground">{t("Voice profile")}</Label>
      <Select
        value={current}
        onValueChange={(v) => {
          if (v === NONE) return onChange(undefined);
          const [kind, ...rest] = v.split(":");
          const value = rest.join(":");
          if (kind === "file") return onChange({ profile_file: value });
          if (kind === "pack") return onChange({ pack: value });
          onChange({ profile: value });
        }}
      >
        <SelectTrigger className="max-w-md" aria-label={t("Voice profile")}>
          <SelectValue placeholder={t("None bound")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NONE}>{t("None bound")}</SelectItem>
          {governance.voice_files.map((f) => (
            <SelectItem key={`file:${f}`} value={`file:${f}`}>
              {f}
            </SelectItem>
          ))}
          {governance.packs.map((p) => (
            <SelectItem key={`pack:${p}`} value={`pack:${p}`}>
              {t("starter pack: {name}", { name: p })}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-[10px] text-muted-foreground">
        {t("A file is edited on the Voice page. A starter pack is read-only.")}
      </p>
    </div>
  );
}

/** The declared axes of the project's default point. */
function CoordinatesField({
  coordinates,
  onChange,
  governance,
}: {
  coordinates: Record<string, string>;
  onChange: (next: Record<string, string> | undefined) => void;
  governance: RecipeGovernance;
}) {
  const [axis, setAxis] = useState("");
  const rows = Object.entries(coordinates);
  const structural = governance.axes.filter((a) => !a.declarable);
  const refusal = structural.find((a) => a.axis === axis.trim())?.refusal;
  const canAdd = axis.trim() !== "" && !refusal && !(axis.trim() in coordinates);

  const set = (next: Record<string, string>) =>
    onChange(Object.keys(next).length ? next : undefined);

  return (
    <div className="space-y-2" data-testid="coordinates-editor">
      <Label className="block text-xs text-muted-foreground">{t("Default point")}</Label>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("Every collection sits at the project's own point.")}
        </p>
      ) : (
        <ul className="space-y-1">
          {rows.map(([a, value]) => {
            const known = governance.axes.find((x) => x.axis === a);
            return (
              <li key={a} className="flex items-center gap-2" data-testid="coordinate-row">
                <code className="w-28 shrink-0 font-mono text-xs">{a}</code>
                {known?.values?.length ? (
                  <Select value={value} onValueChange={(v) => set({ ...coordinates, [a]: v })}>
                    <SelectTrigger className="max-w-xs" aria-label={a}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {known.values.map((v) => (
                        <SelectItem key={v} value={v}>
                          {v}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                ) : (
                  <Input
                    className="max-w-xs"
                    value={value}
                    aria-label={a}
                    onChange={(e) => set({ ...coordinates, [a]: e.target.value })}
                  />
                )}
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("Remove {axis}", { axis: a })}
                  onClick={() => {
                    const next = { ...coordinates };
                    delete next[a];
                    set(next);
                  }}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </li>
            );
          })}
        </ul>
      )}

      <div className="flex items-center gap-2">
        <Input
          className="max-w-[12rem]"
          placeholder={t("axis, e.g. brand")}
          value={axis}
          aria-label={t("New axis")}
          onChange={(e) => setAxis(e.target.value)}
        />
        <Button
          variant="ghost"
          size="sm"
          disabled={!canAdd}
          onClick={() => {
            set({ ...coordinates, [axis.trim()]: "" });
            setAxis("");
          }}
        >
          <Plus className="mr-1 size-3" />
          {t("Add axis")}
        </Button>
      </div>
      {refusal && (
        <p className="text-xs text-destructive" data-testid="axis-refusal">
          {refusal}
        </p>
      )}
      <p className="text-[10px] text-muted-foreground">
        {t("{axes} come from a collection's channel.", {
          axes: structural.map((a) => a.axis).join(" and "),
        })}
      </p>
    </div>
  );
}

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

  return (
    <>
      {/* Where content sits: the project's own point, and the channel map. */}
      <section>
        <SectionHeading className="mb-3" icon={<Compass size={14} />}>
          Where content sits
        </SectionHeading>
        <Card>
          <CardContent className="space-y-4 p-4">
            <CoordinatesField
              coordinates={defaults.coordinates ?? {}}
              governance={governance}
              onChange={(coordinates) => updateDefaults({ coordinates })}
            />

            <VoiceBindingField
              binding={defaults.voice}
              governance={governance}
              onChange={(voice) => updateDefaults({ voice })}
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
