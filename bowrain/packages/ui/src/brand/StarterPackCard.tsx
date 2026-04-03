import type { StarterPackMeta } from "./data/starter-packs";
import { Badge } from "@neokapi/ui-primitives/components/ui/badge";
import { Card } from "@neokapi/ui-primitives/components/ui/card";
import { Briefcase, Heart, Pencil, Headset, FileCode } from "../components/icons";

const iconMap = {
  Briefcase,
  Heart,
  Pencil,
  Headset,
  FileCode,
} as const;

interface StarterPackCardProps {
  pack: StarterPackMeta;
  onClick: (pack: StarterPackMeta) => void;
}

export function StarterPackCard({ pack, onClick }: StarterPackCardProps) {
  const Icon = iconMap[pack.icon];

  return (
    <Card
      className="group relative cursor-pointer overflow-hidden transition-all hover:shadow-md hover:border-foreground/20"
      onClick={() => onClick(pack)}
    >
      {/* Accent top bar */}
      <div className="h-1" style={{ backgroundColor: pack.accentColor }} />

      <div className="p-5 space-y-3">
        {/* Header: icon + title */}
        <div className="flex items-start gap-3">
          <div
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
            style={{ backgroundColor: pack.accentColor + "22" }}
          >
            <Icon className="h-4.5 w-4.5" style={{ color: pack.accentColor }} />
          </div>
          <div className="min-w-0">
            <h3 className="text-sm font-semibold leading-tight">{pack.label}</h3>
            <p className="text-xs text-muted-foreground mt-0.5">{pack.tagline}</p>
          </div>
        </div>

        {/* Personality tags */}
        <div className="flex flex-wrap gap-1">
          {pack.personalityTags.map((tag) => (
            <Badge key={tag} variant="secondary" className="text-xs px-1.5 py-0">
              {tag}
            </Badge>
          ))}
          <Badge variant="outline" className="text-xs px-1.5 py-0 capitalize">
            {pack.formality}
          </Badge>
        </div>

        {/* Sample text preview */}
        <p className="text-xs italic text-foreground/60 line-clamp-2 leading-relaxed">
          &ldquo;{pack.sampleText}&rdquo;
        </p>
      </div>
    </Card>
  );
}
