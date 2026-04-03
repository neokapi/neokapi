import { useState, useCallback } from "react";
import type {
  VoiceProfile,
  ToneProfile,
  StyleRules,
  VocabularyRules,
  TermRule,
  Pattern,
  VoiceExample,
} from "./types";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import { Label } from "@neokapi/ui-primitives/components/ui/label";
import { Badge } from "@neokapi/ui-primitives/components/ui/badge";
import { Card } from "@neokapi/ui-primitives/components/ui/card";
import { Switch } from "@neokapi/ui-primitives/components/ui/switch";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@neokapi/ui-primitives/components/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@neokapi/ui-primitives/components/ui/tabs";
import { Plus, Trash2, ArrowLeft, X } from "../components/icons";

interface BrandProfileEditorProps {
  profile?: VoiceProfile;
  onSave: (
    data: Omit<
      VoiceProfile,
      "id" | "workspace_id" | "version" | "created_at" | "updated_at" | "created_by"
    >,
  ) => void;
  onCancel: () => void;
}

function defaultTone(): ToneProfile {
  return { personality: [], formality: "neutral", emotion: "neutral", humor: "none" };
}

function defaultStyle(): StyleRules {
  return {
    active_voice: true,
    sentence_length: "medium",
    person_pov: "second",
    contractions: "sometimes",
    prohibited_patterns: [],
    required_patterns: [],
  };
}

function defaultVocabulary(): VocabularyRules {
  return { preferred_terms: [], forbidden_terms: [], competitor_terms: [] };
}

