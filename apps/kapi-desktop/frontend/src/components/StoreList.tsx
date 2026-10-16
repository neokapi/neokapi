// The stores a person can open when no project store is in view: one row per
// store, its name, the file it lives in, its size and when it last changed.
// Terms and Content memory both list their named stores this way.

import {
  SimpleTooltip,
  Skeleton,
  When,
  uiLocaleTag,
  type ResourceInfo,
} from "@neokapi/ui-primitives";

export interface StoreListProps {
  stores: ResourceInfo[];
  loading?: boolean;
  /** The icon each row leads with. */
  icon: React.ReactNode;
  onOpen: (store: ResourceInfo) => void;
  /** Names the list for assistive technology. */
  "aria-label": string;
}

/** The last segment of a path, which is the part a reader recognises. */
function lastSegment(path: string): string {
  const parts = path.split(/[\\/]/).filter(Boolean);
  return parts[parts.length - 1] ?? path;
}

/** A store's size in the reader's own number format. */
function formatSize(bytes: number): string {
  const kb = bytes / 1024;
  const [value, unit] = kb < 1024 ? [kb, "kilobyte"] : [kb / 1024, "megabyte"];
  const options: Intl.NumberFormatOptions = {
    style: "unit",
    unit,
    unitDisplay: "short",
    maximumFractionDigits: 1,
  };
  try {
    return new Intl.NumberFormat(uiLocaleTag(), options).format(value);
  } catch {
    // A UI language the runtime does not know reads the size in English.
    return new Intl.NumberFormat("en", options).format(value);
  }
}

export function StoreList({ stores, loading, icon, onOpen, "aria-label": label }: StoreListProps) {
  if (loading) {
    return (
      <div className="flex flex-col gap-1" aria-busy>
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-10 w-full rounded-md" />
        ))}
      </div>
    );
  }
  if (stores.length === 0) return null;
  return (
    <ul data-slot="store-list" aria-label={label} className="flex flex-col gap-px">
      {stores.map((store) => (
        <li key={store.path}>
          <button
            type="button"
            data-slot="store-row"
            onClick={() => onOpen(store)}
            className="group grid min-h-10 w-full grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-3 rounded-md px-3 py-2 text-left text-sm hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
          >
            <span className="flex justify-center text-muted-foreground group-hover:text-foreground">
              {icon}
            </span>
            <span className="min-w-0 truncate">
              <span className="font-medium" translate="no">
                {store.name}
              </span>
              <SimpleTooltip content={store.path}>
                <span className="ml-2 font-mono text-[11px] text-muted-foreground" translate="no">
                  {lastSegment(store.path)}
                </span>
              </SimpleTooltip>
            </span>
            <span className="flex shrink-0 items-center gap-3 text-[11px] text-muted-foreground tabular-nums">
              {store.size > 0 && <span>{formatSize(store.size)}</span>}
              {store.modified && <When iso={store.modified} relative />}
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}
