import { useState, useEffect, useCallback } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import {
  BrandProfileWizard,
  StarterPackPicker,
  useBrandProfile,
  useCreateBrandProfile,
  useUpdateBrandProfile,
} from "@neokapi/ui";
import type { StarterPackMeta } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";
import type { ToneProfile, StyleRules, VocabularyRules, VoiceExample } from "@neokapi/ui";

/** Full YAML-equivalent data for pre-populating the wizard from a starter pack. */
const starterPackDefaults: Record<
  string,
  {
    tone: ToneProfile;
    style: StyleRules;
    vocabulary: VocabularyRules;
    examples: VoiceExample[];
  }
> = {
  "professional-b2b": {
    tone: {
      personality: ["professional", "knowledgeable", "trustworthy"],
      formality: "formal",
      emotion: "authoritative",
      humor: "none",
      guidelines:
        "Maintain a professional, authoritative tone. Lead with data and expertise. Avoid casual language, slang, or humor.",
    },
    style: {
      active_voice: true,
      sentence_length: "medium",
      person_pov: "first_plural",
      contractions: "never",
      prohibited_patterns: [
        {
          regex: "\\b(gonna|wanna|gotta)\\b",
          description: "Informal contractions",
          severity: "major",
        },
        {
          regex: "!\\s*$",
          description: "Exclamation marks (too informal)",
          severity: "minor",
        },
      ],
    },
    vocabulary: {
      forbidden_terms: [
        { term: "leverage", replacement: "use", severity: "minor" },
        { term: "synergy", replacement: "collaboration", severity: "minor" },
        {
          term: "paradigm shift",
          replacement: "significant change",
          severity: "minor",
        },
      ],
      preferred_terms: [
        { term: "solution", note: "Use when describing products/services" },
        { term: "platform", note: "Preferred over 'tool' or 'system'" },
      ],
    },
    examples: [
      {
        before: "Hey! We've got this awesome new feature that's gonna blow your mind!",
        after:
          "We are pleased to introduce a new capability that addresses a key challenge in enterprise workflows.",
        explanation: "Replace casual enthusiasm with professional framing",
        category: "tone",
      },
    ],
  },
  "friendly-dtc": {
    tone: {
      personality: ["friendly", "approachable", "authentic"],
      formality: "casual",
      emotion: "warm",
      humor: "light",
      guidelines:
        "Write like you're talking to a friend. Be warm, genuine, and relatable. Use everyday language.",
    },
    style: {
      active_voice: true,
      sentence_length: "varied",
      person_pov: "second",
      contractions: "always",
      prohibited_patterns: [
        {
          regex: "\\b(pursuant to|hereby|aforementioned)\\b",
          description: "Legalese and overly formal language",
          severity: "major",
        },
      ],
    },
    vocabulary: {
      forbidden_terms: [
        { term: "utilize", replacement: "use", severity: "minor" },
        { term: "facilitate", replacement: "help", severity: "minor" },
        { term: "purchase", replacement: "buy", severity: "minor" },
      ],
      preferred_terms: [
        { term: "you", note: "Always address the customer directly" },
        { term: "easy", note: "Reinforce simplicity in messaging" },
      ],
    },
    examples: [
      {
        before: "We are pleased to inform you that your order has been dispatched.",
        after: "Great news \u2014 your order's on its way!",
        explanation: "Replace formal notification with warm, excited language",
        category: "tone",
      },
    ],
  },
  "marketing-blog": {
    tone: {
      personality: ["engaging", "conversational", "insightful"],
      formality: "neutral",
      emotion: "warm",
      humor: "light",
      guidelines:
        "Write as if having a conversation with an informed reader. Use storytelling techniques. Vary sentence length for rhythm.",
    },
    style: {
      active_voice: true,
      sentence_length: "varied",
      person_pov: "second",
      contractions: "sometimes",
      prohibited_patterns: [
        {
          regex: "\\b(in order to)\\b",
          description: "Wordy phrase (use 'to' instead)",
          severity: "minor",
        },
      ],
    },
    vocabulary: {
      forbidden_terms: [
        { term: "in order to", replacement: "to", severity: "minor" },
        { term: "thought leader", replacement: "expert", severity: "minor" },
      ],
      preferred_terms: [
        { term: "imagine", note: "Use to draw readers into scenarios" },
        { term: "let's", note: "Creates a collaborative feeling" },
      ],
    },
    examples: [
      {
        before: "This article will discuss the benefits of automated testing.",
        after:
          "Ever shipped a bug that a five-minute test would have caught? Let's talk about how automated testing saves your future self.",
        explanation: "Replace dry introduction with an engaging hook",
        category: "tone",
      },
    ],
  },
  "customer-support": {
    tone: {
      personality: ["empathetic", "helpful", "reassuring"],
      formality: "neutral",
      emotion: "warm",
      humor: "none",
      guidelines:
        "Lead with empathy. Focus on solutions, not problems. Be clear about next steps. Always end with a path forward.",
    },
    style: {
      active_voice: true,
      sentence_length: "medium",
      person_pov: "second",
      contractions: "sometimes",
      prohibited_patterns: [
        {
          regex: "\\b(you should have|you need to understand)\\b",
          description: "Blaming or condescending language",
          severity: "major",
        },
      ],
    },
    vocabulary: {
      forbidden_terms: [
        { term: "unfortunately", replacement: "", severity: "minor" },
        {
          term: "impossible",
          replacement: "not currently available",
          severity: "major",
        },
        { term: "calm down", replacement: "", severity: "major" },
      ],
      preferred_terms: [
        { term: "I understand", note: "Acknowledge the customer's concern" },
        { term: "happy to help", note: "Reinforces willingness to assist" },
      ],
    },
    examples: [
      {
        before:
          "Unfortunately, that feature is not available. You should have read the documentation.",
        after:
          "I understand you're looking for that feature. While it's not available yet, here's what we can do in the meantime.",
        explanation: "Replace blame with empathy and offer an alternative",
        category: "tone",
      },
    ],
  },
  "technical-docs": {
    tone: {
      personality: ["precise", "clear", "instructive"],
      formality: "technical",
      emotion: "neutral",
      humor: "none",
      guidelines:
        "Write with precision and clarity. Use imperative mood for instructions. Avoid ambiguity and unnecessary words.",
    },
    style: {
      active_voice: true,
      sentence_length: "short",
      person_pov: "third",
      contractions: "never",
      prohibited_patterns: [
        {
          regex: "\\b(simply|just|easily|obviously|clearly)\\b",
          description: "Minimizing words that assume reader knowledge",
          severity: "major",
        },
      ],
    },
    vocabulary: {
      forbidden_terms: [
        { term: "please", replacement: "", severity: "minor" },
        { term: "simple", replacement: "", severity: "minor" },
        { term: "obviously", replacement: "", severity: "major" },
      ],
      preferred_terms: [
        { term: "run", note: "Preferred over 'execute' for commands" },
        { term: "return", note: "Preferred over 'give back' or 'send back'" },
      ],
    },
    examples: [
      {
        before: "You can simply click the button to easily configure your settings.",
        after: "Click Settings to open the configuration panel.",
        explanation: "Remove minimizing language and use imperative mood",
        category: "style",
      },
    ],
  },
};

