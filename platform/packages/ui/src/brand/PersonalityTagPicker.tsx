import { useState, useCallback } from "react";
import { Badge } from "../components/ui/badge";
import { Input } from "../components/ui/input";
import { X } from "../components/icons";
import { personalityTagCategories } from "./data/personality-tags";

interface PersonalityTagPickerProps {
  tags: string[];
  onChange: (tags: string[]) => void;
}

export function PersonalityTagPicker({ tags, onChange }: PersonalityTagPickerProps) {
  const [input, setInput] = useState("");

  const addTag = useCallback(
    (tag: string) => {
      const normalized = tag.trim().toLowerCase();
      if (normalized && !tags.includes(normalized)) {
        onChange([...tags, normalized]);
      }
    },
    [tags, onChange],
  );

  const removeTag = useCallback(
    (tag: string) => {
      onChange(tags.filter((t) => t !== tag));
    },
    [tags, onChange],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter") {
        e.preventDefault();
        addTag(input);
        setInput("");
      }
    },
    [input, addTag],
  );

  return (
    <div className="space-y-4">
      <div className="text-sm font-medium">Personality Tags</div>

      {/* Selected tags */}
      {tags.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {tags.map((tag) => (
            <Badge key={tag} variant="secondary" className="gap-1 px-2.5 py-1">
              {tag}
              <button
                type="button"
                onClick={() => removeTag(tag)}
                className="ml-0.5 hover:text-destructive bg-transparent border-none cursor-pointer p-0"
                aria-label={`Remove ${tag}`}
              >
                <X className="w-3 h-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}

      {/* Suggested tags by category */}
      <div className="space-y-3">
        {personalityTagCategories.map((category) => {
          const availableTags = category.tags.filter((t) => !tags.includes(t));
          if (availableTags.length === 0) return null;
          return (
            <div key={category.label} className="space-y-1.5">
              <div className="text-xs text-muted-foreground">{category.label}</div>
              <div className="flex flex-wrap gap-1">
                {category.tags.map((tag) => {
                  const isSelected = tags.includes(tag);
                  return (
                    <button
                      key={tag}
                      type="button"
                      disabled={isSelected}
                      onClick={() => addTag(tag)}
                      className={`rounded-full border px-2.5 py-0.5 text-xs transition-colors cursor-pointer bg-transparent ${
                        isSelected
                          ? "border-border/50 text-muted-foreground/50 cursor-default"
                          : "border-border text-muted-foreground hover:border-foreground/50 hover:text-foreground"
                      }`}
                    >
                      {tag}
                    </button>
                  );
                })}
              </div>
            </div>
          );
        })}
      </div>

      {/* Custom tag input */}
      <Input
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Type a custom tag and press Enter"
        className="text-sm"
      />
    </div>
  );
}
