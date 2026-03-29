import { Database, Plus } from "lucide-react";

export function MemoriesPage() {
  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Translation Memories</h1>
        <button className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90">
          <Plus size={12} />
          New TM
        </button>
      </div>
      <div className="rounded-lg border border-dashed border-border p-8 text-center">
        <Database size={24} className="mx-auto mb-2 text-muted-foreground/50" />
        <p className="text-sm text-muted-foreground">
          Manage translation memories. Import TMX files, leverage existing translations, and build TMs from your projects.
        </p>
      </div>
    </div>
  );
}