export function BrandEditorRoute() {
  const navigate = useNavigate();
  const { workspace, profileId } = useParams({ strict: false });
  const { activeWorkspace } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;

  const isNew = profileId === "new";
  const [showPicker, setShowPicker] = useState(false);
  const [starterProfile, setStarterProfile] = useState<{
    name: string;
    description: string;
    tone: ToneProfile;
    style: StyleRules;
    vocabulary: VocabularyRules;
    examples: VoiceExample[];
  } | null>(null);

  // Show picker on first render for new profiles (triggered via "From Starter" button)
  const [pickerShownOnce, setPickerShownOnce] = useState(false);

  const { data: profile } = useBrandProfile(isNew ? "" : (profileId ?? ""));
  const createMutation = useCreateBrandProfile();
  const updateMutation = useUpdateBrandProfile();

  useEffect(() => {
    const name = isNew ? "New Profile" : (profile?.name ?? "Edit Profile");
    document.title = `${name} \u2014 Brand Voice \u2014 ${activeWorkspace?.name ?? ""} \u2014 Bowrain`;
  }, [isNew, profile?.name, activeWorkspace?.name]);

  // Auto-show picker for new profiles on initial load
  useEffect(() => {
    if (isNew && !pickerShownOnce) {
      setShowPicker(true);
      setPickerShownOnce(true);
    }
  }, [isNew, pickerShownOnce]);

  const handleCancel = useCallback(() => {
    void navigate({
      to: "/$workspace/brand/voice",
      params: { workspace: workspace ?? "" },
    });
  }, [navigate, workspace]);

  const handleSave = useCallback(
    async (data: Parameters<typeof createMutation.mutateAsync>[0]) => {
      if (isNew) {
        await createMutation.mutateAsync(data);
      } else {
        await updateMutation.mutateAsync({ ...data, id: profileId ?? "" });
      }
      void navigate({
        to: "/$workspace/brand/voice",
        params: { workspace: workspace ?? "" },
      });
    },
    [isNew, profileId, createMutation, updateMutation, navigate, workspace],
  );

  const handleSelectPack = useCallback((pack: StarterPackMeta) => {
    const defaults = starterPackDefaults[pack.name];
    if (defaults) {
      setStarterProfile({
        name: pack.label,
        description: pack.description,
        ...defaults,
      });
    }
    setShowPicker(false);
  }, []);

  const handleScratch = useCallback(() => {
    setStarterProfile(null);
    setShowPicker(false);
  }, []);

  if (!isNew && !profile) {
    return (
      <div className="flex items-center justify-center h-64 text-sm text-muted-foreground">
        Loading profile...
      </div>
    );
  }

  // Build the profile to pass to the wizard
  const wizardProfile = isNew
    ? starterProfile
      ? ({
          id: "",
          name: starterProfile.name,
          description: starterProfile.description,
          tone: starterProfile.tone,
          style: starterProfile.style,
          vocabulary: starterProfile.vocabulary,
          examples: starterProfile.examples,
          workspace_id: "",
          version: 0,
          created_at: "",
          updated_at: "",
        } as import("@neokapi/ui").VoiceProfile)
      : undefined
    : (profile ?? undefined);

  return (
    <>
      <StarterPackPicker
        open={showPicker}
        onOpenChange={setShowPicker}
        onSelect={handleSelectPack}
        onScratch={handleScratch}
      />
      <BrandProfileWizard
        key={starterProfile?.name ?? "blank"}
        profile={wizardProfile}
        onSave={handleSave}
        onCancel={handleCancel}
      />
    </>
  );
}
