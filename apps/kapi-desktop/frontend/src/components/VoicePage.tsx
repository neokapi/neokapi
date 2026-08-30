// Context › Voice — the profile that governs each point, read whole.
//
// The voice profile decides what `kapi check` fails on, what a review finding
// says, and how an AI proposal is worded. The page answers three questions in
// the order a reader asks them: which point am I standing at, which profile
// governs there and why that one, and what does that profile actually say.
//
// The resolution chain is the reason the page exists. A binding whose validity
// window excluded the run instant is drawn as a skipped rung with its boundary
// date, followed by what governs in its place — the one place in the app where
// governance changing on a date is visible.

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowRight,
  BookOpen,
  Braces,
  FileText,
  Languages,
  MessageSquareQuote,
  Radio,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import { Badge, PageHeader, Separator, Skeleton, SimpleTooltip, cn } from "@neokapi/ui-primitives";
import { EmptyHint, ErrorHint } from "@neokapi/concept-ui";
import { t } from "@neokapi/i18n-react/runtime";
import { call } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { severityFails } from "../types/voice";
import type {
  Pattern,
  ProjectVoiceResult,
  TermRule,
  ToneProfile,
  StyleRules,
  VoiceExample,
  VoicePoint,
} from "../types/voice";

export interface VoicePageProps {
  tabID: string;
  /** Injected in tests and stories; production reads the Wails backend. */
  result?: ProjectVoiceResult;
}

/** A severity, and whether it fails a check or only reports. */
function SeverityPill({ severity }: { severity?: string }) {
  const label = severity || t("unset");
  const fails = severityFails(severity);
  return (
    <SimpleTooltip
      content={fails ? t("A violation fails the check") : t("A violation is reported only")}
    >
      <Badge
        variant="outline"
        className={cn(
          "font-normal",
          fails
            ? "border-destructive/40 text-destructive"
            : "border-amber-500/40 text-amber-700 dark:text-amber-500",
        )}
      >
        {label}
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
      <h3 className="flex items-center gap-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
        {icon}
        {title}
        {count !== undefined && <span className="font-normal normal-case">{count}</span>}
      </h3>
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
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3">
        <Fact label={t("Formality")} value={tone.formality} />
        <Fact label={t("Emotion")} value={tone.emotion} />
        <Fact label={t("Humor")} value={tone.humor} />
      </dl>
      {tone.guidelines && <p className="text-sm text-foreground">{tone.guidelines}</p>}
    </div>
  );
}

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
      <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]">{pattern.regex}</code>
      <span className="min-w-0 flex-1 text-muted-foreground">{pattern.description}</span>
      {pattern.scope && (
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {pattern.scope}
        </Badge>
      )}
      {allowance && <span className="text-xs text-muted-foreground">{allowance}</span>}
      <SeverityPill severity={pattern.severity} />
    </li>
  );
}

