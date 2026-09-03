// Context › Voice — the profile that governs each point, read whole.
//
// The voice profile decides what `kapi check` fails on, what a review finding
// says, and how an AI proposal is worded. The page answers three questions in
// the order a reader asks them: which point am I standing at, which profile
// governs there and why that one, and what does that profile actually say.
//
// The resolution chain is the reason the page exists. A binding whose validity
// window excluded the run instant is drawn as a skipped rung with its boundary
// date, followed by what governs in its place. It is the one place in the app
// where governance changing on a date is visible, so it stays prominent while
// the recipe plumbing that selected the profile collapses to one quiet line.
//
// The profile then reads as a document, wording rules first: the say-this and
// never-say constraints are what a writer reaches for, so they lead, before
// tone, style, examples and the per-language, per-channel and per-persona
// overrides.

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  BookOpen,
  Braces,
  FileText,
  Languages,
  Link2,
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
import { severityFails } from "../types/voice";
import { VoiceProfileEditor } from "./voice/VoiceProfileEditor";
import type {
  FieldValueSet,
  Pattern,
  ProjectVoiceResult,
  StyleRules,
  TermRule,
  ToneProfile,
  VoiceExample,
  VoicePoint,
  VoiceProfile,
  VoiceSaveResult,
  VoiceValidity,
} from "../types/voice";

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

/**
 * A severity as a neutral chip.
 *
 * Colour is reserved for a judgement a person makes or a finding a check
 * raised (see `packages/ui/docs/judgement-colours.md`); a rule stating what a
 * violation would cost is neither, so the chip stays neutral and the tooltip
 * carries whether it fails or only reports. A rule resolved from a terms store
 * carries no severity, and then no chip is drawn.
 */
function SeverityChip({ severity }: { severity?: string }) {
  if (!severity) return null;
  const fails = severityFails(severity);
  return (
    <SimpleTooltip
      content={fails ? t("A violation fails the check") : t("A violation is reported only")}
    >
      <Badge
        variant="outline"
        className="font-normal text-muted-foreground"
        data-testid="voice-severity"
      >
        {severity}
      </Badge>
    </SimpleTooltip>
  );
}

