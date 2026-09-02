import type { ReactNode } from "react";
import { One, Other, Plural, t } from "@neokapi/i18n-react/runtime";
import { Badge, Button, Card, Skeleton, cn } from "@neokapi/ui-primitives";
import { ContextHub } from "../shell/ContextHub";
import { EmptyState, formatRelative } from "../shell/atoms";
import { CoordinateReadout } from "./Coordinates";
import { useContextProfile } from "./useContextProfiles";
import type { ContextProfile, ContextProfileVoice } from "../../types/context-profiles";
import {
  BookOpen,
  FlaskConical,
  Folder,
  Layers,
  Palette,
  ShieldCheck,
  Sparkles,
} from "../../components/icons";

export interface ProfileDetailViewProps {
  slug: string;
  onBack: () => void;
  /** Opens the bound voice in the voice editor. */
  onOpenVoice: (profileId: string) => void;
  /** Opens the workspace vocabulary. */
  onOpenTerms: () => void;
  /** Opens the change-set queue. */
  onOpenChanges: () => void;
  /** Opens the hosted context scan. Omitted when the server runs no scan jobs. */
  onScanVoice?: () => void;
}

/**
 * One governance profile: its coordinates, the voice bound to it, the vocabulary
 * in force, the content that sits there, and the changes waiting on it.
 */
export function ProfileDetailView({
  slug,
  onBack,
  onOpenVoice,
  onOpenTerms,
  onOpenChanges,
  onScanVoice,
}: ProfileDetailViewProps) {
  const { data, profile, isLoading, error } = useContextProfile(slug);

  if (isLoading) return <ProfileDetailSkeleton />;

  if (error || !profile) {
    return (
      <ContextHub title="Profile" width="wide">
        <EmptyState
          icon={<Layers />}
          title={error ? "This profile could not be loaded" : "No such profile"}
          description={
            error instanceof Error
              ? error.message
              : "A profile exists for as long as content sits at its point. Nothing sits here."
          }
          action={
            <Button variant="outline" onClick={onBack}>
              Back to profiles
            </Button>
          }
        />
      </ContextHub>
    );
  }

  return (
    <ContextHub
      title={profile.label}
      description={profileSubtitle(profile)}
      width="wide"
      actions={
        <div className="flex items-center gap-1.5">
          {profile.is_default && <Badge variant="outline">Default</Badge>}
          {!profile.declared && !profile.is_default && (
            <Badge variant="outline">Bound to no point</Badge>
          )}
        </div>
      }
    >
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Section icon={<Layers />} title="Coordinates">
          {profile.coordinates && Object.keys(profile.coordinates).length > 0 ? (
            <div className="space-y-3">
              <CoordinateReadout coordinates={profile.coordinates} />
              {profile.name && (
                <p className="text-xs text-muted-foreground">
                  Overrides for this point live in{" "}
                  <code className="font-mono">.kapi/profiles/{profile.name}/</code>.
                </p>
              )}
              {profile.channel && (
                <p className="text-xs text-muted-foreground">
                  The channel <code className="font-mono">{profile.channel}</code> selects the
                  matching override inside the voice.
                </p>
              )}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              {profile.is_default
                ? "No coordinates. This is where content that declares none sits, and its files are the flat ones in .kapi/."
                : "This voice sits at no point. Bind it under profiles[].voice: in a recipe to give it one."}
            </p>
          )}
        </Section>

        <Section
          icon={<Palette />}
          title="Voice"
          action={
            profile.voice && (
              <Button variant="ghost" size="sm" onClick={() => onOpenVoice(profile.voice!.id)}>
                Open
              </Button>
            )
          }
        >
          {profile.voice ? (
            <VoiceSummary voice={profile.voice} />
          ) : (
            <p className="text-sm text-muted-foreground">
              No voice bound. Name one under profiles[].voice: in the recipe that declares this
              point, or give the project a default voice.
            </p>
          )}
        </Section>

        <Section
          icon={<BookOpen />}
          title="Terms"
          action={
            <Button variant="ghost" size="sm" onClick={onOpenTerms}>
              Open
            </Button>
          }
        >
          <div className="space-y-3 text-sm">
            <Stat
              label="Workspace vocabulary"
              value={`${data?.terms.concept_count ?? 0} concepts`}
              hint="Governed per workspace and scoped by market, not by coordinates, so every profile shares this base."
            />
            {profile.voice ? (
              <Stat
                label="This profile's overrides"
                value={`${
                  profile.voice.preferred_terms +
                  profile.voice.forbidden_terms +
                  profile.voice.competitor_terms
                } rules`}
                hint={`${profile.voice.preferred_terms} preferred · ${profile.voice.forbidden_terms} forbidden · ${profile.voice.competitor_terms} competitor`}
              />
            ) : (
              <p className="text-muted-foreground">
                No overrides: a profile narrows the vocabulary through its voice, and none is bound.
              </p>
            )}
          </div>
        </Section>

        <Section icon={<ShieldCheck />} title="Standing">
          {profile.checks ? (
            <div className="space-y-3 text-sm">
              <Stat
                label="Voice score"
                value={`${profile.checks.score}`}
                hint={t(
                  "{count, plural, one {Across # checked block, last {when}.} other {Across # checked blocks, last {when}.}}",
                  {
                    count: profile.checks.scored_blocks,
                    when: formatRelative(profile.checks.last_checked_at),
                  },
                )}
              />
              <Stat
                label="Open findings"
                value={`${profile.checks.findings}`}
                hint="What the checks raised on the content this voice governs."
              />
              <p className="text-xs text-muted-foreground">
                Scoped by the voice, not the point: a stored check records the voice it resolved
                through, so two points sharing one voice share this standing.
              </p>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              Nothing here has been checked. A voice check runs over a collection&rsquo;s blocks and
              stores what it scored, and that is what this reports.
            </p>
          )}
        </Section>

        <Section
          icon={<FlaskConical />}
          title="Changes"
          action={
            <Button variant="ghost" size="sm" onClick={onOpenChanges}>
              Open
            </Button>
          }
        >
          {profile.pending_changes > 0 ? (
            <p className="text-sm text-foreground">
              <Plural count={profile.pending_changes}>
                <One>
                  {profile.pending_changes} change-set is in review carrying a voice rule for this
                  profile.
                </One>
                <Other>
                  {profile.pending_changes} change-sets are in review carrying a voice rule for this
                  profile.
                </Other>
              </Plural>
            </p>
          ) : (
            <p className="text-sm text-muted-foreground">
              Nothing in review for this profile. A governed edit to its voice, such as banning a
              term or changing a preferred one, arrives here as a change-set.
            </p>
          )}
        </Section>

        <Section icon={<Folder />} title="Content" className="lg:col-span-2">
          {profile.collections.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No collection sits here yet. A collection arrives when a project declares it under
              content: with this point&rsquo;s context: and runs kapi push.
            </p>
          ) : (
            <CollectionTable profile={profile} />
          )}
        </Section>

        {profile.is_default && (
          <Section icon={<Sparkles />} title="Context scan" className="lg:col-span-2">
            <div className="space-y-3">
              {data?.scan ? (
                <p className="text-sm text-foreground">
                  Last scan {data.scan.status}
                  {data.scan.status === "processing" && ` (${data.scan.progress}%)`} ·{" "}
                  <span className="text-muted-foreground">
                    {formatRelative(data.scan.updated_at)}
                  </span>
                </p>
              ) : (
                <p className="text-sm text-muted-foreground">
                  No scan yet. A scan reads your existing material and drafts a voice and candidate
                  terms for review.
                </p>
              )}
              <p className="text-xs text-muted-foreground">
                A scan covers the whole workspace: it carries no coordinates, so it is reported here
                rather than split across points it never distinguished.
              </p>
              {onScanVoice && (
                <Button variant="outline" size="sm" onClick={onScanVoice}>
                  <Sparkles className="mr-1.5 size-3.5" /> Scan your brand
                </Button>
              )}
            </div>
          </Section>
        )}
      </div>
    </ContextHub>
  );
}

