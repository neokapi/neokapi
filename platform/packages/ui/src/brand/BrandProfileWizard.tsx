import { useState, useCallback } from "react";
import type { VoiceProfile, ToneProfile, StyleRules, VocabularyRules, VoiceExample } from "./types";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Card } from "../components/ui/card";
import { Switch } from "../components/ui/switch";
import { ArrowLeft, Check } from "../components/icons";
import { defaultTone, defaultStyle, defaultVocabulary } from "./defaults";
import { ToneSpectrumSelector } from "./ToneSpectrumSelector";
import { PersonalityTagPicker } from "./PersonalityTagPicker";
import { BrandVoicePreview } from "./BrandVoicePreview";
import { PatternListEditor } from "./PatternListEditor";
import { VocabularyEditor } from "./VocabularyEditor";
import { ExamplesEditor } from "./ExamplesEditor";
import {
  formalitySpectrum,
  emotionSpectrum,
  humorSpectrum,
  sentenceLengthSpectrum,
  povSpectrum,
  contractionsSpectrum,
} from "./data/tone-spectrums";

interface BrandProfileWizardProps {
  profile?: VoiceProfile;
  onSave: (
    data: Omit<
      VoiceProfile,
      "id" | "workspace_id" | "version" | "created_at" | "updated_at" | "created_by"
    >,
  ) => void;
  onCancel: () => void;
}

const steps = [
  { key: "identity", label: "Identity" },
  { key: "tone", label: "Tone" },
  { key: "style", label: "Style" },
  { key: "content", label: "Vocabulary & Examples" },
] as const;

type StepKey = (typeof steps)[number]["key"];

