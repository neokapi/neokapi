import { Button, Skeleton } from "@neokapi/ui-primitives";
import { BrandHub } from "../shell/BrandHub";
import { EmptyState } from "../shell/atoms";
import { ProfileCard } from "./ProfileCard";
import { useContextProfiles } from "./useContextProfiles";
import { Layers, Sparkles } from "../../components/icons";

export interface ProfilesViewProps {
  /** Opens one profile. */
  onOpenProfile: (slug: string) => void;
  /** Opens the hosted brand scan. Omitted when the server runs no scan jobs. */
  onScanBrand?: () => void;
}

/**
 * The Context hub's landing view: one card per governance profile — every point
 * the workspace's content occupies, and what governs each.
 */
export function ProfilesView({ onOpenProfile, onScanBrand }: ProfilesViewProps) {
  const { data, isLoading, error } = useContextProfiles();

  if (isLoading) return <ProfilesSkeleton />;

  if (error) {
    return (
      <BrandHub title="Profiles" width="wide">
        <EmptyState
          icon={<Layers />}
          title="Profiles could not be loaded"
          description={error instanceof Error ? error.message : "Try again in a moment."}
        />
      </BrandHub>
    );
  }

  const profiles = data?.profiles ?? [];
  const points = profiles.filter((p) => p.declared || p.is_default);
  const unbound = profiles.filter((p) => !p.declared && !p.is_default);
  const declaredPoints = points.filter((p) => !p.is_default);

  return (
    <BrandHub
      title="Profiles"
      description="Every point your content sits at, and what governs it."
      width="wide"
      actions={
        onScanBrand && (
          <Button variant="outline" size="sm" onClick={onScanBrand}>
            <Sparkles className="mr-1.5 size-3.5" /> Scan your brand
          </Button>
        )
      }
    >
      <div className="space-y-8">
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {points.map((profile) => (
            <ProfileCard key={profile.slug} profile={profile} onSelect={onOpenProfile} />
          ))}
        </div>

        {declaredPoints.length === 0 && (
          <EmptyState
            icon={<Layers />}
            title="One point so far"
            description="Declare axes under coordinates: in a project's kapi.yaml, give each content collection a context:, then run kapi push. Every point you declare appears here."
          />
        )}

        {unbound.length > 0 && (
          <section className="space-y-3">
            <div className="space-y-1">
              <h2 className="text-sm font-medium text-foreground">Voices with no point</h2>
              <p className="text-sm text-muted-foreground">
                These voices exist in the workspace and govern nothing. Bind one under
                profiles[].voice: in a recipe, or set it as the workspace default.
              </p>
            </div>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
              {unbound.map((profile) => (
                <ProfileCard key={profile.slug} profile={profile} onSelect={onOpenProfile} />
              ))}
            </div>
          </section>
        )}
      </div>
    </BrandHub>
  );
}

/** The loading shape of the profile grid. */
export function ProfilesSkeleton() {
  return (
    <BrandHub title="Profiles" width="wide">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-44 w-full rounded-xl" />
        ))}
      </div>
    </BrandHub>
  );
}