function profileSubtitle(profile: ContextProfile): string {
  if (profile.is_default) return "The workspace's own point.";
  if (!profile.declared) return "A voice the workspace holds, bound to no point.";
  const count = profile.collections.length;
  // A plain string, so the plural is ICU inside t() rather than <Plural>. "sit"
  // agrees with the count too — the one-collection form is "sits".
  return t("{count, plural, one {# collection sits here.} other {# collections sit here.}}", {
    count,
  });
}

function Section({
  icon,
  title,
  action,
  children,
  className,
}: {
  icon: ReactNode;
  title: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={cn("space-y-4 p-5", className)}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground [&_svg]:size-4">{icon}</span>
          <h2 className="text-sm font-medium text-foreground">{title}</h2>
        </div>
        {action}
      </div>
      {children}
    </Card>
  );
}

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="space-y-0.5">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium text-foreground">{value}</span>
      </div>
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

function VoiceSummary({ voice }: { voice: ContextProfileVoice }) {
  return (
    <div className="space-y-3">
      <div className="flex items-baseline gap-2">
        <span className="text-base font-medium text-foreground">{voice.name}</span>
        <span className="font-mono text-xs text-muted-foreground">v{voice.version}</span>
      </div>
      {voice.description && <p className="text-sm text-muted-foreground">{voice.description}</p>}
      <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
        <VoiceStat label="Vocabulary rules" value={voice.preferred_terms + voice.forbidden_terms} />
        <VoiceStat label="Pattern rules" value={voice.pattern_rules} />
        <VoiceStat label="Locale overrides" value={voice.locale_overrides} />
        <VoiceStat label="Channel overrides" value={voice.channel_overrides} />
      </dl>
      <p className="text-xs text-muted-foreground">Updated {formatRelative(voice.updated_at)}.</p>
    </div>
  );
}

function VoiceStat({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-baseline justify-between gap-2">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="font-medium text-foreground">{value}</dd>
    </div>
  );
}

function CollectionTable({ profile }: { profile: ContextProfile }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-md text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs uppercase tracking-wider text-muted-foreground">
            <th className="pb-2 pr-4 font-medium">Collection</th>
            <th className="pb-2 pr-4 font-medium">Project</th>
            <th className="pb-2 pr-4 font-medium">Stream</th>
            <th className="pb-2 font-medium">Declared by</th>
          </tr>
        </thead>
        <tbody>
          {profile.collections.map((c) => (
            <tr key={`${c.project_id}:${c.id}`} className="border-b border-border/50 last:border-0">
              <td className="py-2 pr-4 font-medium text-foreground">{c.name}</td>
              <td className="py-2 pr-4 text-muted-foreground">{c.project_name}</td>
              <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">
                {c.stream || "·"}
              </td>
              <td className="py-2">
                <Badge variant="outline" className="font-normal">
                  {c.owner === "recipe" ? "kapi.yaml" : "the hub"}
                </Badge>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** The loading shape of the profile detail. */
export function ProfileDetailSkeleton() {
  return (
    <ContextHub title="Profile" width="wide">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-40 w-full rounded-xl" />
        ))}
      </div>
    </ContextHub>
  );
}
