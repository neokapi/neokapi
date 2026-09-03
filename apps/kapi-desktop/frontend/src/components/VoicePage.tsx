// Context › Voice — the profile that governs each point, read whole.
//
// The voice profile decides what `kapi check` fails on, what a review finding
// says, and how an AI proposal is worded. The page answers three questions in
// the order a reader asks them: which point am I standing at, which profile
// governs there and why that one, and what does that profile actually say.
//
// The resolution header (`voice/resolution-header`) answers the second, with
// the fell-through chain kept prominent. The profile then reads as a document,
// wording rules first (`voice/rules`), because the say-this and never-say
// constraints are what a writer reaches for, before tone, style, examples and
// the per-language, per-channel and per-persona overrides.

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  FileText,
  Languages,
  ListChecks,
  MessageSquareQuote,
  Pencil,
  Radio,
  ShieldCheck,
  Sparkles,
  UserRound,
} from "lucide-react";
import {
  Badge,
  Button,
  CoordinateChip,
  LocaleLabel,
  PageHeader,
  SectionHeading,
  Separator,
  Skeleton,
  SimpleTooltip,
  cn,
} from "@neokapi/ui-primitives";
import { EmptyHint, ErrorHint } from "@neokapi/concept-ui";
import { t } from "@neokapi/i18n-react/runtime";
import { call } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import type {
  FieldValueSet,
  ProjectVoiceResult,
  StyleRules,
  ToneProfile,
  VoiceExample,
  VoicePoint,
  VoiceProfile,
  VoiceSaveResult,
} from "../types/voice";
import { VoiceProfileEditor } from "./voice/VoiceProfileEditor";
import { FactGrid } from "./voice/facts";
import { PatternGroups, RulesBlock, TermGroup } from "./voice/rules";
import { ResolutionHeader, shortDate } from "./voice/resolution-header";

export interface VoicePageProps {
  tabID: string;
  /** Injected in tests and stories; production reads the Wails backend. */
  result?: ProjectVoiceResult;
  /** False renders the profile without the edit affordance. */
  editable?: boolean;
  /** Injected in tests and stories; production writes through the backend. */
  save?: (profile: VoiceProfile) => Promise<VoiceSaveResult | null>;
  valueSets?: Record<string, FieldValueSet>;
}

/** A section of the profile document. */
function Section({
  title,
  icon,
  count,
  children,
}: {
  title: string;
  icon?: React.ReactNode;
  count?: number;
  children: React.ReactNode;
}) {
  return (
    <section className="space-y-2">
      <SectionHeading icon={icon} count={count}>
        {title}
      </SectionHeading>
      {children}
    </section>
  );
}

function ToneBlock({ tone }: { tone?: ToneProfile }) {
  if (!tone) return null;
  const empty =
    !tone.personality?.length &&
    !tone.formality &&
    !tone.emotion &&
    !tone.humor &&
    !tone.guidelines;
  if (empty) return null;
  return (
    <div className="space-y-2">
      {!!tone.personality?.length && (
        <ul className="flex flex-wrap gap-1.5" data-testid="voice-personality">
          {tone.personality.map((p) => (
            <li key={p}>
              <Badge variant="secondary" className="font-normal">
                {p}
              </Badge>
            </li>
          ))}
        </ul>
      )}
      <FactGrid
        facts={[
          { label: t("Formality"), value: tone.formality },
          { label: t("Emotion"), value: tone.emotion },
          { label: t("Humor"), value: tone.humor },
        ]}
      />
      {tone.guidelines && <p className="text-sm text-foreground">{tone.guidelines}</p>}
    </div>
  );
}

/** The style facts, without the patterns, which live in the Rules section. */
function StyleFacts({ style }: { style?: StyleRules }) {
  if (!style) return null;
  return (
    <FactGrid
      columns={4}
      facts={[
        {
          label: t("Voice"),
          value:
            style.active_voice === undefined ? undefined : style.active_voice ? "active" : "any",
        },
        { label: t("Sentences"), value: style.sentence_length },
        { label: t("Point of view"), value: style.person_pov },
        { label: t("Contractions"), value: style.contractions },
      ]}
    />
  );
}