/** A labelled value in a compact definition grid. */
function Fact({ label, value }: { label: string; value?: string }) {
  if (!value) return null;
  return (
    <div className="min-w-0">
      <dt className="text-[11px] tracking-wide text-muted-foreground uppercase">{label}</dt>
      <dd className="truncate text-sm text-foreground">{value}</dd>
    </div>
  );
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

/** A titled group of rule rows within the Rules section. */
function RuleGroup({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <p className="mb-1 text-xs font-medium text-foreground">{title}</p>
      <ul className="divide-y text-sm">{children}</ul>
    </div>
  );
}

/**
 * One pattern rule.
 *
 * The human description is the label; the regular expression it matches on is
 * tucked behind the `{}` affordance, so the row reads as a rule rather than as
 * code.
 */
function PatternRow({ pattern }: { pattern: Pattern }) {
  const allowance = pattern.rate
    ? pattern.rate.per_words
      ? t("up to {max} per {words} words", {
          max: pattern.rate.max,
          words: pattern.rate.per_words,
        })
      : t("up to {max}", { max: pattern.rate.max })
    : undefined;
  return (
    <li className="flex flex-wrap items-baseline gap-2 py-1.5" data-testid="voice-pattern">
      <span className="min-w-0 flex-1 text-sm text-foreground">
        {pattern.description || pattern.regex}
      </span>
      {pattern.description && (
        <SimpleTooltip content={pattern.regex}>
          <span
            className="cursor-help text-muted-foreground/70"
            data-testid="voice-pattern-regex"
            aria-label={t("Matching pattern")}
          >
            <Braces className="size-3.5" />
          </span>
        </SimpleTooltip>
      )}
      {pattern.scope && (
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {pattern.scope}
        </Badge>
      )}
      {allowance && <span className="text-[11px] text-muted-foreground">{allowance}</span>}
      <SeverityChip severity={pattern.severity} />
    </li>
  );
}

/**
 * One term rule, drawn to the same shape as a pattern.
 *
 * The wording change is the label; how the rule matches (its other forms, case
 * sensitivity) is tucked behind the `{}` affordance, and a link marks a rule
 * backed by a concept in the terms store.
 */
function TermRuleRow({ rule }: { rule: TermRule }) {
  const matching = [
    rule.forms?.length ? t("also {forms}", { forms: rule.forms.join(", ") }) : undefined,
    rule.case_sensitive ? t("case-sensitive") : undefined,
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <li className="flex flex-wrap items-baseline gap-2 py-1.5" data-testid="voice-term-rule">
      <span className="min-w-0 flex-1 text-sm">
        <span className="font-medium text-foreground">{rule.term}</span>
        {rule.replacement ? (
          <span className="text-muted-foreground">
            {" "}
            <ArrowRight className="inline size-3 align-[-1px]" /> {rule.replacement}
          </span>
        ) : (
          <span className="ml-1 text-[11px] text-muted-foreground">
            {t("no replacement, so tools skip it")}
          </span>
        )}
        {rule.note && <span className="ml-2 text-[11px] text-muted-foreground">{rule.note}</span>}
      </span>
      {matching && (
        <SimpleTooltip content={matching}>
          <span
            className="cursor-help text-muted-foreground/70"
            data-testid="voice-term-detail"
            aria-label={t("Matching detail")}
          >
            <Braces className="size-3.5" />
          </span>
        </SimpleTooltip>
      )}
      {rule.do_not_translate && (
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {t("keep as written")}
        </Badge>
      )}
      {rule.scope && (
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {rule.scope}
        </Badge>
      )}
      {rule.concept_id && (
        <SimpleTooltip content={t("Linked to a concept in your terms")}>
          <span
            className="text-muted-foreground/70"
            data-testid="voice-concept-link"
            aria-label={t("Linked concept")}
          >
            <Link2 className="size-3" />
          </span>
        </SimpleTooltip>
      )}
      <SeverityChip severity={rule.severity} />
    </li>
  );
}

/** A term-rule group, or nothing when the group is empty. */
function TermGroup({ title, rules }: { title: string; rules?: TermRule[] }) {
  if (!rules?.length) return null;
  return (
    <RuleGroup title={title}>
      {rules.map((r) => (
        <TermRuleRow key={`${r.term}:${r.replacement ?? ""}`} rule={r} />
      ))}
    </RuleGroup>
  );
}

/** The never-write and always-write pattern groups of a style, or nothing. */
function PatternGroups({ style }: { style?: StyleRules }) {
  if (!style?.prohibited_patterns?.length && !style?.required_patterns?.length) return null;
  return (
    <>
      {!!style.prohibited_patterns?.length && (
        <RuleGroup title={t("Never write")}>
          {style.prohibited_patterns.map((p) => (
            <PatternRow key={p.regex} pattern={p} />
          ))}
        </RuleGroup>
      )}
      {!!style.required_patterns?.length && (
        <RuleGroup title={t("Always write")}>
          {style.required_patterns.map((p) => (
            <PatternRow key={p.regex} pattern={p} />
          ))}
        </RuleGroup>
      )}
    </>
  );
}

/**
 * Every wording rule in one place: the say-this and never-say term rules and
 * the always-write and never-write patterns, each drawn to the same row shape,
 * with abbreviations beneath them.
 */
function RulesBlock({ profile }: { profile: VoiceProfile }) {
  const vocab = profile.vocabulary;
  const style = profile.style;
  const abbreviations = vocab?.abbreviations ?? {};
  const hasAbbreviations = Object.keys(abbreviations).length > 0;
  const hasRules =
    !!vocab?.preferred_terms?.length ||
    !!vocab?.forbidden_terms?.length ||
    !!vocab?.competitor_terms?.length ||
    !!style?.prohibited_patterns?.length ||
    !!style?.required_patterns?.length;
  return (
    <div className="space-y-3">
      <TermGroup title={t("Say this")} rules={vocab?.preferred_terms} />
      <TermGroup title={t("Never say")} rules={vocab?.forbidden_terms} />
      <TermGroup title={t("Competitor names")} rules={vocab?.competitor_terms} />
      <PatternGroups style={style} />
      {hasAbbreviations && (
        <div>
          <p className="mb-1 text-xs font-medium text-foreground">{t("Abbreviations")}</p>
          <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm sm:grid-cols-3">
            {Object.entries(abbreviations).map(([short, long]) => (
              <div key={short} className="flex items-baseline gap-2">
                <dt className="font-medium">{short}</dt>
                <dd className="truncate text-muted-foreground">{long}</dd>
              </div>
            ))}
          </dl>
        </div>
      )}
      {!hasRules && !hasAbbreviations && (
        <p className="text-sm text-muted-foreground">{t("This profile constrains no wording.")}</p>
      )}
    </div>
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
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3">
        <Fact label={t("Formality")} value={tone.formality} />
        <Fact label={t("Emotion")} value={tone.emotion} />
        <Fact label={t("Humor")} value={tone.humor} />
      </dl>
      {tone.guidelines && <p className="text-sm text-foreground">{tone.guidelines}</p>}
    </div>
  );
}

/** The style facts, without the patterns, which live in the Rules section. */
function StyleFacts({ style }: { style?: StyleRules }) {
  if (!style) return null;
  const empty =
    style.active_voice === undefined &&
    !style.sentence_length &&
    !style.person_pov &&
    !style.contractions;
  if (empty) return null;
  return (
    <dl className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4">
      <Fact
        label={t("Voice")}
        value={style.active_voice === undefined ? undefined : style.active_voice ? "active" : "any"}
      />
      <Fact label={t("Sentences")} value={style.sentence_length} />
      <Fact label={t("Point of view")} value={style.person_pov} />
      <Fact label={t("Contractions")} value={style.contractions} />
    </dl>
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

/** An RFC3339 instant as the date a reader needs. */
function shortDate(value: string): string {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toISOString().slice(0, 10);
}

/**
 * The chain the resolver walked to arrive at this voice, kept prominent.
 *
 * A skipped rung is drawn, struck through, with the boundary that excluded it,
 * so a reader can see that governance moved on a date rather than that a
 * profile was never bound. The voice that governs instead is emphasized.
 */
function ResolutionChain({ point }: { point: VoicePoint }) {
  const winner = point.fallback
    ? point.fallback.governing || t("project default")
    : (point.profile?.name ?? point.label);
  return (
    <div
      className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2"
      data-testid="voice-chain"
    >
      {point.fallback && (
        <>
          <span className="flex items-center gap-1.5 text-sm">
            <span className="text-muted-foreground line-through">{point.fallback.profile}</span>
            <Badge variant="outline" className="border-destructive/40 font-normal text-destructive">
              {point.fallback.expired ? t("window closed") : t("not yet in force")}
              {point.fallback.boundary && (
                <span className="ml-1 font-mono text-[11px] opacity-70">
                  {shortDate(point.fallback.boundary)}
                </span>
              )}
            </Badge>
          </span>
          <ArrowRight className="size-4 text-muted-foreground" />
        </>
      )}
      <span className="text-sm font-semibold text-foreground">{winner}</span>
    </div>
  );
}

/** A validity window, coloured because a lapsed one is an alert, not a state. */
function ValidityChip({ validity }: { validity: VoiceValidity }) {
  const range = [validity.from, validity.to]
    .filter(Boolean)
    .map((v) => shortDate(v as string))
    .join(" → ");
  return (
    <Badge
      variant="outline"
      className={cn(
        "font-normal",
        validity.state === "expired"
          ? "border-destructive/40 text-destructive"
          : validity.state === "upcoming"
            ? "border-amber-500/40 text-amber-700 dark:text-amber-500"
            : "border-emerald-500/40 text-emerald-700 dark:text-emerald-500",
      )}
      data-testid="voice-validity"
    >
      {validity.state}
      {range && <span className="ml-1 font-mono text-[11px] opacity-70">{range}</span>}
    </Badge>
  );
}

/**
 * The recipe plumbing that selected this voice, collapsed to one quiet line:
 * the key it was bound on, how it was selected, where it lives, the terms it
 * reads, the channels it covers and its validity window.
 */
function PlumbingLine({ point }: { point: VoicePoint }) {
  const path = point.source || point.binding?.value;
  const hasAny =
    !!point.field ||
    !!point.binding ||
    !!path ||
    !!point.termstore ||
    !!point.validity ||
    !!point.channels?.length;
  if (!hasAny) return null;
  return (
    <div
      className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground"
      data-testid="voice-plumbing"
    >
      {point.field && (
        <SimpleTooltip content={t("The recipe key this voice is bound on")}>
          <span className="inline-flex items-baseline gap-1">
            {t("bound on")} <code className="font-mono">{point.field}</code>
          </span>
        </SimpleTooltip>
      )}
      {point.binding && (
        <SimpleTooltip content={t("How the profile was selected")}>
          <code className="font-mono">{point.binding.kind}</code>
        </SimpleTooltip>
      )}
      {path && (
        <SimpleTooltip content={path}>
          <code className="inline-block max-w-[18rem] truncate align-bottom font-mono">{path}</code>
        </SimpleTooltip>
      )}
      {point.termstore && (
        <span className="inline-flex items-center gap-1">
          <BookOpen className="size-3" />
          {point.termstore}
        </span>
      )}
      {!!point.channels?.length && (
        <span className="inline-flex items-center gap-1">
          <Radio className="size-3" />
          {point.channels.join(", ")}
        </span>
      )}
      {point.validity && <ValidityChip validity={point.validity} />}
    </div>
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
        <div className="min-w-0 flex-1 space-y-2">
          <ResolutionChain point={point} />
          <PlumbingLine point={point} />
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
                      <dl className="mt-1 grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3">
                        <Fact label={t("Formality")} value={override.formality} />
                        <Fact label={t("Humor")} value={override.humor} />
                        <Fact label={t("Point of view")} value={override.person_pov} />
                      </dl>
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