function StyleBlock({ style }: { style?: StyleRules }) {
  if (!style) return null;
  return (
    <div className="space-y-3">
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4">
        <Fact
          label={t("Voice")}
          value={
            style.active_voice === undefined ? undefined : style.active_voice ? "active" : "any"
          }
        />
        <Fact label={t("Sentences")} value={style.sentence_length} />
        <Fact label={t("Point of view")} value={style.person_pov} />
        <Fact label={t("Contractions")} value={style.contractions} />
      </dl>
      {!!style.prohibited_patterns?.length && (
        <div>
          <p className="mb-1 text-xs font-medium text-foreground">{t("Never write")}</p>
          <ul className="divide-y text-sm">
            {style.prohibited_patterns.map((p) => (
              <PatternRow key={p.regex} pattern={p} />
            ))}
          </ul>
        </div>
      )}
      {!!style.required_patterns?.length && (
        <div>
          <p className="mb-1 text-xs font-medium text-foreground">{t("Always write")}</p>
          <ul className="divide-y text-sm">
            {style.required_patterns.map((p) => (
              <PatternRow key={p.regex} pattern={p} />
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function TermRuleRow({ rule }: { rule: TermRule }) {
  return (
    <li className="flex flex-wrap items-baseline gap-2 py-1.5" data-testid="voice-term-rule">
      <span className="font-medium">{rule.term}</span>
      {rule.replacement ? (
        <span className="flex items-center gap-1.5 text-muted-foreground">
          <ArrowRight className="size-3" />
          {rule.replacement}
        </span>
      ) : (
        <span className="text-xs text-muted-foreground">
          {t("no replacement, so tools skip it")}
        </span>
      )}
      {rule.do_not_translate && (
        <Badge variant="outline" className="font-normal">
          {t("keep as written")}
        </Badge>
      )}
      {rule.scope && (
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {rule.scope}
        </Badge>
      )}
      {!!rule.forms?.length && (
        <span className="text-xs text-muted-foreground">
          {t("also {forms}", { forms: rule.forms.join(", ") })}
        </span>
      )}
      {rule.concept_id && (
        <code className="font-mono text-[11px] text-muted-foreground">{rule.concept_id}</code>
      )}
      {rule.note && <span className="min-w-0 flex-1 text-muted-foreground">{rule.note}</span>}
      <SeverityPill severity={rule.severity} />
    </li>
  );
}

function TermRuleList({ title, rules }: { title: string; rules?: TermRule[] }) {
  if (!rules?.length) return null;
  return (
    <div>
      <p className="mb-1 text-xs font-medium text-foreground">{title}</p>
      <ul className="divide-y text-sm">
        {rules.map((r) => (
          <TermRuleRow key={`${r.term}:${r.replacement ?? ""}`} rule={r} />
        ))}
      </ul>
    </div>
  );
}

function ExampleRow({ example }: { example: VoiceExample }) {
  return (
    <li className="space-y-1 py-2" data-testid="voice-example">
      <div className="flex flex-wrap items-baseline gap-2">
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
        <p className="text-xs text-muted-foreground">{example.explanation}</p>
      )}
    </li>
  );
}

/**
 * The chain the resolver walked to arrive at this voice.
 *
 * A skipped rung is drawn, struck through, with the boundary that excluded it,
 * so a reader can see that governance moved on a date rather than that a
 * profile was never bound.
 */
function ResolutionChain({ point }: { point: VoicePoint }) {
  const governing = point.fallback ? point.fallback.governing || t("project default") : point.label;
  return (
    <div
      className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2 text-sm"
      data-testid="voice-chain"
    >
      {point.fallback && (
        <>
          <span className="flex items-center gap-1.5">
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
          <ArrowRight className="size-3.5 text-muted-foreground" />
        </>
      )}
      <span className="font-medium">{governing}</span>
      {point.field && (
        <code className="font-mono text-[11px] text-muted-foreground">{point.field}</code>
      )}
      {point.binding && (
        <Badge variant="secondary" className="font-normal">
          {point.binding.kind}
          <span className="ml-1 font-mono text-[11px]">{point.binding.value}</span>
        </Badge>
      )}
    </div>
  );
}

/** An RFC3339 instant as the date a reader needs. */
function shortDate(value: string): string {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toISOString().slice(0, 10);
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
  const address = point.coordinates
    ? Object.entries(point.coordinates)
        .map(([axis, value]) => `${axis}:${value}`)
        .join(" · ")
    : undefined;
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
          <span className="truncate font-medium">{point.label}</span>
          {point.fallback && (
            <Badge variant="outline" className="border-destructive/40 font-normal text-destructive">
              {t("fell through")}
            </Badge>
          )}
        </div>
        {address && (
          <p className="truncate font-mono text-[11px] text-muted-foreground">{address}</p>
        )}
        <p className="truncate text-xs text-muted-foreground">
          {point.profile ? point.profile.name : t("no voice profile binds here")}
        </p>
        {point.collections.length > 0 && (
          <p className="truncate text-xs text-muted-foreground">{point.collections.join(", ")}</p>
        )}
      </button>
    </li>
  );
}

/** The selected point's profile, read as a document. */
function PointDetail({ point }: { point: VoicePoint }) {
  const profile = point.profile;
  return (
    <div className="space-y-5" data-testid="voice-detail">
      <ResolutionChain point={point} />

      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        {point.source && (
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]">
            {point.source}
          </code>
        )}
        {point.termstore && (
          <Badge variant="outline" className="font-normal">
            <BookOpen className="mr-1 size-3" />
            {point.termstore}
          </Badge>
        )}
        {!!point.channels?.length && (
          <Badge variant="outline" className="font-normal">
            <Radio className="mr-1 size-3" />
            {point.channels.join(", ")}
          </Badge>
        )}
        {point.validity && (
          <Badge
            variant="outline"
            className={cn(
              "font-normal",
              point.validity.state === "expired"
                ? "border-destructive/40 text-destructive"
                : point.validity.state === "upcoming"
                  ? "border-amber-500/40 text-amber-700 dark:text-amber-500"
                  : "border-emerald-500/40 text-emerald-700 dark:text-emerald-500",
            )}
            data-testid="voice-validity"
          >
            {point.validity.state}
            <span className="ml-1 font-mono text-[11px] opacity-70">
              {[point.validity.from, point.validity.to]
                .filter(Boolean)
                .map((v) => shortDate(v as string))
                .join(" → ")}
            </span>
          </Badge>
        )}
      </div>

      {!profile ? (
        <EmptyHint
          title={t("No voice profile binds at this point")}
          description={t("Bind one with defaults.voice, or put a profile at .kapi/voice.yaml.")}
        />
      ) : (
        <div className="space-y-5">
          <div>
            <h2 className="text-base font-semibold">{profile.name}</h2>
            {profile.description && (
              <p className="text-sm text-muted-foreground">{profile.description}</p>
            )}
            {profile.min_score !== undefined && profile.min_score > 0 && (
              <p className="mt-1 text-xs text-muted-foreground">
                {t("Compliance bar {score}", { score: profile.min_score })}
              </p>
            )}
          </div>

          <Separator />
          <Section title={t("Tone")} icon={<MessageSquareQuote className="size-3.5" />}>
            <ToneBlock tone={profile.tone} />
          </Section>

          <Separator />
          <Section title={t("Style")} icon={<FileText className="size-3.5" />}>
            <StyleBlock style={profile.style} />
          </Section>

          <Separator />
          <Section title={t("Vocabulary")} icon={<BookOpen className="size-3.5" />}>
            <div className="space-y-3">
              <TermRuleList title={t("Say this")} rules={profile.vocabulary?.preferred_terms} />
              <TermRuleList title={t("Never say")} rules={profile.vocabulary?.forbidden_terms} />
              <TermRuleList
                title={t("Competitor names")}
                rules={profile.vocabulary?.competitor_terms}
              />
              {!!profile.vocabulary?.abbreviations &&
                Object.keys(profile.vocabulary.abbreviations).length > 0 && (
                  <div>
                    <p className="mb-1 text-xs font-medium text-foreground">{t("Abbreviations")}</p>
                    <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm sm:grid-cols-3">
                      {Object.entries(profile.vocabulary.abbreviations).map(([short, long]) => (
                        <div key={short} className="flex items-baseline gap-2">
                          <dt className="font-medium">{short}</dt>
                          <dd className="truncate text-muted-foreground">{long}</dd>
                        </div>
                      ))}
                    </dl>
                  </div>
                )}
              {!profile.vocabulary?.preferred_terms?.length &&
                !profile.vocabulary?.forbidden_terms?.length &&
                !profile.vocabulary?.competitor_terms?.length && (
                  <p className="text-sm text-muted-foreground">
                    {t("This profile constrains no wording.")}
                  </p>
                )}
            </div>
          </Section>

          {!!profile.examples?.length && (
            <>
              <Separator />
              <Section
                title={t("Examples")}
                icon={<Braces className="size-3.5" />}
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
                      <p className="text-sm font-medium">{locale}</p>
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
                      <TermRuleList
                        title={t("Wording here")}
                        rules={override.vocabulary_overrides}
                      />
                      {!!override.example_overrides?.length && (
                        <ul className="divide-y text-sm">
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
                    <div key={channel} className="rounded-lg border px-3 py-2">
                      <p className="text-sm font-medium">{channel}</p>
                      <ToneBlock tone={override.tone} />
                      <StyleBlock style={override.style} />
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
                    <div key={persona} className="rounded-lg border px-3 py-2">
                      <p className="text-sm font-medium">{persona}</p>
                      <ToneBlock tone={override.tone} />
                      <StyleBlock style={override.style} />
                      <TermRuleList title={t("Say this")} rules={override.preferred_terms} />
                      <TermRuleList title={t("Never say")} rules={override.avoided_terms} />
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
export function VoicePage({ tabID, result }: VoicePageProps) {
  const query = useQuery({
    queryKey: qk.projectVoice(tabID),
    queryFn: () => call<ProjectVoiceResult>("ProjectVoice", tabID),
    enabled: !result && !!tabID,
  });
  const data = result ?? query.data ?? undefined;

  const [selected, setSelected] = useState<string | null>(null);
  const active = useMemo(() => {
    if (!data?.points.length) return undefined;
    return data.points.find((p) => p.label === selected) ?? data.points[0];
  }, [data, selected]);

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
              {active && <PointDetail point={active} />}
            </div>
          )}
        </>
      )}
    </div>
  );
}
