import { Badge, Card, cn } from "@neokapi/ui-primitives";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import type { ContextProfile, ContextProfileCustodian } from "../../types/context-profiles";
import { CoordinateReadout } from "./Coordinates";
import { BookOpen, Folder, Palette, ShieldCheck, Users } from "../../components/icons";

/** How many collections a card names before it counts the rest. */
const NAMED_COLLECTIONS = 3;

/** How many custodians a card names before it counts the rest. */
const NAMED_CUSTODIANS = 2;

/** Names the first few custodians, falling back to the id when we hold no name. */
function custodianNames(custodians: ContextProfileCustodian[]): string {
  return custodians
    .slice(0, NAMED_CUSTODIANS)
    .map((c) => c.name || c.email || c.user_id)
    .join(", ");
}

export interface ProfileCardProps {
  profile: ContextProfile;
  /** The workspace vocabulary every profile shares, from the same aggregation. */
  conceptCount?: number;
  onSelect: (slug: string) => void;
  className?: string;
}

/**
 * One governance profile: the point, the voice governing it, and the content
 * that sits there.
 */
export function ProfileCard({ profile, conceptCount, onSelect, className }: ProfileCardProps) {
  const projects = new Set(profile.collections.map((c) => c.project_name));
  const named = profile.collections.slice(0, NAMED_COLLECTIONS);
  const rest = profile.collections.length - named.length;
  const unbound = !profile.declared && !profile.is_default;
  const voiceRules = profile.voice
    ? profile.voice.preferred_terms + profile.voice.forbidden_terms + profile.voice.competitor_terms
    : 0;

  return (
    <Card
      role="button"
      tabIndex={0}
      onClick={() => onSelect(profile.slug)}
      onKeyDown={(e: React.KeyboardEvent) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect(profile.slug);
        }
      }}
      className={cn(
        "flex cursor-pointer flex-col gap-4 p-5 transition-colors",
        "hover:border-primary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className,
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h3 className="truncate text-base font-semibold tracking-tight text-foreground">
            {profile.label}
          </h3>
          {profile.is_default && (
            <p className="text-xs text-muted-foreground">
              Where content that declares no coordinates sits.
            </p>
          )}
          {unbound && <p className="text-xs text-muted-foreground">Bound to no point.</p>}
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {profile.is_default && <Badge variant="outline">Default</Badge>}
          {profile.pending_changes > 0 && (
            <Badge className="border-transparent bg-warning/15 text-warning">
              {profile.pending_changes} in review
            </Badge>
          )}
        </div>
      </div>

      <CoordinateReadout coordinates={profile.coordinates} />

      <div className="mt-auto space-y-2 border-t border-border pt-3 text-sm">
        <div className="flex items-center gap-2">
          <Palette className="size-3.5 shrink-0 text-muted-foreground" />
          {profile.voice ? (
            <span className="truncate text-foreground">
              {profile.voice.name}
              <span className="ml-1.5 font-mono text-xs text-muted-foreground">
                v{profile.voice.version}
              </span>
            </span>
          ) : (
            <span className="text-muted-foreground">No voice bound</span>
          )}
        </div>

        <div className="flex items-center gap-2">
          <BookOpen className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="truncate text-muted-foreground">
            <Plural count={conceptCount ?? 0}>
              <One>{conceptCount ?? 0} concept</One>
              <Other>{conceptCount ?? 0} concepts</Other>
            </Plural>
            {voiceRules > 0 && (
              <span className="opacity-60">
                {" · "}
                <Plural count={voiceRules}>
                  <One>{voiceRules} rule here</One>
                  <Other>{voiceRules} rules here</Other>
                </Plural>
              </span>
            )}
          </span>
        </div>

        <div className="flex items-center gap-2">
          <ShieldCheck className="size-3.5 shrink-0 text-muted-foreground" />
          {profile.checks ? (
            <span className="truncate text-muted-foreground">
              <span className="font-medium text-foreground">{profile.checks.score}</span> from{" "}
              <Plural count={profile.checks.scored_blocks}>
                <One>{profile.checks.scored_blocks} checked block</One>
                <Other>{profile.checks.scored_blocks} checked blocks</Other>
              </Plural>
              {profile.checks.findings > 0 && (
                <span className="opacity-60">
                  {" · "}
                  <Plural count={profile.checks.findings}>
                    <One>{profile.checks.findings} finding</One>
                    <Other>{profile.checks.findings} findings</Other>
                  </Plural>
                </span>
              )}
            </span>
          ) : (
            <span className="text-muted-foreground">Not checked yet</span>
          )}
        </div>

        {profile.custody && (
          <div className="flex items-center gap-2">
            <Users className="size-3.5 shrink-0 text-muted-foreground" />
            {profile.custody.covered ? (
              <span className="truncate text-muted-foreground">
                <span className="font-medium text-foreground">
                  {custodianNames(profile.custody.custodians)}
                </span>
                {profile.custody.custodians.length > NAMED_CUSTODIANS && (
                  <span className="opacity-60">
                    {` +${profile.custody.custodians.length - NAMED_CUSTODIANS}`}
                  </span>
                )}
              </span>
            ) : (
              // Reported, never blocked: an ungoverned point is an org-chart gap
              // rather than a content defect, so this reads as a fact about the
              // workspace and not as a failure.
              <span className="truncate text-muted-foreground">
                Nobody holds this point
                {(profile.custody.fallback?.length ?? 0) > 0 && (
                  <span className="opacity-60"> · falls back to the workspace owner</span>
                )}
              </span>
            )}
          </div>
        )}

        <div className="flex items-center gap-2">
          <Folder className="size-3.5 shrink-0 text-muted-foreground" />
          {profile.collections.length === 0 ? (
            <span className="text-muted-foreground">
              {unbound ? "Governs nothing" : "No content here yet"}
            </span>
          ) : (
            <span className="truncate text-muted-foreground">
              {named.map((c) => c.name).join(", ")}
              {rest > 0 && ` +${rest}`}
              <span className="opacity-60">
                {" · "}
                <Plural count={projects.size}>
                  <One>{projects.size} project</One>
                  <Other>{projects.size} projects</Other>
                </Plural>
              </span>
            </span>
          )}
        </div>
      </div>
    </Card>
  );
}
