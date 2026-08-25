import { Button, Skeleton } from "@neokapi/ui-primitives";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import { ContextHub } from "../shell/ContextHub";
import { EmptyState } from "../shell/atoms";
import { ChannelProposalsPanel } from "../proposals";
import { ContextOnboarding } from "./ContextOnboarding";
import { ProfileCard } from "./ProfileCard";
import { useContextProfiles } from "./useContextProfiles";
import { useWorkspace } from "../../context/WorkspaceContext";
import { Layers, Sparkles } from "../../components/icons";

export interface ProfilesViewProps {
  /** Opens one profile. */
  onOpenProfile: (slug: string) => void;
  /** Opens the hosted context scan. Omitted when the server runs no scan jobs. */
  onScanVoice?: () => void;
  /** Server origin folded into the onboarding prompt (web shells). */
  serverUrl?: string;
}

/**
 * The Context hub's landing view: one card per governance profile — every point
 * the workspace's content occupies, and what governs each.
 */
export function ProfilesView({ onOpenProfile, onScanVoice, serverUrl }: ProfilesViewProps) {
  const { data, isLoading, error } = useContextProfiles();
  const { activeWorkspace } = useWorkspace();

  if (isLoading) return <ProfilesSkeleton />;

  if (error) {
    return (
      <ContextHub title="Profiles" width="wide">
        <EmptyState
          icon={<Layers />}
          title="Profiles could not be loaded"
          description={error instanceof Error ? error.message : "Try again in a moment."}
        />
      </ContextHub>
    );
  }

  const profiles = data?.profiles ?? [];
  const points = profiles.filter((p) => p.declared || p.is_default);
  const unbound = profiles.filter((p) => !p.declared && !p.is_default);
  const declaredPoints = points.filter((p) => !p.is_default);
  // Coverage is the same list read a third way: the grid shows who holds each
  // point, and this counts the ones nobody does. It is a fact about the
  // workspace, never a gate — the points still ship.
  const uncovered = points.filter((p) => p.custody && !p.custody.covered).length;
  // Nothing has been pushed and nothing governs anything: the front door of the
  // hub is this workspace's first screen, so it offers the ways in rather than
  // the next refinement.
  const nothingYet = profiles.every((p) => !p.declared && !p.voice);

  return (
    <ContextHub
      title="Profiles"
      description="Every point your content sits at, and what governs it."
      width="wide"
      actions={
        onScanVoice && (
          <Button variant="outline" size="sm" onClick={onScanVoice}>
            <Sparkles className="mr-1.5 size-3.5" /> Scan your brand
          </Button>
        )
      }
    >
      <div className="space-y-8">
        {uncovered > 0 && (
          <p className="text-sm text-muted-foreground">
            <Plural count={uncovered}>
              <One>{uncovered} point has nobody holding it.</One>
              <Other>{uncovered} points have nobody holding them.</Other>
            </Plural>{" "}
            Content there still ships; approvals fall back to the workspace owner.
          </p>
        )}

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {points.map((profile) => (
            <ProfileCard
              key={profile.slug}
              profile={profile}
              conceptCount={data?.terms.concept_count}
              onSelect={onOpenProfile}
            />
          ))}
        </div>

        <ChannelProposalsPanel />

        {nothingYet ? (
          <ContextOnboarding
            workspaceName={activeWorkspace?.name}
            serverUrl={serverUrl}
            onScanVoice={onScanVoice}
          />
        ) : (
          declaredPoints.length === 0 && (
            <EmptyState
              icon={<Layers />}
              title="One point so far"
              description="Declare axes under coordinates: in a project's kapi.yaml, give each content collection a context:, then run kapi push. Every point you declare appears here."
            />
          )
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
                <ProfileCard
                  key={profile.slug}
                  profile={profile}
                  conceptCount={data?.terms.concept_count}
                  onSelect={onOpenProfile}
                />
              ))}
            </div>
          </section>
        )}
      </div>
    </ContextHub>
  );
}

/** The loading shape of the profile grid. */
export function ProfilesSkeleton() {
  return (
    <ContextHub title="Profiles" width="wide">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-44 w-full rounded-xl" />
        ))}
      </div>
    </ContextHub>
  );
}
