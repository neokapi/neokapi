import type { CollectionInfo } from "../types/api";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "./ui/dropdown-menu";
import { Plus, Pencil, Trash2, Plug } from "./icons";

export interface CollectionTabsProps {
  collections: CollectionInfo[];
  activeCollectionId: string | null;
  onSelectCollection: (id: string) => void;
  onCreateCollection?: () => void;
  onEditCollection?: (collection: CollectionInfo) => void;
  onDeleteCollection?: (id: string) => void;
}

export function CollectionTabs({
  collections,
  activeCollectionId,
  onSelectCollection,
  onCreateCollection,
  onEditCollection,
  onDeleteCollection,
}: CollectionTabsProps) {
  if (collections.length <= 1 && !onCreateCollection) return null;

  const activeId = activeCollectionId ?? collections[0]?.id;

  return (
    <div className="flex items-center gap-1 mb-4 overflow-x-auto pb-px">
      <div className="flex items-center gap-0.5 rounded-lg bg-accent/30 p-0.5">
        {collections.map((coll) => {
          const isActive = coll.id === activeId;
          const showActions = isActive && !coll.is_default;
          return (
            <div key={coll.id} className="flex items-center">
              <button
                onClick={() => onSelectCollection(coll.id)}
                className={`
                  flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium
                  transition-all duration-150 cursor-pointer border-none
                  ${showActions ? "rounded-l-md" : "rounded-md"}
                  ${
                    isActive
                      ? "bg-background text-foreground shadow-sm"
                      : "bg-transparent text-muted-foreground hover:text-foreground hover:bg-background"
                  }
                `}
              >
                {coll.kind === "connected" && <Plug className="w-3 h-3 shrink-0 opacity-60" />}
                <span className="truncate max-w-[140px]">
                  {coll.is_default ? "All Items" : coll.name}
                </span>
                <span
                  className={`
                    text-[10px] tabular-nums ml-0.5
                    ${isActive ? "text-muted-foreground" : "text-muted-foreground/60"}
                  `}
                >
                  {coll.item_count}
                </span>
              </button>

              {/* Edit/actions button — visible on active non-default tabs */}
              {showActions && (onEditCollection || onDeleteCollection) && (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button
                      className="
                        px-1.5 py-1.5 rounded-r-md bg-background shadow-sm
                        text-muted-foreground hover:text-foreground
                        transition-colors cursor-pointer border-none
                        border-l border-border/30
                      "
                      data-testid={`edit-collection-${coll.id}`}
                    >
                      <Pencil className="w-3 h-3" />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" className="w-[160px]">
                    {onEditCollection && (
                      <DropdownMenuItem
                        onClick={() => onEditCollection(coll)}
                        className="flex items-center gap-2 text-sm"
                      >
                        <Pencil className="w-3.5 h-3.5" /> Edit
                      </DropdownMenuItem>
                    )}
                    {onDeleteCollection && (
                      <>
                        {onEditCollection && <DropdownMenuSeparator />}
                        <DropdownMenuItem
                          onClick={() => onDeleteCollection(coll.id)}
                          className="flex items-center gap-2 text-sm text-destructive"
                        >
                          <Trash2 className="w-3.5 h-3.5" /> Delete
                        </DropdownMenuItem>
                      </>
                    )}
                  </DropdownMenuContent>
                </DropdownMenu>
              )}
            </div>
          );
        })}
      </div>

      {onCreateCollection && (
        <button
          onClick={onCreateCollection}
          className="
            flex items-center gap-1 px-2 py-1.5 rounded-md text-sm
            text-muted-foreground hover:text-foreground hover:bg-accent/40
            transition-colors cursor-pointer bg-transparent border-none shrink-0
          "
        >
          <Plus className="w-3.5 h-3.5" />
          <span className="hidden sm:inline">Collection</span>
        </button>
      )}
    </div>
  );
}