export function BrandProfileEditor({ profile, onSave, onCancel }: BrandProfileEditorProps) {
  const [name, setName] = useState(profile?.name ?? "");
  const [description, setDescription] = useState(profile?.description ?? "");
  const [tone, setTone] = useState<ToneProfile>(profile?.tone ?? defaultTone());
  const [style, setStyle] = useState<StyleRules>(profile?.style ?? defaultStyle());
  const [vocabulary, setVocabulary] = useState<VocabularyRules>(
    profile?.vocabulary ?? defaultVocabulary(),
  );
  const [examples, setExamples] = useState<VoiceExample[]>(profile?.examples ?? []);
  const [personalityInput, setPersonalityInput] = useState("");

  const handleSubmit = useCallback(() => {
    onSave({ name, description, tone, style, vocabulary, examples });
  }, [name, description, tone, style, vocabulary, examples, onSave]);

  const addPersonalityTag = useCallback(() => {
    const tag = personalityInput.trim().toLowerCase();
    if (tag && !tone.personality.includes(tag)) {
      setTone((prev) => ({ ...prev, personality: [...prev.personality, tag] }));
      setPersonalityInput("");
    }
  }, [personalityInput, tone.personality]);

  const removePersonalityTag = useCallback((tag: string) => {
    setTone((prev) => ({ ...prev, personality: prev.personality.filter((t) => t !== tag) }));
  }, []);

  const addTermRule = useCallback(
    (category: "preferred_terms" | "forbidden_terms" | "competitor_terms") => {
      setVocabulary((prev) => ({
        ...prev,
        [category]: [...(prev[category] ?? []), { term: "", replacement: "", note: "" }],
      }));
    },
    [],
  );

  const updateTermRule = useCallback(
    (
      category: "preferred_terms" | "forbidden_terms" | "competitor_terms",
      index: number,
      field: keyof TermRule,
      value: string,
    ) => {
      setVocabulary((prev) => {
        const list = [...(prev[category] ?? [])];
        list[index] = { ...list[index], [field]: value };
        return { ...prev, [category]: list };
      });
    },
    [],
  );

  const removeTermRule = useCallback(
    (category: "preferred_terms" | "forbidden_terms" | "competitor_terms", index: number) => {
      setVocabulary((prev) => {
        const list = [...(prev[category] ?? [])];
        list.splice(index, 1);
        return { ...prev, [category]: list };
      });
    },
    [],
  );

  const addPattern = useCallback((category: "prohibited_patterns" | "required_patterns") => {
    setStyle((prev) => ({
      ...prev,
      [category]: [
        ...(prev[category] ?? []),
        { regex: "", description: "", severity: "minor" as const },
      ],
    }));
  }, []);

  const updatePattern = useCallback(
    (
      category: "prohibited_patterns" | "required_patterns",
      index: number,
      field: keyof Pattern,
      value: string,
    ) => {
      setStyle((prev) => {
        const list = [...(prev[category] ?? [])];
        list[index] = { ...list[index], [field]: value };
        return { ...prev, [category]: list };
      });
    },
    [],
  );

  const removePattern = useCallback(
    (category: "prohibited_patterns" | "required_patterns", index: number) => {
      setStyle((prev) => {
        const list = [...(prev[category] ?? [])];
        list.splice(index, 1);
        return { ...prev, [category]: list };
      });
    },
    [],
  );

  const addExample = useCallback(() => {
    setExamples((prev) => [...prev, { before: "", after: "", explanation: "" }]);
  }, []);

  const updateExample = useCallback((index: number, field: keyof VoiceExample, value: string) => {
    setExamples((prev) => {
      const list = [...prev];
      list[index] = { ...list[index], [field]: value };
      return list;
    });
  }, []);

  const removeExample = useCallback((index: number) => {
    setExamples((prev) => prev.filter((_, i) => i !== index));
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={onCancel}>
          <ArrowLeft className="w-4 h-4" />
        </Button>
        <h1 className="text-lg font-semibold">
          {profile ? "Edit Profile" : "New Brand Voice Profile"}
        </h1>
      </div>

      {/* Name & Description */}
      <Card className="p-5 space-y-4">
        <div className="space-y-2">
          <Label htmlFor="profile-name">Name</Label>
          <Input
            id="profile-name"
            value={name}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setName(e.target.value)}
            placeholder="e.g. Enterprise Documentation"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="profile-desc">Description</Label>
          <Input
            id="profile-desc"
            value={description}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setDescription(e.target.value)}
            placeholder="Brief description of this voice profile"
          />
        </div>
      </Card>

      <Tabs defaultValue="tone">
        <TabsList className="w-full grid grid-cols-4">
          <TabsTrigger value="tone">Tone</TabsTrigger>
          <TabsTrigger value="style">Style</TabsTrigger>
          <TabsTrigger value="vocabulary">Vocabulary</TabsTrigger>
          <TabsTrigger value="examples">Examples</TabsTrigger>
        </TabsList>

        {/* Tone Tab */}
        <TabsContent value="tone">
          <Card className="p-5 space-y-4">
            <div className="space-y-2">
              <Label>Personality Tags</Label>
              <div className="flex flex-wrap gap-1.5 mb-2">
                {tone.personality.map((tag) => (
                  <Badge key={tag} variant="secondary" className="gap-1">
                    {tag}
                    <button
                      onClick={() => removePersonalityTag(tag)}
                      className="ml-0.5 hover:text-destructive bg-transparent border-none cursor-pointer p-0"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </Badge>
                ))}
              </div>
              <div className="flex gap-2">
                <Input
                  value={personalityInput}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) => setPersonalityInput(e.target.value)}
                  placeholder="Add tag (e.g. friendly, professional)"
                  onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => e.key === "Enter" && (e.preventDefault(), addPersonalityTag())}
                />
                <Button variant="outline" size="sm" onClick={addPersonalityTag}>
                  Add
                </Button>
              </div>
            </div>

            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label>Formality</Label>
                <Select
                  value={tone.formality}
                  onValueChange={(v: string) =>
                    setTone((prev) => ({ ...prev, formality: v as ToneProfile["formality"] }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="casual">Casual</SelectItem>
                    <SelectItem value="neutral">Neutral</SelectItem>
                    <SelectItem value="formal">Formal</SelectItem>
                    <SelectItem value="technical">Technical</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>Emotion</Label>
                <Select
                  value={tone.emotion}
                  onValueChange={(v: string) =>
                    setTone((prev) => ({ ...prev, emotion: v as ToneProfile["emotion"] }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="warm">Warm</SelectItem>
                    <SelectItem value="neutral">Neutral</SelectItem>
                    <SelectItem value="authoritative">Authoritative</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>Humor</Label>
                <Select
                  value={tone.humor}
                  onValueChange={(v: string) =>
                    setTone((prev) => ({ ...prev, humor: v as ToneProfile["humor"] }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">None</SelectItem>
                    <SelectItem value="light">Light</SelectItem>
                    <SelectItem value="frequent">Frequent</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          </Card>
        </TabsContent>

        {/* Style Tab */}
        <TabsContent value="style">
          <Card className="p-5 space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="flex items-center justify-between">
                <Label>Active Voice</Label>
                <Switch
                  checked={style.active_voice}
                  onCheckedChange={(v: boolean) =>
                    setStyle((prev) => ({ ...prev, active_voice: v }))
                  }
                />
              </div>

              <div className="space-y-2">
                <Label>Sentence Length</Label>
                <Select
                  value={style.sentence_length}
                  onValueChange={(v: string) =>
                    setStyle((prev) => ({
                      ...prev,
                      sentence_length: v as StyleRules["sentence_length"],
                    }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="short">Short</SelectItem>
                    <SelectItem value="medium">Medium</SelectItem>
                    <SelectItem value="varied">Varied</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>Point of View</Label>
                <Select
                  value={style.person_pov}
                  onValueChange={(v: string) =>
                    setStyle((prev) => ({ ...prev, person_pov: v as StyleRules["person_pov"] }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="first_plural">We (first person plural)</SelectItem>
                    <SelectItem value="second">You (second person)</SelectItem>
                    <SelectItem value="third">They (third person)</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>Contractions</Label>
                <Select
                  value={style.contractions}
                  onValueChange={(v: string) =>
                    setStyle((prev) => ({ ...prev, contractions: v as StyleRules["contractions"] }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="always">Always</SelectItem>
                    <SelectItem value="sometimes">Sometimes</SelectItem>
                    <SelectItem value="never">Never</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Prohibited Patterns */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Prohibited Patterns</Label>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => addPattern("prohibited_patterns")}
                >
                  <Plus className="w-3.5 h-3.5 mr-1" /> Add
                </Button>
              </div>
              {(style.prohibited_patterns ?? []).map((pat, i) => (
                <div key={i} className="flex gap-2 items-start">
                  <Input
                    placeholder="Regex"
                    value={pat.regex}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      updatePattern("prohibited_patterns", i, "regex", e.target.value)
                    }
                    className="font-mono text-xs"
                  />
                  <Input
                    placeholder="Description"
                    value={pat.description}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      updatePattern("prohibited_patterns", i, "description", e.target.value)
                    }
                  />
                  <Select
                    value={pat.severity}
                    onValueChange={(v: string) =>
                      updatePattern("prohibited_patterns", i, "severity", v)
                    }
                  >
                    <SelectTrigger className="w-28">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="minor">Minor</SelectItem>
                      <SelectItem value="major">Major</SelectItem>
                      <SelectItem value="critical">Critical</SelectItem>
                    </SelectContent>
                  </Select>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => removePattern("prohibited_patterns", i)}
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </Button>
                </div>
              ))}
            </div>

            {/* Required Patterns */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Required Patterns</Label>
                <Button variant="outline" size="sm" onClick={() => addPattern("required_patterns")}>
                  <Plus className="w-3.5 h-3.5 mr-1" /> Add
                </Button>
              </div>
              {(style.required_patterns ?? []).map((pat, i) => (
                <div key={i} className="flex gap-2 items-start">
                  <Input
                    placeholder="Regex"
                    value={pat.regex}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => updatePattern("required_patterns", i, "regex", e.target.value)}
                    className="font-mono text-xs"
                  />
                  <Input
                    placeholder="Description"
                    value={pat.description}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      updatePattern("required_patterns", i, "description", e.target.value)
                    }
                  />
                  <Select
                    value={pat.severity}
                    onValueChange={(v: string) =>
                      updatePattern("required_patterns", i, "severity", v)
                    }
                  >
                    <SelectTrigger className="w-28">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="minor">Minor</SelectItem>
                      <SelectItem value="major">Major</SelectItem>
                      <SelectItem value="critical">Critical</SelectItem>
                    </SelectContent>
                  </Select>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => removePattern("required_patterns", i)}
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </Button>
                </div>
              ))}
            </div>
          </Card>
        </TabsContent>

        {/* Vocabulary Tab */}
        <TabsContent value="vocabulary">
          <Card className="p-5 space-y-6">
            {(["preferred_terms", "forbidden_terms", "competitor_terms"] as const).map(
              (category) => (
                <div key={category} className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label className="capitalize">
                      {category.replace("_terms", "").replace("_", " ")} Terms
                    </Label>
                    <Button variant="outline" size="sm" onClick={() => addTermRule(category)}>
                      <Plus className="w-3.5 h-3.5 mr-1" /> Add
                    </Button>
                  </div>
                  {(vocabulary[category] ?? []).length === 0 && (
                    <p className="text-xs text-muted-foreground">No terms defined.</p>
                  )}
                  {(vocabulary[category] ?? []).map((rule, i) => (
                    <div key={i} className="flex gap-2 items-start">
                      <Input
                        placeholder="Term"
                        value={rule.term}
                        onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateTermRule(category, i, "term", e.target.value)}
                      />
                      <Input
                        placeholder="Replacement"
                        value={rule.replacement ?? ""}
                        onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateTermRule(category, i, "replacement", e.target.value)}
                      />
                      <Input
                        placeholder="Note"
                        value={rule.note ?? ""}
                        onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateTermRule(category, i, "note", e.target.value)}
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => removeTermRule(category, i)}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  ))}
                </div>
              ),
            )}
          </Card>
        </TabsContent>

        {/* Examples Tab */}
        <TabsContent value="examples">
          <Card className="p-5 space-y-4">
            <div className="flex items-center justify-between">
              <Label>Voice Examples (Before / After)</Label>
              <Button variant="outline" size="sm" onClick={addExample}>
                <Plus className="w-3.5 h-3.5 mr-1" /> Add Example
              </Button>
            </div>
            {examples.length === 0 && (
              <p className="text-xs text-muted-foreground text-center py-4">
                No examples yet. Add before/after pairs to illustrate your brand voice.
              </p>
            )}
            {examples.map((ex, i) => (
              <div key={i} className="space-y-2 rounded-md border p-3">
                <div className="flex justify-end">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-6 w-6"
                    onClick={() => removeExample(i)}
                  >
                    <Trash2 className="w-3 h-3" />
                  </Button>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1">
                    <Label className="text-xs">Before</Label>
                    <Input
                      value={ex.before}
                      onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateExample(i, "before", e.target.value)}
                      placeholder="Original text"
                    />
                  </div>
                  <div className="space-y-1">
                    <Label className="text-xs">After</Label>
                    <Input
                      value={ex.after}
                      onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateExample(i, "after", e.target.value)}
                      placeholder="Brand-compliant text"
                    />
                  </div>
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">Explanation</Label>
                  <Input
                    value={ex.explanation ?? ""}
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateExample(i, "explanation", e.target.value)}
                    placeholder="Why this change?"
                  />
                </div>
              </div>
            ))}
          </Card>
        </TabsContent>
      </Tabs>

      {/* Actions */}
      <div className="flex items-center gap-3 justify-end">
        <Button variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={!name.trim()}>
          {profile ? "Save Changes" : "Create Profile"}
        </Button>
      </div>
    </div>
  );
}