function ExampleRow({ example }: { example: VoiceExample }) {
  return (
    <li className="space-y-1 py-2" data-testid="voice-example">
      <div className="flex flex-wrap items-baseline gap-2 text-sm">
        <span className="text-muted-foreground line-through">{example.before}</span>
        <ArrowRight className="size-3 text-muted-foreground" />
        <span className="font-medium">{example.after}</span>
        {example.category && (
          <Badge variant="outline" className="font-normal text-muted-foreground">
            {example.category}
          </Badge>
        )}
      </div>
      {example.explanation && (
        <p className="text-[11px] text-muted-foreground">{example.explanation}</p>
      )}
    </li>
  );
}

/** One row in the point rail. */
function PointRow({
  point,
  selected,
  onSelect,
}: {
  point: VoicePoint;
  selected: boolean;
  onSelect: () => void;
}) {
  const coordinates = Object.entries(point.coordinates ?? {});
  return (
    <li>
      <button
        type="button"
        onClick={onSelect}
        aria-current={selected ? "true" : undefined}
        className={cn(
          "w-full rounded-lg border px-3 py-2 text-left transition-colors",
          selected ? "border-primary bg-accent" : "border-transparent hover:bg-accent/60",
        )}
        data-testid="voice-point-row"
      >
        <div className="flex items-baseline gap-2">
          <span className="truncate text-sm font-medium">{point.label}</span>
          {point.fallback && (
            <Badge variant="outline" className="border-destructive/40 font-normal text-destructive">
              {t("fell through")}
            </Badge>
          )}
        </div>
        {coordinates.length > 0 && (
          <span className="mt-1 flex flex-wrap items-center gap-1">
            {coordinates.map(([axis, value]) => (
              <CoordinateChip key={axis} axis={axis} value={value} />
            ))}
          </span>
        )}
        <p className="truncate text-[11px] text-muted-foreground">
          {point.profile ? point.profile.name : t("no voice profile binds here")}
        </p>
        {point.collections.length > 0 && (
          <p className="truncate text-[11px] text-muted-foreground">
            {point.collections.join(", ")}
          </p>
        )}
      </button>
    </li>
  );
}

