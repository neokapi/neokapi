import { useCallback } from "react";
import type { VocabularyRules, TermRule } from "./types";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Plus, Trash2 } from "../components/icons";

interface VocabularyEditorProps {
  vocabulary: VocabularyRules;
  onChange: (vocabulary: VocabularyRules) => void;
}

type TermCategory = "preferred_terms" | "forbidden_terms" | "competitor_terms";

const categoryLabels: Record<TermCategory, string> = {
  preferred_terms: "Preferred Terms",
  forbidden_terms: "Forbidden Terms",
  competitor_terms: "Competitor Terms",
};

const categoryDescriptions: Record<TermCategory, string> = {
  preferred_terms: "Terms your brand should consistently use",
  forbidden_terms: "Terms to avoid, with suggested replacements",
  competitor_terms: "Competitor names or branded terms to avoid",
};

export function VocabularyEditor({ vocabulary, onChange }: VocabularyEditorProps) {
  const addTerm = useCallback(
    (category: TermCategory) => {
      onChange({
        ...vocabulary,
        [category]: [...(vocabulary[category] ?? []), { term: "", replacement: "", note: "" }],
      });
    },
    [vocabulary, onChange],
  );

  const updateTerm = useCallback(
    (category: TermCategory, index: number, field: keyof TermRule, value: string) => {
      const list = [...(vocabulary[category] ?? [])];
      list[index] = { ...list[index], [field]: value };
      onChange({ ...vocabulary, [category]: list });
    },
    [vocabulary, onChange],
  );

  const removeTerm = useCallback(
    (category: TermCategory, index: number) => {
      const list = [...(vocabulary[category] ?? [])];
      list.splice(index, 1);
      onChange({ ...vocabulary, [category]: list });
    },
    [vocabulary, onChange],
  );

  return (
    <div className="space-y-6">
      {(["preferred_terms", "forbidden_terms", "competitor_terms"] as const).map((category) => (
        <div key={category} className="space-y-2">
          <div className="flex items-center justify-between">
            <div>
              <Label>{categoryLabels[category]}</Label>
              <p className="text-xs text-muted-foreground mt-0.5">
                {categoryDescriptions[category]}
              </p>
            </div>
            <Button variant="outline" size="sm" onClick={() => addTerm(category)}>
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
                onChange={(e) => updateTerm(category, i, "term", e.target.value)}
              />
              <Input
                placeholder="Replacement"
                value={rule.replacement ?? ""}
                onChange={(e) => updateTerm(category, i, "replacement", e.target.value)}
              />
              <Input
                placeholder="Note"
                value={rule.note ?? ""}
                onChange={(e) => updateTerm(category, i, "note", e.target.value)}
              />
              <Button variant="ghost" size="icon" onClick={() => removeTerm(category, i)}>
                <Trash2 className="w-3.5 h-3.5" />
              </Button>
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}
