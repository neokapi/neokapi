import { useEffect, useMemo, useState } from "react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  cn,
} from "@neokapi/ui-primitives";
import type { CollectionInfo } from "../types/api";
import {
  defaultGroupingAxis,
  groupCollectionsByAxis,
  type CollectionGroup,
} from "./collections/collectionGrouping";
import { Plus, Pencil, Trash2, Plug, ChevronDown, ChevronRight, MoreHorizontal } from "./icons";

export interface CollectionRailProps {
  collections: CollectionInfo[];
  activeCollectionId: string | null;
  onSelectCollection: (id: string) => void;
  onCreateCollection?: () => void;
  onEditCollection?: (collection: CollectionInfo) => void;
  onDeleteCollection?: (id: string) => void;
}

/** The label for the group holding collections that sit at no declared point. */
const UNGROUPED_LABEL = "Ungrouped";

/**
 * CollectionRail is how a project's content is navigated: its collections
 * grouped by where they sit in context space, each row one surface within a
 * group.
 *
 * The arrangement is the project's own coordinate, not a decoration over a flat
 * list. A recipe declares `channel: bowrain/app`, which resolves to
 * `{product: "bowrain", channel: "app"}` and travels to the server; the
 * collection's *name* spells the same thing out ("bowrain-app") and a row of
 * tabs then loses it — two products with six surfaces each read as twelve
 * unrelated tabs, and the strip ran out of room at about six, after which the
 * rest of the project lived behind a horizontal scrollbar.
 *
 * The grouping axis is chosen the way the overview chooses it
 * (`defaultGroupingAxis`), so a workspace that governs by `market` or `tenant`
 * groups by that instead — nothing here knows an axis name.
 */
export function CollectionRail({
  collections,
  activeCollectionId,
  onSelectCollection,
  onCreateCollection,
  onEditCollection,
  onDeleteCollection,
}: CollectionRailProps) {
  // The default collection stands for every point at once, so it is not one of
  // the groups: it sits above them, the way "all" always does.
  const { all, rest } = useMemo(() => split(collections), [collections]);
  const axis = useMemo(() => defaultGroupingAxis(rest), [rest]);
  const groups = useMemo(() => groupCollectionsByAxis(rest, axis), [rest, axis]);
  const itemCount = rest.reduce((n, c) => n + c.item_count, 0);
  const activeGroup = groupValueOf(groups, activeCollectionId);

  // Collapsed groups are the reader's, not the URL's: which product is open is
  // a way of looking, not a place. The group holding the active collection
  // opens whenever the selection moves into it.
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  useEffect(() => {
    if (activeGroup === null) return;
    setCollapsed((prev) => (prev[activeGroup] ? { ...prev, [activeGroup]: false } : prev));
  }, [activeGroup]);

  if (collections.length === 0) return null;

  // One group named by nothing is a flat list — every row would carry the same
  // header, which says less than no header at all.
  const headed = groups.length > 1 || (groups.length === 1 && groups[0].value !== "");

  return (
    <nav
      className="flex flex-col gap-1 text-sm"
      aria-label="Collections"
      data-testid="collection-rail"
    >
      <div className="flex items-baseline justify-between px-2 pb-1">
        <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Collections
        </span>
        <span className="text-xs tabular-nums text-muted-foreground/70">{itemCount}</span>
      </div>

      {all && (
        <CollectionRow
          collection={all}
          label="All items"
          active={all.id === activeCollectionId}
          indented={false}
          onSelect={onSelectCollection}
          onEditCollection={onEditCollection}
          onDeleteCollection={onDeleteCollection}
        />
      )}

      {groups.map((group) => (
        <Group
          key={group.value || " ungrouped"}
          group={group}
          axis={axis}
          headed={headed}
          collapsed={collapsed[group.value] ?? false}
          onToggle={() =>
            setCollapsed((prev) => ({ ...prev, [group.value]: !(prev[group.value] ?? false) }))
          }
          activeCollectionId={activeCollectionId}
          onSelectCollection={onSelectCollection}
          onEditCollection={onEditCollection}
          onDeleteCollection={onDeleteCollection}
        />
      ))}

      {onCreateCollection && (
        <button
          type="button"
          onClick={onCreateCollection}
          className="mt-1 flex items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-sm text-muted-foreground transition-colors hover:bg-accent/40 hover:text-foreground"
          data-testid="create-collection"
        >
          <Plus className="h-3.5 w-3.5" />
          Collection
        </button>
      )}
    </nav>
  );
}

/** The default collection, and the ones that can be grouped. */
function split(collections: CollectionInfo[]): {
  all: CollectionInfo | null;
  rest: CollectionInfo[];
} {
  let all: CollectionInfo | null = null;
  const rest: CollectionInfo[] = [];
  for (const collection of collections) {
    if (collection.is_default) all = collection;
    else rest.push(collection);
  }
  return { all, rest };
}

/**
 * What a collection is called inside its group.
 *
 * Its point already says: grouped by `product`, a collection carrying
 * `{product: "bowrain", channel: "app"}` is "app" — the one coordinate the
 * group header does not already state. With nothing left, or more than one
 * axis left to say, the name is what there is. The name is never *parsed*: a
 * collection called "bowrain-app" that declares no point is not filed under a
 * product it never claimed.
 */