/** The selected point's profile, read as a document. */
function PointDetail({ point, onEdit }: { point: VoicePoint; onEdit?: () => void }) {
  const profile = point.profile;
  const edit = point.edit;
  return (
    <div className="space-y-5" data-testid="voice-detail">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <ResolutionHeader point={point} />
        </div>
        {onEdit &&
          (edit?.writable ? (
            <Button variant="outline" size="sm" onClick={onEdit} data-testid="voice-edit">
              <Pencil className="mr-1 size-3.5" />
              {edit.exists && !edit.inherited
                ? t("Edit")
                : edit.inherited
                  ? t("Give this point its own voice")
                  : t("Create a voice")}
            </Button>
          ) : (
            <SimpleTooltip content={edit?.reason ?? ""}>
              <span className="inline-flex">
                <Button variant="outline" size="sm" disabled data-testid="voice-edit">
                  <Pencil className="mr-1 size-3.5" />
                  {t("Edit")}
                </Button>
              </span>
            </SimpleTooltip>
          ))}
      </div>
      {onEdit && edit && !edit.writable && edit.reason && (
        <p className="text-xs text-muted-foreground" data-testid="voice-edit-reason">
          {edit.reason}
        </p>
      )}

      {!profile ? (
        <EmptyHint
          title={t("No voice profile binds at this point")}
          description={
            edit?.writable && edit.target
              ? t("Create one at {target}, and checks here will apply it.", {
                  target: edit.target,
                })
              : t("Bind one with defaults.voice, or put a profile at .kapi/voice.yaml.")
          }
        />
      ) : (
        <div className="space-y-5">
          {(profile.description || (profile.min_score !== undefined && profile.min_score > 0)) && (
            <div className="space-y-1">
              {profile.description && (
                <p className="text-sm text-muted-foreground">{profile.description}</p>
              )}
              {profile.min_score !== undefined && profile.min_score > 0 && (
                <p className="text-[11px] text-muted-foreground">
                  {t("Compliance bar {score}", { score: profile.min_score })}
                </p>
              )}
            </div>
          )}

          <Separator />
          <Section title={t("Rules")} icon={<ListChecks className="size-3.5" />}>
            <RulesBlock profile={profile} />
          </Section>

          <Separator />
          <Section title={t("Tone")} icon={<MessageSquareQuote className="size-3.5" />}>
            <ToneBlock tone={profile.tone} />
          </Section>

          <Separator />
          <Section title={t("Style")} icon={<FileText className="size-3.5" />}>
            <StyleFacts style={profile.style} />
          </Section>

          {!!profile.examples?.length && (
            <>
              <Separator />
              <Section
                title={t("Examples")}
                icon={<Sparkles className="size-3.5" />}
                count={profile.examples.length}
              >
                <ul className="divide-y text-sm">
                  {profile.examples.map((e) => (
                    <ExampleRow key={`${e.before}:${e.after}`} example={e} />
                  ))}
                </ul>
              </Section>
            </>
          )}

          {!!profile.locales && Object.keys(profile.locales).length > 0 && (
            <>
              <Separator />
              <Section title={t("By language")} icon={<Languages className="size-3.5" />}>
                <div className="space-y-3">
                  {Object.entries(profile.locales).map(([locale, override]) => (
                    <div key={locale} className="rounded-lg border px-3 py-2">
                      <p className="text-sm font-medium">
                        <LocaleLabel locale={locale} />
                      </p>
                      <div className="mt-1">
                        <FactGrid
                          facts={[
                            { label: t("Formality"), value: override.formality },
                            { label: t("Humor"), value: override.humor },
                            { label: t("Point of view"), value: override.person_pov },
                          ]}
                        />
                      </div>
                      {override.cultural_notes && (
                        <p className="mt-1 text-sm text-muted-foreground">
                          {override.cultural_notes}
                        </p>
                      )}
                      <div className="mt-2 space-y-3">
                        <TermGroup
                          title={t("Wording here")}
                          rules={override.vocabulary_overrides}
                        />
                      </div>
                      {!!override.example_overrides?.length && (
                        <ul className="mt-2 divide-y text-sm">
                          {override.example_overrides.map((e) => (
                            <ExampleRow key={`${e.before}:${e.after}`} example={e} />
                          ))}
                        </ul>
                      )}
                    </div>
                  ))}
                </div>
              </Section>
            </>
          )}

          {!!profile.channels && Object.keys(profile.channels).length > 0 && (
            <>
              <Separator />
              <Section title={t("By channel")} icon={<Radio className="size-3.5" />}>
                <div className="space-y-3">
                  {Object.entries(profile.channels).map(([channel, override]) => (
                    <div key={channel} className="space-y-2 rounded-lg border px-3 py-2">
                      <p className="text-sm font-medium">{channel}</p>
                      <ToneBlock tone={override.tone} />
                      <StyleFacts style={override.style} />
                      <PatternGroups style={override.style} />
                    </div>
                  ))}
                </div>
              </Section>
            </>
          )}

          {!!profile.personas && Object.keys(profile.personas).length > 0 && (
            <>
              <Separator />
              <Section title={t("By persona")} icon={<UserRound className="size-3.5" />}>
                <div className="space-y-3">
                  {Object.entries(profile.personas).map(([persona, override]) => (
                    <div key={persona} className="space-y-2 rounded-lg border px-3 py-2">
                      <p className="text-sm font-medium">{persona}</p>
                      <ToneBlock tone={override.tone} />
                      <StyleFacts style={override.style} />
                      <PatternGroups style={override.style} />
                      <TermGroup title={t("Say this")} rules={override.preferred_terms} />
                      <TermGroup title={t("Never say")} rules={override.avoided_terms} />
                    </div>
                  ))}
                </div>
              </Section>
            </>
          )}

          {point.guide && (
            <>
              <Separator />
              <Section
                title={t("The guide a tool reads")}
                icon={<ShieldCheck className="size-3.5" />}
              >
                <pre className="max-h-80 overflow-auto rounded-lg border bg-muted/40 p-3 font-mono text-[11px] whitespace-pre-wrap text-muted-foreground">
                  {point.guide}
                </pre>
              </Section>
            </>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * The Voice page for the open project.
 *
 * Every point the recipe declares is listed, the project's own default first,
 * so a single-point project reads as one row rather than as a surface with
 * something missing.
 */
export function VoicePage({ tabID, result, editable = true, save, valueSets }: VoicePageProps) {
  const queries = useQueryClient();
  const query = useQuery({
    queryKey: qk.projectVoice(tabID),
    queryFn: () => call<ProjectVoiceResult>("ProjectVoice", tabID),
    enabled: !result && !!tabID,
  });
  const data = result ?? query.data ?? undefined;

  const [selected, setSelected] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const active = useMemo(() => {
    if (!data?.points.length) return undefined;
    return data.points.find((p) => p.label === selected) ?? data.points[0];
  }, [data, selected]);

  // A profile the point inherits is a starting point, not the thing being
  // edited: saving writes this point's own file. A point with nothing bound
  // starts from its name.
  const draft: VoiceProfile = useMemo(() => {
    if (!active) return { name: "" };
    if (active.profile && !active.edit?.inherited) return active.profile;
    return active.profile ?? { name: active.label === "project default" ? "" : active.label };
  }, [active]);

  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto p-6">
      <PageHeader
        title={t("Voice")}
        subtitle={t("The profile that governs each point, and why that one.")}
      />

      {!result && query.isError && (
        <ErrorHint
          title={t("The voice profiles did not load")}
          description={(query.error as Error).message}
        />
      )}

      {!result && query.isLoading && (
        <div className="mt-4 space-y-2">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      )}

      {data && (
        <>
          {!!data.notes?.length && (
            <ul className="mt-4 space-y-1 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs">
              {data.notes.map((note) => (
                <li key={note}>{note}</li>
              ))}
            </ul>
          )}

          {data.points.length === 0 ? (
            <EmptyHint
              title={t("This project declares no points")}
              description={t("Open the recipe to bind a voice profile.")}
            />
          ) : (
            <div className="mt-4 grid min-h-0 flex-1 gap-6 lg:grid-cols-[18rem_minmax(0,1fr)]">
              <nav aria-label={t("Points")}>
                <ul className="space-y-1">
                  {data.points.map((p) => (
                    <PointRow
                      key={p.label}
                      point={p}
                      selected={active?.label === p.label}
                      onSelect={() => setSelected(p.label)}
                    />
                  ))}
                </ul>
                <p className="mt-3 px-3 text-[11px] text-muted-foreground">
                  {t("Resolved {at}", { at: shortDate(data.at) })}
                </p>
              </nav>
              {active &&
                (editing ? (
                  <VoiceProfileEditor
                    tabID={tabID}
                    profileName={active.point.profile ?? ""}
                    target={active.edit}
                    profile={draft}
                    valueSets={valueSets}
                    save={save}
                    onCancel={() => setEditing(false)}
                    onSaved={() => {
                      setEditing(false);
                      void queries.invalidateQueries({ queryKey: qk.projectVoice(tabID) });
                    }}
                  />
                ) : (
                  <PointDetail
                    point={active}
                    onEdit={editable ? () => setEditing(true) : undefined}
                  />
                ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}
