import { useState, useCallback, useMemo } from "react";
import type { VoiceProfile } from "./types";
import { BrandProfileCard } from "./BrandProfileCard";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@neokapi/ui-primitives/components/ui/dialog";
import { Plus, Search } from "../components/icons";
import { useSetBreadcrumb } from "../context/BreadcrumbContext";

interface BrandProfileListProps {
  profiles: VoiceProfile[];
  onSelect: (profile: VoiceProfile) => void;
  onCreate: () => void;
  onDelete: (profileId: string) => Promise<void>;
}

export function BrandProfileList({
  profiles,
  onSelect,
  onCreate,
  onDelete,
}: BrandProfileListProps) {
  useSetBreadcrumb(null);
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
        <h1 className="text-lg font-semibold">Brand Voice Profiles</h1>
        <Button size="sm" onClick={onCreate}>
          <Plus className="w-3.5 h-3.5 mr-1.5" /> New Profile
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
        <div className="text-center py-16 space-y-4">
          <p className="text-sm text-muted-foreground">
            No brand voice profiles yet. Create one to define your brand's writing style.
          </p>
          <Button onClick={onCreate}>
            <Plus className="w-3.5 h-3.5 mr-1.5" /> New Profile
          </Button>
        </div>
      ) : filteredProfiles.length === 0 ? (
        <div className="text-center py-12">
          <p className="text-sm text-muted-foreground">
            No profiles matching &ldquo;{searchQuery}&rdquo;
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredProfiles.map((profile) => (
            <BrandProfileCard
              key={profile.id}
              profile={profile}
              onClick={onSelect}
              onDelete={setDeleteTarget}
            />
          ))}
        </div>
      )}

      <Dialog
        open={!!deleteTarget}
        onOpenChange={(open: boolean) => !open && setDeleteTarget(null)}
      >
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
