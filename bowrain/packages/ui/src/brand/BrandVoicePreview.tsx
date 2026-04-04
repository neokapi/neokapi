import { Badge } from "@neokapi/ui-primitives";
import type { ToneProfile, StyleRules } from "./types";
import { getVoicePreview } from "./data/voice-previews";

interface BrandVoicePreviewProps {
  tone: ToneProfile;
  style: StyleRules;
}

export function BrandVoicePreview({ tone, style }: BrandVoicePreviewProps) {
  const previewText = getVoicePreview(tone.formality, tone.emotion);

  return (
    <div className="space-y-4 rounded-lg border border-border/50 bg-muted/20 p-4">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">Voice Preview</h3>
        <p className="text-xs text-muted-foreground">This is what your brand voice sounds like</p>
      </div>

      {/* Sample text */}
      <div className="rounded-md bg-background p-3">
        <p className="text-sm leading-relaxed">{previewText}</p>
      </div>

      {/* Current settings summary */}
      <div className="space-y-2">
        {tone.personality.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {tone.personality.map((tag) => (
              <Badge key={tag} variant="secondary" className="text-xs">
                {tag}
              </Badge>
            ))}
          </div>
        )}

        <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
          <div className="text-muted-foreground">Formality</div>
          <div className="capitalize">{tone.formality}</div>

          <div className="text-muted-foreground">Emotion</div>
          <div className="capitalize">{tone.emotion}</div>

          <div className="text-muted-foreground">Humor</div>
          <div className="capitalize">{tone.humor}</div>

          <div className="text-muted-foreground">Point of View</div>
          <div>
            {style.person_pov === "first_plural"
              ? "We"
              : style.person_pov === "second"
                ? "You"
                : "They"}
          </div>

          <div className="text-muted-foreground">Contractions</div>
          <div className="capitalize">{style.contractions}</div>

          <div className="text-muted-foreground">Active Voice</div>
          <div>{style.active_voice ? "Yes" : "No"}</div>
        </div>
      </div>
    </div>
  );
}
