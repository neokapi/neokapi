import { Badge, Button, Card, Checkbox, Input, Label, Switch } from "@neokapi/ui-primitives";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import type {
  VoiceProfile,
  ToneProfile,
  StyleRules,
  VocabularyRules,
  VoiceExample,
} from "../voice/types";
import type { ContextScanDraft, ContextScanFieldEvidence, ContextScanTerm } from "../types/api";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import { PersonalityTagPicker } from "../voice/PersonalityTagPicker";
import { ToneSpectrumSelector } from "../voice/ToneSpectrumSelector";
import { VocabularyEditor } from "../voice/VocabularyEditor";
import { ExamplesEditor } from "../voice/ExamplesEditor";
import { VoicePreview } from "../voice/VoicePreview";
import {
  formalitySpectrum,
  emotionSpectrum,
  humorSpectrum,
  sentenceLengthSpectrum,
  povSpectrum,
  contractionsSpectrum,
} from "../voice/data/tone-spectrums";
import { ContextScanAxes } from "./ContextScanAxes";
import { ContextScanLiveTester } from "./ContextScanLiveTester";
import { scanAxes, scanTerms, scanVoice } from "./artefacts";
import { TEST_IDS } from "../test-ids";
import { Loader2, Wand2 } from "../components/icons";

export interface ContextScanReviewProps {
  draft: ContextScanDraft;
  /** Called with the created profile after a successful approve. */
  onApproved: (profile: VoiceProfile) => void;
  /** Re-runs the scan with the same request. */
  onRegenerate?: () => void;
  /** Debounce for the live tester (ms). Tests lower it. */
  testerDebounceMs?: number;
}

/** Confidence badge + source attribution for one inferred field. */
function FieldEvidence({ evidence }: { evidence?: ContextScanFieldEvidence }) {
  if (!evidence) return null;
  const pct = Math.round(Math.max(0, Math.min(1, evidence.confidence)) * 100);
  const toneClass =
    pct >= 75
      ? "border-emerald-500/40 text-emerald-600 dark:text-emerald-400"
      : pct >= 45
        ? "border-amber-500/40 text-amber-600 dark:text-amber-400"
        : "border-border text-muted-foreground";
  return (
    <div className="flex items-center gap-2 min-w-0">
      <Badge
        variant="outline"
        data-testid={TEST_IDS.contextScan.fieldConfidence}
        className={`shrink-0 text-[11px] tabular-nums ${toneClass}`}
      >
        {pct}% confidence
      </Badge>
      {evidence.source && (
        <span className="text-xs text-muted-foreground truncate" title={evidence.source}>
          {evidence.source}
        </span>
      )}
    </div>
  );
}

/**
 * The review-first surface for a completed context scan (epic 016): every
 * inferred field renders with confidence and source attribution and stays
 * editable; candidate terms start selected and are individually
 * deselectable (opt-out — the "N of M selected" count keeps the selection
 * explicit, and approved candidates enter curation as "proposed", never as
 * governed terms); a live tester scores sample copy against the current
 * edits. Nothing persists until Approve.
 */

/**
 * Decides whether there is anything to review, before any hook runs.
 *
 * A scan that read a corpus and proposed no voice is a real outcome — an empty
 * or unreadable corpus, most often. Saying so beats an editor full of blank
 * fields that looks like a loaded draft, and deciding here rather than inside
 * the editor means the editor can take a voice as a fact instead of inventing
 * defaults for tone, formality and the rest to satisfy its state hooks.
 */
export function ContextScanReview(props: ContextScanReviewProps) {
  const voice = scanVoice(props.draft);
  if (!voice) {
    return (
      <div data-testid={TEST_IDS.contextScan.review} className="space-y-2">
        <h2 className="text-sm font-semibold">Nothing to review</h2>
        <p className="text-xs text-muted-foreground">
          The scan finished but proposed no voice. That usually means the sources held too little
          readable text. Add pasted text, pages, or files and scan again.
        </p>
      </div>
    );
  }
  return <ContextScanReviewEditor {...props} voice={voice} />;
}