function rowLabel(collection: CollectionInfo, axis?: string): string {
  const rest = Object.entries(collection.coordinates ?? {}).filter(([a]) => a !== axis);
  return rest.length === 1 ? rest[0][1] : collection.name;
}

/** The group value holding a collection, or null when it is in none. */
function groupValueOf(
  groups: CollectionGroup<CollectionInfo>[],
  collectionId: string | null,
): string | null {
  if (!collectionId) return null;
  for (const group of groups) {
    if (group.collections.some((c) => c.id === collectionId)) return group.value;
  }
  return null;
}

function Group({
  group,
  axis,
  headed,
  collapsed,
  onToggle,
  activeCollectionId,
  onSelectCollection,
  onEditCollection,
  onDeleteCollection,
}: {
  group: CollectionGroup<CollectionInfo>;
  axis?: string;
  headed: boolean;
  collapsed: boolean;
  onToggle: () => void;
  activeCollectionId: string | null;
  onSelectCollection: (id: string) => void;
  onEditCollection?: (collection: CollectionInfo) => void;
  onDeleteCollection?: (id: string) => void;
}) {
  const rows = group.collections.map((collection) => (
    <CollectionRow
      key={collection.id}
      collection={collection}
      label={rowLabel(collection, axis)}
      active={collection.id === activeCollectionId}
      indented={headed}
      onSelect={onSelectCollection}
      onEditCollection={onEditCollection}
      onDeleteCollection={onDeleteCollection}
    />
  ));

  if (!headed) return <>{rows}</>;

  const slug = group.value || "ungrouped";
  return (
    <div className="mt-1">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={!collapsed}
        aria-controls={`collection-group-${slug}`}
        className="flex w-full items-center gap-1 rounded-md px-2 py-1 text-left text-xs font-semibold text-muted-foreground transition-colors hover:text-foreground"
        data-testid={`collection-group-${slug}`}
      >
        {collapsed ? (
          <ChevronRight className="h-3 w-3 shrink-0" />
        ) : (
          <ChevronDown className="h-3 w-3 shrink-0" />
        )}
        <span className="truncate" translate="no">
          {group.value || UNGROUPED_LABEL}
        </span>
        <span className="ml-auto font-normal tabular-nums text-muted-foreground/70">
          {group.itemCount}
        </span>
      </button>
      {!collapsed && (
        <div id={`collection-group-${slug}`} className="flex flex-col gap-0.5">
          {rows}
        </div>
      )}
    </div>
  );
}

function CollectionRow({
  collection,
  label,
  active,
  indented,
  onSelect,
  onEditCollection,
  onDeleteCollection,
}: {
  collection: CollectionInfo;
  label: string;
  active: boolean;
  indented: boolean;
  onSelect: (id: string) => void;
  onEditCollection?: (collection: CollectionInfo) => void;
  onDeleteCollection?: (id: string) => void;
}) {
  // A connector-sourced collection is read-only: it has no edit/delete to
  // offer. `editable` folds in the project-level source-connector signal; the
  // kind check is the fallback for older responses.
  const editable = collection.editable ?? collection.kind === "uploaded";
  const actions = !collection.is_default && editable && (onEditCollection || onDeleteCollection);

  return (
    <div className="group/row flex items-center">
      <button
        type="button"
        onClick={() => onSelect(collection.id)}
        aria-current={active ? "true" : undefined}
        title={collection.name}
        className={cn(
          "flex min-w-0 flex-1 items-center gap-1.5 rounded-md py-1.5 pr-2 text-left transition-colors",
          indented ? "pl-6" : "pl-2",
          active
            ? "bg-accent font-medium text-foreground"
            : "text-muted-foreground hover:bg-accent/40 hover:text-foreground",
        )}
        data-testid={`collection-${collection.id}`}
      >
        {collection.kind === "connected" && <Plug className="h-3 w-3 shrink-0 opacity-60" />}
        <span className="truncate" translate="no">
          {label}
        </span>
        <span
          className={cn(
            "ml-auto pl-2 text-xs tabular-nums",
            active ? "text-muted-foreground" : "text-muted-foreground/60",
          )}
        >
          {collection.item_count}
        </span>
      </button>

      {actions && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="rounded-md px-1 py-1.5 text-muted-foreground opacity-0 transition-opacity hover:text-foreground focus-visible:opacity-100 group-hover/row:opacity-100"
              aria-label={`Actions for ${label}`}
              data-testid={`edit-collection-${collection.id}`}
            >
              <MoreHorizontal className="h-3.5 w-3.5" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-[160px]">
            {onEditCollection && (
              <DropdownMenuItem
                onClick={() => onEditCollection(collection)}
                className="flex items-center gap-2 text-sm"
              >
                <Pencil className="h-3.5 w-3.5" /> Edit
              </DropdownMenuItem>
            )}
            {onDeleteCollection && (
              <>
                {onEditCollection && <DropdownMenuSeparator />}
                <DropdownMenuItem
                  onClick={() => onDeleteCollection(collection.id)}
                  className="flex items-center gap-2 text-sm text-destructive"
                >
                  <Trash2 className="h-3.5 w-3.5" /> Delete
                </DropdownMenuItem>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
    </div>
  );
}
