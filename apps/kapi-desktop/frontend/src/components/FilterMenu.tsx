import { useState } from "react";
import { Filter, ChevronDown, Check, Plus, Pencil, Users, Trash2 } from "lucide-react";
import {
  Button,
  Badge,
  Checkbox,
  Label,
  Input,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { KapiProject, ProjectFilter } from "../types/api";
import { useActiveFilter } from "../context/ActiveFilterContext";
import { isEmptyFilter } from "../lib/filter";

/**
 * Menu-bar control for the project's Active Filter: shows the active filter (or
 * "All"), switches between saved filters in one click, and opens an editor to
 * create/edit filters (collections + glob + languages, shared or personal).
 */
export function FilterMenu({ project }: { project: KapiProject }) {
  const { filters, activeId, active, setActive, saveFilter, deleteFilter } = useActiveFilter();
  const [editing, setEditing] = useState<ProjectFilter | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);

  const edit = (f: ProjectFilter) => {
    setMenuOpen(false);
    setEditing(f);
  };

  return (
    <>
      <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className="h-7 gap-1.5"
            aria-label={t("Active filter")}
          >
            <Filter size={13} className={active ? "text-primary" : "text-muted-foreground"} />
            <span className="max-w-[10rem] truncate">{active ? active.name : t("All")}</span>
            <ChevronDown size={12} className="text-muted-foreground" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuLabel>{t("Active filter")}</DropdownMenuLabel>
          <DropdownMenuItem onClick={() => void setActive("")}>
            <Check size={14} className={activeId === "" ? "opacity-100" : "opacity-0"} />
            {t("All")}
          </DropdownMenuItem>
          {filters.map((f) => (
            <DropdownMenuItem key={f.id} onClick={() => void setActive(f.id)}>
              <Check size={14} className={activeId === f.id ? "opacity-100" : "opacity-0"} />
              <span className="flex-1 truncate">{f.name}</span>
              {f.shared && <Users size={11} className="text-muted-foreground" />}
              <span
                role="button"
                tabIndex={0}
                aria-label={t("Edit {name}", { name: f.name })}
                className="ml-1 rounded p-0.5 text-muted-foreground opacity-70 hover:bg-accent hover:text-foreground hover:opacity-100"
                onPointerDown={(e) => e.stopPropagation()}
                onClick={(e) => {
                  e.stopPropagation();
                  e.preventDefault();
                  edit(f);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.stopPropagation();
                    e.preventDefault();
                    edit(f);
                  }
                }}
              >
                <Pencil size={12} />
              </span>
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={() => edit({ id: "", name: "", collections: [], glob: "", languages: [] })}
          >
            <Plus size={14} />
            {t("New filter…")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {editing && (
        <FilterEditorDialog
          project={project}
          filter={editing}
          onClose={() => setEditing(null)}
          onSave={async (f) => {
            const wasNew = !editing.id;
            const wasActive = activeId === editing.id;
            const saved = await saveFilter(f);
            // Activate a freshly created filter (or re-activate the one you were
            // already using); don't hijack the active selection when editing a
            // different saved filter.
            if (saved && (wasNew || wasActive)) await setActive(saved.id);
            setEditing(null);
          }}
          onDelete={
            editing.id
              ? async () => {
                  const id = editing.id;
                  await deleteFilter(id);
                  if (activeId === id) await setActive("");
                  setEditing(null);
                }
              : undefined
          }
        />
      )}
    </>
  );
}

function FilterEditorDialog({
  project,
  filter,
  onClose,
  onSave,
  onDelete,
}: {
  project: KapiProject;
  filter: ProjectFilter;
  onClose: () => void;
  onSave: (f: ProjectFilter) => void;
  onDelete?: () => void;
}) {
  const [name, setName] = useState(filter.name);
  const [collections, setCollections] = useState<string[]>(filter.collections ?? []);
  const [glob, setGlob] = useState(filter.glob ?? "");
  const [languages, setLanguages] = useState<string[]>(filter.languages ?? []);
  const [shared, setShared] = useState(!!filter.shared);

  const collectionNames = (project.content ?? [])
    .map((c) => c.name)
    .filter((n): n is string => !!n);
  const allLanguages = project.defaults?.target_languages ?? [];

  const toggle = (list: string[], set: (v: string[]) => void, value: string) =>
    set(list.includes(value) ? list.filter((x) => x !== value) : [...list, value]);

  const draft: ProjectFilter = {
    ...filter,
    name: name.trim(),
    collections,
    glob: glob.trim(),
    languages,
    shared,
  };
  const narrowsNothing = isEmptyFilter(draft);

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{filter.id ? t("Edit filter") : t("New filter")}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <Label htmlFor="filter-name" className="mb-1 block text-xs text-muted-foreground">
              {t("Name")}
            </Label>
            <Input
              id="filter-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("e.g. DACH launch")}
            />
          </div>

          {collectionNames.length > 0 && (
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">
                {t("Collections")} <span className="opacity-70">({t("empty = all")})</span>
              </Label>
              <div className="flex flex-wrap gap-x-4 gap-y-1.5">
                {collectionNames.map((c) => (
                  <label key={c} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={collections.includes(c)}
                      onCheckedChange={() => toggle(collections, setCollections, c)}
                    />
                    <span translate="no">{c}</span>
                  </label>
                ))}
              </div>
            </div>
          )}

          <div>
            <Label htmlFor="filter-glob" className="mb-1 block text-xs text-muted-foreground">
              {t("File glob")} <span className="opacity-70">({t("within collections")})</span>
            </Label>
            <Input
              id="filter-glob"
              value={glob}
              onChange={(e) => setGlob(e.target.value)}
              placeholder="**/api*.md"
            />
          </div>

          {allLanguages.length > 0 && (
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">
                {t("Languages")} <span className="opacity-70">({t("empty = all")})</span>
              </Label>
              <div className="flex flex-wrap gap-x-4 gap-y-1.5">
                {allLanguages.map((l) => (
                  <label key={l} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={languages.includes(l)}
                      onCheckedChange={() => toggle(languages, setLanguages, l)}
                    />
                    <span translate="no">{l}</span>
                  </label>
                ))}
              </div>
            </div>
          )}

          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={shared} onCheckedChange={() => setShared(!shared)} />
            {t("Save to project")}
            <Badge variant="outline" className="gap-1 font-normal">
              <Users size={10} />
              {t("shared")}
            </Badge>
          </label>
          {narrowsNothing && (
            <p className="text-xs text-muted-foreground">
              {t("This filter narrows nothing yet — it will behave like “All”.")}
            </p>
          )}
        </div>

        <DialogFooter>
          {onDelete && (
            <Button
              variant="outline"
              onClick={onDelete}
              className="mr-auto text-destructive hover:bg-destructive/10"
            >
              <Trash2 size={13} />
              {t("Delete")}
            </Button>
          )}
          <Button variant="outline" onClick={onClose}>
            {t("Cancel")}
          </Button>
          <Button onClick={() => onSave(draft)} disabled={!name.trim()}>
            {t("Save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
