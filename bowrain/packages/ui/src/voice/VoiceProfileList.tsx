import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
} from "@neokapi/ui-primitives";
import { useState, useCallback, useMemo } from "react";
import type { VoiceProfile } from "./types";
import { VoiceProfileCard } from "./VoiceProfileCard";
import { ContextScanLocalLaneCard } from "../context-scan/ContextScanLocalLaneCard";
import { ListCapRow } from "../components/ListCapRow";
import { Plus, Search, Sparkles } from "../components/icons";

/** Hard render cap for the profile grid. */
const MAX_PROFILE_CARDS = 100;

interface VoiceProfileListProps {
  profiles: VoiceProfile[];
  onSelect: (profile: VoiceProfile) => void;
  onCreate: () => void;
  onDelete: (profileId: string) => Promise<void>;
  /** Opens the correction-review surface for a profile (correction-learning loop). */
  onReview?: (profileId: string) => void;
  /** Opens the AI context scan (epic 016). Renders the empty-state CTA when set. */
  onScanVoice?: () => void;
  /** Opens the local-lane guidance (kapi Agent Skill). */
  onLocalLane?: () => void;
}

export function VoiceProfileList({
  profiles,
  onSelect,
  onCreate,
  onDelete,
  onReview,
  onScanVoice,
  onLocalLane,
}: VoiceProfileListProps) {
  const [deleteTarget, setDeleteTarget] = useState<VoiceProfile | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");

  const filteredProfiles = useMemo(() => {
    if (!searchQuery.trim()) return profiles;
    const q = searchQuery.toLowerCase();
    return profiles.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        (p.description && p.description.toLowerCase().includes(q)),
    );
  }, [profiles, searchQuery]);

  const handleConfirmDelete = useCallback(async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await onDelete(deleteTarget.id);
    } finally {
      setDeleting(false);
      setDeleteTarget(null);
    }
  }, [deleteTarget, onDelete]);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Voice profiles</h1>
        <Button size="sm" onClick={onCreate}>
          <Plus className="w-3.5 h-3.5 mr-1.5" /> New profile
        </Button>
      </div>

      {/* Search — only shown when there are profiles to search through */}
      {profiles.length > 0 && (
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground pointer-events-none" />
          <Input
            value={searchQuery}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearchQuery(e.target.value)}
            placeholder="Search profiles..."
            className="pl-9 h-9 text-sm"
          />
        </div>
      )}

      {profiles.length === 0 ? (
        <div className="py-12 flex flex-col items-center space-y-6">
          <div className="text-center space-y-4 max-w-md">
            <p className="text-sm text-muted-foreground">
              {onScanVoice
                ? "No voice profiles yet. A profile binds one under profiles[].voice: in a recipe. Scan your existing material to draft one, or define it by hand."
                : "No voice profiles yet. A profile binds one under profiles[].voice: in a recipe. Define one by hand, or draft one locally with the kapi Agent Skill."}
            </p>
            <div className="flex items-center justify-center gap-3">
              {onScanVoice && (
                <Button onClick={onScanVoice}>
                  <Sparkles className="w-3.5 h-3.5 mr-1.5" /> Scan your brand
                </Button>
              )}
              <Button variant={onScanVoice ? "outline" : "default"} onClick={onCreate}>
                <Plus className="w-3.5 h-3.5 mr-1.5" /> New profile
              </Button>
            </div>
            {onScanVoice && (
              <p className="text-xs text-muted-foreground">
                Paste text, add links, or drop your marketing files. The scan drafts a voice profile
                and candidate terms for your review.
              </p>
            )}
          </div>
          <ContextScanLocalLaneCard onLearnMore={onLocalLane} className="w-full max-w-md" />
        </div>
      ) : filteredProfiles.length === 0 ? (
        <div className="text-center py-12">
          <p className="text-sm text-muted-foreground">
            No profiles matching &ldquo;{searchQuery}&rdquo;
          </p>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {filteredProfiles.slice(0, MAX_PROFILE_CARDS).map((profile) => (
              <VoiceProfileCard
                key={profile.id}
                profile={profile}
                onClick={onSelect}
                onReview={onReview ? (p) => onReview(p.id) : undefined}
                onDelete={setDeleteTarget}
              />
            ))}
          </div>
          <ListCapRow
            shown={Math.min(filteredProfiles.length, MAX_PROFILE_CARDS)}
            total={filteredProfiles.length}
            noun="profiles"
            hint="Refine the search to narrow the list."
            className="border border-border rounded-md"
          />
        </>
      )}

      <Dialog
        open={!!deleteTarget}
        onOpenChange={(open: boolean) => !open && setDeleteTarget(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete profile</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            Are you sure you want to delete &ldquo;{deleteTarget?.name}&rdquo;? This action cannot
            be undone.
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)} disabled={deleting}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleConfirmDelete} disabled={deleting}>
              {deleting ? "Deleting..." : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
