import { useState, useCallback } from "react";
import type { VoiceProfile } from "./types";
import { BrandProfileCard } from "./BrandProfileCard";
import { Button } from "../components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "../components/ui/dialog";
import { Plus, Sparkles } from "../components/icons";
import { useSetBreadcrumb } from "../context/BreadcrumbContext";

interface BrandProfileListProps {
  profiles: VoiceProfile[];
  onSelect: (profile: VoiceProfile) => void;
  onCreate: () => void;
  onCreateFromStarter: () => void;
  onDelete: (profileId: string) => Promise<void>;
}

export function BrandProfileList({
  profiles,
  onSelect,
  onCreate,
  onCreateFromStarter,
  onDelete,
}: BrandProfileListProps) {
  useSetBreadcrumb(null);
  const [deleteTarget, setDeleteTarget] = useState<VoiceProfile | null>(null);
  const [deleting, setDeleting] = useState(false);

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
    <div className="max-w-5xl mx-auto px-4 py-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Brand Voice Profiles</h1>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={onCreateFromStarter}>
            <Sparkles className="w-3.5 h-3.5 mr-1.5" /> From Starter
          </Button>
          <Button size="sm" onClick={onCreate}>
            <Plus className="w-3.5 h-3.5 mr-1.5" /> New Profile
          </Button>
        </div>
      </div>

      {profiles.length === 0 ? (
        <div className="text-center py-16 space-y-4">
          <p className="text-sm text-muted-foreground">
            No brand voice profiles yet. Create one to define your brand's writing style.
          </p>
          <div className="flex gap-3 justify-center">
            <Button variant="outline" onClick={onCreateFromStarter}>
              <Sparkles className="w-3.5 h-3.5 mr-1.5" /> Start from a template
            </Button>
            <Button onClick={onCreate}>
              <Plus className="w-3.5 h-3.5 mr-1.5" /> Create from scratch
            </Button>
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {profiles.map((profile) => (
            <BrandProfileCard
              key={profile.id}
              profile={profile}
              onClick={onSelect}
              onDelete={setDeleteTarget}
            />
          ))}
        </div>
      )}

      <Dialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Profile</DialogTitle>
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