export function BrandProfileWizard({ profile, onSave, onCancel }: BrandProfileWizardProps) {
  const [currentStep, setCurrentStep] = useState<StepKey>("identity");
  const [name, setName] = useState(profile?.name ?? "");
  const [description, setDescription] = useState(profile?.description ?? "");
  const [tone, setTone] = useState<ToneProfile>(profile?.tone ?? defaultTone());
  const [style, setStyle] = useState<StyleRules>(profile?.style ?? defaultStyle());
  const [vocabulary, setVocabulary] = useState<VocabularyRules>(
    profile?.vocabulary ?? defaultVocabulary(),
  );
  const [examples, setExamples] = useState<VoiceExample[]>(profile?.examples ?? []);

  const currentIndex = steps.findIndex((s) => s.key === currentStep);

  const handleNext = useCallback(() => {
    if (currentIndex < steps.length - 1) {
      setCurrentStep(steps[currentIndex + 1].key);
    }
  }, [currentIndex]);

  const handleBack = useCallback(() => {
    if (currentIndex > 0) {
      setCurrentStep(steps[currentIndex - 1].key);
    }
  }, [currentIndex]);

  const handleSubmit = useCallback(() => {
    onSave({ name, description, tone, style, vocabulary, examples });
  }, [name, description, tone, style, vocabulary, examples, onSave]);

  const isLastStep = currentIndex === steps.length - 1;
  const canProceed = currentStep === "identity" ? name.trim().length > 0 : true;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={onCancel}>
          <ArrowLeft className="w-4 h-4" />
        </Button>
        <h1 className="text-lg font-semibold">
          {profile ? "Edit Profile" : "New Brand Voice Profile"}
        </h1>
      </div>

      <div className="flex gap-6">
        {/* Left: Stepper + Content */}
        <div className="flex-1 min-w-0 space-y-6">
          {/* Step indicator */}
          <nav className="flex gap-1">
            {steps.map((step, i) => {
              const isActive = step.key === currentStep;
              const isComplete = i < currentIndex;
              return (
                <button
                  key={step.key}
                  type="button"
                  onClick={() => setCurrentStep(step.key)}
                  className={`flex items-center gap-2 rounded-full px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer border-none ${
                    isActive
                      ? "bg-primary text-primary-foreground"
                      : isComplete
                        ? "bg-primary/10 text-primary"
                        : "bg-muted text-muted-foreground"
                  }`}
                >
                  <span
                    className={`flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-bold ${
                      isActive
                        ? "bg-primary-foreground/20"
                        : isComplete
                          ? "bg-primary/20"
                          : "bg-muted-foreground/20"
                    }`}
                  >
                    {isComplete ? <Check className="w-3 h-3" /> : i + 1}
                  </span>
                  {step.label}
                </button>
              );
            })}
          </nav>

          {/* Step content */}
          {currentStep === "identity" && (
            <Card className="p-5 space-y-4">
              <div>
                <h2 className="text-sm font-semibold">Profile Identity</h2>
                <p className="text-xs text-muted-foreground mt-1">
                  Give your brand voice a name and brief description.
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="profile-name">Name</Label>
                <Input
                  id="profile-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Enterprise Documentation"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="profile-desc">Description</Label>
                <Input
                  id="profile-desc"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Brief description of this voice profile"
                />
              </div>
            </Card>
          )}

          {currentStep === "tone" && (
            <Card className="p-5 space-y-6">
              <div>
                <h2 className="text-sm font-semibold">Tone & Personality</h2>
                <p className="text-xs text-muted-foreground mt-1">
                  Define how your brand sounds. Pick personality traits and adjust the dials.
                </p>
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
                  setTone((prev) => ({
                    ...prev,
                    formality: formality as ToneProfile["formality"],
                  }))
                }
              />
              <ToneSpectrumSelector
                label="Emotion"
                options={emotionSpectrum}
                value={tone.emotion}
                onChange={(emotion) =>
                  setTone((prev) => ({
                    ...prev,
                    emotion: emotion as ToneProfile["emotion"],
                  }))
                }
              />
              <ToneSpectrumSelector
                label="Humor"
                options={humorSpectrum}
                value={tone.humor}
                onChange={(humor) =>
                  setTone((prev) => ({
                    ...prev,
                    humor: humor as ToneProfile["humor"],
                  }))
                }
              />
            </Card>
          )}

          {currentStep === "style" && (
            <Card className="p-5 space-y-6">
              <div>
                <h2 className="text-sm font-semibold">Writing Style</h2>
                <p className="text-xs text-muted-foreground mt-1">
                  Control the structural aspects of your writing.
                </p>
              </div>

              <div className="flex items-center justify-between rounded-md border border-border/50 bg-muted/30 px-3 py-2.5">
                <div>
                  <div className="text-sm font-medium">Active Voice</div>
                  <div className="text-xs text-muted-foreground">
                    Prefer active voice over passive constructions
                  </div>
                </div>
                <Switch
                  checked={style.active_voice}
                  onCheckedChange={(v: boolean) =>
                    setStyle((prev) => ({ ...prev, active_voice: v }))
                  }
                />
              </div>

              <ToneSpectrumSelector
                label="Sentence Length"
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
                label="Point of View"
                options={povSpectrum}
                value={style.person_pov}
                onChange={(v) =>
                  setStyle((prev) => ({
                    ...prev,
                    person_pov: v as StyleRules["person_pov"],
                  }))
                }
              />
              <ToneSpectrumSelector
                label="Contractions"
                options={contractionsSpectrum}
                value={style.contractions}
                onChange={(v) =>
                  setStyle((prev) => ({
                    ...prev,
                    contractions: v as StyleRules["contractions"],
                  }))
                }
              />

              <PatternListEditor
                label="Prohibited Patterns"
                patterns={style.prohibited_patterns ?? []}
                onChange={(prohibited_patterns) =>
                  setStyle((prev) => ({ ...prev, prohibited_patterns }))
                }
              />
              <PatternListEditor
                label="Required Patterns"
                patterns={style.required_patterns ?? []}
                onChange={(required_patterns) =>
                  setStyle((prev) => ({ ...prev, required_patterns }))
                }
              />
            </Card>
          )}

          {currentStep === "content" && (
            <div className="space-y-6">
              <Card className="p-5">
                <VocabularyEditor vocabulary={vocabulary} onChange={setVocabulary} />
              </Card>
              <Card className="p-5">
                <ExamplesEditor examples={examples} onChange={setExamples} />
              </Card>
            </div>
          )}

          {/* Navigation */}
          <div className="flex items-center gap-3 justify-between">
            <div>
              {currentIndex > 0 && (
                <Button variant="outline" onClick={handleBack}>
                  Back
                </Button>
              )}
            </div>
            <div className="flex items-center gap-3">
              <Button variant="outline" onClick={onCancel}>
                Cancel
              </Button>
              {isLastStep ? (
                <Button onClick={handleSubmit} disabled={!name.trim()}>
                  {profile ? "Save Changes" : "Create Profile"}
                </Button>
              ) : (
                <Button onClick={handleNext} disabled={!canProceed}>
                  Next
                </Button>
              )}
            </div>
          </div>
        </div>

        {/* Right: Live preview (hidden on small screens) */}
        <div className="hidden lg:block w-72 shrink-0">
          <div className="sticky top-6">
            <BrandVoicePreview tone={tone} style={style} />
          </div>
        </div>
      </div>
    </div>
  );
}