function ContextScanReviewEditor({
  draft,
  voice,
  onApproved,
  onRegenerate,
  testerDebounceMs,
}: ContextScanReviewProps & { voice: NonNullable<ReturnType<typeof scanVoice>> }) {
  const api = useApi();
  const queryClient = useQueryClient();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const termLocale = activeWorkspace?.languages?.[0] ?? "en";

  // The vocabulary is read by kind rather than by position, so a kind added
  // later cannot quietly become "the terms" here.
  const proposedProfile = voice.profile;
  const proposedTerms = useMemo(() => scanTerms(draft), [draft]);
  const proposedAxes = useMemo(() => scanAxes(draft), [draft]);

  const [name, setName] = useState(proposedProfile.name);
  const [description, setDescription] = useState(proposedProfile.description ?? "");
  const [tone, setTone] = useState<ToneProfile>(proposedProfile.tone);
  const [style, setStyle] = useState<StyleRules>(proposedProfile.style);
  const [vocabulary, setVocabulary] = useState<VocabularyRules>(proposedProfile.vocabulary);
  const [examples, setExamples] = useState<VoiceExample[]>(proposedProfile.examples ?? []);
  const [selectedTerms, setSelectedTerms] = useState<Set<number>>(
    () => new Set(proposedTerms.map((_, i) => i)),
  );
  const [approving, setApproving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const evidence = voice.evidence?.fields ?? {};

  /** The draft as currently edited — fed to the live tester and to Approve. */
  const editedProfile = useMemo<VoiceProfile>(
    () => ({ ...proposedProfile, name, description, tone, style, vocabulary, examples }),
    [proposedProfile, name, description, tone, style, vocabulary, examples],
  );

  const toggleTerm = useCallback((index: number) => {
    setSelectedTerms((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  }, []);

  const handleApprove = useCallback(async () => {
    if (!name.trim() || approving) return;
    setApproving(true);
    setError(null);
    try {
      const profile = await api.createVoiceProfile(ws, {
        name: name.trim(),
        description,
        tone,
        style,
        vocabulary,
        examples,
      });
      const terms: ContextScanTerm[] = proposedTerms.filter((_, i) => selectedTerms.has(i));
      for (const term of terms) {
        // Candidates enter curation as "proposed": creating a term directly as
        // preferred/forbidden is a governed transition that requires a
        // change-set, and the scan must never bypass that workflow.
        await api.createConcept(ws, {
          domain: term.domain,
          definition: term.definition,
          terms: [{ text: term.term, locale: termLocale, status: "proposed" }],
        });
      }
      void queryClient.invalidateQueries({ queryKey: ["voice-profiles", ws] });
      void queryClient.invalidateQueries({ queryKey: ["concepts", ws] });
      onApproved(profile);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setApproving(false);
    }
  }, [
    api,
    queryClient,
    ws,
    name,
    description,
    tone,
    style,
    vocabulary,
    examples,
    proposedTerms,
    selectedTerms,
    termLocale,
    approving,
    onApproved,
  ]);

  return (
    <div data-testid={TEST_IDS.contextScan.review} className="flex gap-6">
      <div className="flex-1 min-w-0 space-y-4">
        <ContextScanAxes axes={proposedAxes} />

        {/* Provenance */}
        <Card className="p-5 space-y-2">
          <h2 className="text-sm font-semibold">What the scan read</h2>
          <ul className="text-xs text-muted-foreground space-y-1">
            {draft.sources.map((s) => (
              <li key={`${s.kind}-${s.label}`} className="flex items-center gap-2">
                <Badge variant="secondary" className="text-[10px] uppercase tracking-wide">
                  {s.kind}
                </Badge>
                <span className="truncate">{s.label}</span>
                <span className="tabular-nums shrink-0">{s.runes.toLocaleString()} characters</span>
              </li>
            ))}
          </ul>
          {draft.truncated && (
            <p className="text-xs text-muted-foreground">
              The sources exceeded the corpus cap, so the draft rests on a truncated sample.
            </p>
          )}
        </Card>

        {/* Identity */}
        <Card className="p-5 space-y-4">
          <h2 className="text-sm font-semibold">Profile</h2>
          <div className="space-y-2">
            <Label htmlFor="scan-profile-name">Name</Label>
            <Input
              id="scan-profile-name"
              value={name}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setName(e.target.value)}
              placeholder="Name this voice"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="scan-profile-desc">Description</Label>
            <Input
              id="scan-profile-desc"
              value={description}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setDescription(e.target.value)}
              placeholder="Brief description of this voice profile"
            />
          </div>
        </Card>

        {/* Tone */}
        <Card className="p-5 space-y-5">
          <div className="flex items-start justify-between gap-3 flex-wrap">
            <h2 className="text-sm font-semibold">Tone</h2>
            <FieldEvidence evidence={evidence.tone} />
          </div>
          <PersonalityTagPicker
            tags={tone.personality}
            onChange={(personality) => setTone((prev) => ({ ...prev, personality }))}
          />
          <ToneSpectrumSelector
            label="Formality"
            options={formalitySpectrum}
            value={tone.formality}
            onChange={(formality) =>
              setTone((prev) => ({ ...prev, formality: formality as ToneProfile["formality"] }))
            }
          />
          <ToneSpectrumSelector
            label="Emotion"
            options={emotionSpectrum}
            value={tone.emotion}
            onChange={(emotion) =>
              setTone((prev) => ({ ...prev, emotion: emotion as ToneProfile["emotion"] }))
            }
          />
          <ToneSpectrumSelector
            label="Humor"
            options={humorSpectrum}
            value={tone.humor}
            onChange={(humor) =>
              setTone((prev) => ({ ...prev, humor: humor as ToneProfile["humor"] }))
            }
          />
        </Card>

        {/* Style */}
        <Card className="p-5 space-y-5">
          <div className="flex items-start justify-between gap-3 flex-wrap">
            <h2 className="text-sm font-semibold">Style</h2>
            <FieldEvidence evidence={evidence.style} />
          </div>
          <div className="flex items-center justify-between rounded-md border border-border/50 bg-muted/30 px-3 py-2.5">
            <div>
              <div className="text-sm font-medium">Active voice</div>
              <div className="text-xs text-muted-foreground">
                Prefer active voice over passive constructions
              </div>
            </div>
            <Switch
              checked={style.active_voice}
              onCheckedChange={(v: boolean) => setStyle((prev) => ({ ...prev, active_voice: v }))}
            />
          </div>
          <ToneSpectrumSelector
            label="Sentence length"
            options={sentenceLengthSpectrum}
            value={style.sentence_length}
            onChange={(v) =>
              setStyle((prev) => ({
                ...prev,
                sentence_length: v as StyleRules["sentence_length"],
              }))
            }
          />
          <ToneSpectrumSelector
            label="Point of view"
            options={povSpectrum}
            value={style.person_pov}
            onChange={(v) =>
              setStyle((prev) => ({ ...prev, person_pov: v as StyleRules["person_pov"] }))
            }
          />
          <ToneSpectrumSelector
            label="Contractions"
            options={contractionsSpectrum}
            value={style.contractions}
            onChange={(v) =>
              setStyle((prev) => ({ ...prev, contractions: v as StyleRules["contractions"] }))
            }
          />
        </Card>

        {/* Vocabulary */}
        <Card className="p-5 space-y-4">
          <div className="flex items-start justify-between gap-3 flex-wrap">
            <h2 className="text-sm font-semibold">Vocabulary</h2>
            <FieldEvidence evidence={evidence.vocabulary} />
          </div>
          <VocabularyEditor vocabulary={vocabulary} onChange={setVocabulary} />
        </Card>

        {/* Examples */}
        <Card className="p-5 space-y-4">
          <div className="flex items-start justify-between gap-3 flex-wrap">
            <h2 className="text-sm font-semibold">Examples</h2>
            <FieldEvidence evidence={evidence.examples} />
          </div>
          <ExamplesEditor examples={examples} onChange={setExamples} />
        </Card>

        {/* Candidate terms */}
        {proposedTerms.length > 0 && (
          <Card className="p-5 space-y-3">
            <div>
              <h2 className="text-sm font-semibold">Candidate terms</h2>
              <p className="text-xs text-muted-foreground mt-1">
                Terms the scan found in your material. Checked terms are created as concepts when
                you approve: {selectedTerms.size} of {proposedTerms.length} selected.
              </p>
            </div>
            <ul className="divide-y divide-border/50">
              {proposedTerms.map((term, i) => (
                <li
                  key={`${term.term}-${i}`}
                  data-testid={TEST_IDS.contextScan.termRow}
                  className="flex items-start gap-3 py-2.5"
                >
                  <Checkbox
                    aria-label={`Include ${term.term}`}
                    checked={selectedTerms.has(i)}
                    onCheckedChange={() => toggleTerm(i)}
                    className="mt-0.5"
                  />
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{term.term}</span>
                      {term.domain && (
                        <Badge variant="secondary" className="text-[10px]">
                          {term.domain}
                        </Badge>
                      )}
                    </div>
                    {term.definition && (
                      <p className="text-xs text-muted-foreground mt-0.5">{term.definition}</p>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          </Card>
        )}

        {error && (
          <div className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
            {error}
          </div>
        )}

        {/* Actions */}
        <div className="flex items-center justify-between gap-3">
          <div>
            {onRegenerate && (
              <Button variant="outline" onClick={onRegenerate} disabled={approving}>
                <Wand2 className="w-3.5 h-3.5 mr-1.5" /> Regenerate
              </Button>
            )}
          </div>
          <Button
            data-testid={TEST_IDS.contextScan.approve}
            onClick={handleApprove}
            disabled={!name.trim() || approving}
          >
            {approving ? (
              <>
                <Loader2 className="w-4 h-4 mr-1.5 animate-spin" /> Creating…
              </>
            ) : (
              "Approve and create profile"
            )}
          </Button>
        </div>
      </div>

      {/* Right rail: preview + live tester */}
      <div className="hidden lg:block w-80 shrink-0">
        <div className="sticky top-6 space-y-4">
          <VoicePreview tone={tone} style={style} />
          <ContextScanLiveTester profile={editedProfile} debounceMs={testerDebounceMs} />
        </div>
      </div>
    </div>
  );
}
