// Editing the voice profile a point resolves to.
//
// The destination is the committed file in the working tree, the one a reviewer
// reads in the diff, written through the comment-preserving writer so an
// author's reasoning and key order survive an edit made here. Validation is the
// same three stages `kapi voice validate` runs, so a profile this surface
// accepts is one the loop accepts.

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Info, Loader2, Save, X } from "lucide-react";
import { Badge, Button, Input, Separator, Textarea, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { call } from "../../hooks/useApi";
import { qk } from "../../lib/queryKeys";
import { Field, NumberField } from "./fields";
import { AbbreviationsEditor, ExampleListEditor, TermRuleListEditor } from "./lists";
import {
  ChannelOverridesEditor,
  LocaleOverridesEditor,
  PersonaOverridesEditor,
  StyleEditor,
  ToneEditor,
} from "./blocks";
import type { ValueSets } from "./lists";
import type {
  FieldValueSet,
  ProfileProblem,
  TermRule,
  VoiceEditTarget,
  VoiceExample,
  VoiceProfile,
  VoiceSaveResult,
} from "../../types/voice";

export interface VoiceProfileEditorProps {
  tabID: string;
  /** The point being edited: a profile name, or "" for the project's own. */
  profileName: string;
  /** Where a save lands. */
  target: VoiceEditTarget;
  /** The profile to open on. A new profile starts from a name alone. */
  profile: VoiceProfile;
  onSaved: (result: VoiceSaveResult) => void;
  onCancel: () => void;
  /** Injected in tests and stories; production reads the Wails backend. */
  valueSets?: Record<string, FieldValueSet>;
  save?: (profile: VoiceProfile) => Promise<VoiceSaveResult | null>;
}

// Stable empty slices, so an absent section does not hand a memoized editor a
// new object on every render.
const EMPTY_TONE = {} as const;
const EMPTY_STYLE = {} as const;
const EMPTY_RULES: TermRule[] = [];
const EMPTY_EXAMPLES: VoiceExample[] = [];
const EMPTY_MAP = {} as const;

/** A validation problem, and whether it refuses the save or only notes it. */
function ProblemRow({ problem }: { problem: ProfileProblem }) {
  return (
    <li className="flex items-start gap-2 text-xs" data-testid="voice-problem">
      {problem.warning ? (
        <Info className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
      ) : (
        <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-destructive" />
      )}
      <span>
        {problem.field && (
          <code className="mr-1.5 font-mono text-[11px] text-muted-foreground">
            {problem.field}
          </code>
        )}
        {problem.message}
      </span>
    </li>
  );
}

export function VoiceProfileEditor({
  tabID,
  profileName,
  target,
  profile,
  onSaved,
  onCancel,
  valueSets,
  save,
}: VoiceProfileEditorProps) {
  const [draft, setDraft] = useState<VoiceProfile>(profile);
  const [problems, setProblems] = useState<ProfileProblem[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => setDraft(profile), [profile]);

  const sets = useQuery({
    queryKey: qk.voiceFieldValues(),
    queryFn: () => call<Record<string, FieldValueSet>>("VoiceFieldValues"),
    enabled: !valueSets,
    staleTime: Infinity,
  });
  const values: ValueSets = useMemo(() => valueSets ?? sets.data ?? {}, [valueSets, sets.data]);

  const blocking = problems.filter((p) => !p.warning);

  const doSave = useCallback(async () => {
    setSaving(true);
    setError(null);
    try {
      const result = save
        ? await save(draft)
        : await call<VoiceSaveResult>("SaveVoiceProfile", tabID, profileName, draft);
      if (!result) {
        setError(t("The desktop backend is not available."));
        return;
      }
      setProblems(result.problems ?? []);
      if (result.saved) onSaved(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }, [draft, profileName, save, tabID, onSaved]);

  // A profile has a lot of parts, and a keystroke in one must not re-render the
  // rest: the patch is a functional update and every section handler is stable,
  // so a memoized section re-renders only when its own slice moved.
  const update = useCallback(
    (patch: Partial<VoiceProfile>) => setDraft((d) => ({ ...d, ...patch })),
    [],
  );
  const patchVocabulary = useCallback(
    (patch: Partial<NonNullable<VoiceProfile["vocabulary"]>>) =>
      setDraft((d) => ({ ...d, vocabulary: { ...d.vocabulary, ...patch } })),
    [],
  );
  const setTone = useCallback(
    (tone: NonNullable<VoiceProfile["tone"]>) => setDraft((d) => ({ ...d, tone })),
    [],
  );
  const setStyle = useCallback(
    (style: NonNullable<VoiceProfile["style"]>) => setDraft((d) => ({ ...d, style })),
    [],
  );
  const setExamples = useCallback(
    (examples: NonNullable<VoiceProfile["examples"]>) => setDraft((d) => ({ ...d, examples })),
    [],
  );
  const setLocales = useCallback(
    (locales: NonNullable<VoiceProfile["locales"]>) => setDraft((d) => ({ ...d, locales })),
    [],
  );
  const setChannels = useCallback(
    (channels: NonNullable<VoiceProfile["channels"]>) => setDraft((d) => ({ ...d, channels })),
    [],
  );
  const setPersonas = useCallback(
    (personas: NonNullable<VoiceProfile["personas"]>) => setDraft((d) => ({ ...d, personas })),
    [],
  );
  const setPreferred = useCallback(
    (preferred_terms: TermRule[]) => patchVocabulary({ preferred_terms }),
    [patchVocabulary],
  );
  const setForbidden = useCallback(
    (forbidden_terms: TermRule[]) => patchVocabulary({ forbidden_terms }),
    [patchVocabulary],
  );
  const setCompetitor = useCallback(
    (competitor_terms: TermRule[]) => patchVocabulary({ competitor_terms }),
    [patchVocabulary],
  );
  const setAbbreviations = useCallback(
    (abbreviations: Record<string, string>) => patchVocabulary({ abbreviations }),
    [patchVocabulary],
  );

  return (
    <div className="space-y-5" data-testid="voice-editor">
      <div className="sticky top-0 z-10 -mx-1 flex flex-wrap items-center gap-2 border-b bg-background px-1 py-2">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">
            {target.exists ? t("Editing") : t("Creating")}{" "}
            <code className="font-mono text-[11px] text-muted-foreground">{target.target}</code>
          </p>
          {target.inherited && (
            <p className="text-xs text-muted-foreground">
              {t("This point reads a voice bound coarser. Saving gives it one of its own.")}
            </p>
          )}
        </div>
        <Button variant="ghost" size="sm" onClick={onCancel} disabled={saving}>
          <X className="mr-1 size-3.5" />
          {t("Cancel")}
        </Button>
        <Button size="sm" onClick={doSave} disabled={saving}>
          {saving ? (
            <Loader2 className="mr-1 size-3.5 animate-spin" />
          ) : (
            <Save className="mr-1 size-3.5" />
          )}
          {t("Save")}
        </Button>
      </div>

      {error && (
        <p className="rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {error}
        </p>
      )}

      {problems.length > 0 && (
        <div
          className={cn(
            "rounded-lg border px-3 py-2",
            blocking.length > 0
              ? "border-destructive/40 bg-destructive/10"
              : "border-amber-500/40 bg-amber-500/10",
          )}
          data-testid="voice-problems"
        >
          <p className="mb-1 text-xs font-medium">
            {blocking.length > 0 ? t("Fix these before saving") : t("Saved, with notes")}
          </p>
          <ul className="space-y-1">
            {problems.map((p, i) => (
              <ProblemRow key={`${p.field ?? ""}:${i}`} problem={p} />
            ))}
          </ul>
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("Name")}>
          <Input
            value={draft.name ?? ""}
            onChange={(e) => update({ name: e.target.value })}
            aria-label={t("Name")}
          />
        </Field>
        <NumberField
          label={t("Compliance bar")}
          value={draft.min_score}
          min={0}
          max={100}
          hint={t("The score a target must reach. Empty uses the default.")}
          onChange={(min_score) => update({ min_score })}
        />
      </div>
      <Field label={t("Description")}>
        <Textarea
          rows={2}
          value={draft.description ?? ""}
          onChange={(e) => update({ description: e.target.value })}
          aria-label={t("Description")}
        />
      </Field>

      <Separator />
      <section className="space-y-2">
        <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          {t("Tone")}
        </h3>
        <ToneEditor
          tone={draft.tone ?? EMPTY_TONE}
          onChange={setTone}
          sets={values}
          idPrefix="profile"
        />
      </section>

      <Separator />
      <section className="space-y-2">
        <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          {t("Style")}
        </h3>
        <StyleEditor
          style={draft.style ?? EMPTY_STYLE}
          onChange={setStyle}
          sets={values}
          idPrefix="profile"
        />
      </section>

      <Separator />
      <section className="space-y-3">
        <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          {t("Vocabulary")}
        </h3>
        <TermRuleListEditor
          label={t("Say this")}
          rules={draft.vocabulary?.preferred_terms ?? EMPTY_RULES}
          onChange={setPreferred}
          sets={values}
          emptyHint={t("No wording is preferred over another.")}
          testid="preferred-terms"
        />
        <TermRuleListEditor
          label={t("Never say")}
          rules={draft.vocabulary?.forbidden_terms ?? EMPTY_RULES}
          onChange={setForbidden}
          sets={values}
          emptyHint={t("No wording is forbidden.")}
          testid="forbidden-terms"
        />
        <TermRuleListEditor
          label={t("Competitor names")}
          rules={draft.vocabulary?.competitor_terms ?? EMPTY_RULES}
          onChange={setCompetitor}
          sets={values}
          emptyHint={t("No competitor is named.")}
          testid="competitor-terms"
        />
        <AbbreviationsEditor
          abbreviations={draft.vocabulary?.abbreviations ?? EMPTY_MAP}
          onChange={setAbbreviations}
        />
      </section>

      <Separator />
      <section className="space-y-2">
        <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          {t("Examples")}
        </h3>
        <ExampleListEditor
          label={t("Rewrites")}
          examples={draft.examples ?? EMPTY_EXAMPLES}
          onChange={setExamples}
          sets={values}
          emptyHint={t("The voice is described but never shown.")}
          testid="profile-examples"
        />
      </section>

      <Separator />
      <section className="space-y-3">
        <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          {t("Overrides")}
        </h3>
        <LocaleOverridesEditor
          locales={draft.locales ?? EMPTY_MAP}
          onChange={setLocales}
          sets={values}
        />
        <ChannelOverridesEditor
          channels={draft.channels ?? EMPTY_MAP}
          onChange={setChannels}
          sets={values}
        />
        <PersonaOverridesEditor
          personas={draft.personas ?? EMPTY_MAP}
          onChange={setPersonas}
          sets={values}
        />
      </section>

      <Separator />
      <section className="space-y-2">
        <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">
          {t("Suggested rules")}
        </h3>
        <NumberField
          label={t("Promote after this many corrections")}
          value={draft.autonomy?.auto_promote_at_count}
          min={0}
          hint={t("Empty keeps every promotion a decision someone makes.")}
          onChange={(auto_promote_at_count) => update({ autonomy: { auto_promote_at_count } })}
        />
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {t("Saved to {target}", { target: target.target ?? "" })}
        </Badge>
      </section>
    </div>
  );
}
