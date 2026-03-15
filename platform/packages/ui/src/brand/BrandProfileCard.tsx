import type { VoiceProfile } from "./types";
import { Card } from "../components/ui/card";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Trash2 } from "../components/icons";

interface BrandProfileCardProps {
  profile: VoiceProfile;
  onClick: (profile: VoiceProfile) => void;
  onDelete?: (profile: VoiceProfile) => void;
}

export function BrandProfileCard({ profile, onClick, onDelete }: BrandProfileCardProps) {
  return (
    <Card
      className="cursor-pointer hover:ring-1 hover:ring-primary/30 transition-all"
      onClick={() => onClick(profile)}
    >
      <div className="p-5 space-y-3">
        <div className="flex items-start justify-between">
          <div className="flex-1 min-w-0">
            <h3 className="text-sm font-semibold truncate">{profile.name}</h3>
            {profile.description && (
              <p className="text-xs text-muted-foreground mt-1 line-clamp-2">
                {profile.description}
              </p>
            )}
          </div>
          {onDelete && (
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 shrink-0 text-muted-foreground hover:text-destructive"
              onClick={(e: React.MouseEvent) => {
                e.stopPropagation();
                onDelete(profile);
              }}
            >
              <Trash2 className="w-3.5 h-3.5" />
            </Button>
          )}
        </div>

        <div className="flex flex-wrap gap-1">
          {profile.tone.personality.slice(0, 3).map((tag) => (
            <Badge key={tag} variant="secondary" className="text-[10px]">
              {tag}
            </Badge>
          ))}
          <Badge variant="outline" className="text-[10px]">
            {profile.tone.formality}
          </Badge>
        </div>

        <div className="flex items-center justify-between text-[10px] text-muted-foreground">
          <span>v{profile.version}</span>
          <span>
            {profile.vocabulary.preferred_terms?.length ?? 0} preferred,{" "}
            {profile.vocabulary.forbidden_terms?.length ?? 0} forbidden
          </span>
        </div>
      </div>
    </Card>
  );
}
