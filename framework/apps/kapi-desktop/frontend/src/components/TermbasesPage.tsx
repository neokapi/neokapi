import { BookOpen, Plus } from "lucide-react";

export function TermbasesPage() {
  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Termbases</h1>
        <button className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90">
          <Plus size={12} />
          New Termbase
        </button>
      </div>
      <div className="rounded-lg border border-dashed border-border p-8 text-center">
        <BookOpen size={24} className="mx-auto mb-2 text-muted-foreground/50" />
        <p className="text-sm text-muted-foreground">
          Manage terminology databases. Import CSV/TBX files, look up terms, and enforce terminology in your translations.
        </p>
      </div>
    </div>
  );
}
