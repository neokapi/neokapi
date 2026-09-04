// Where a collection's content sits: the channel it names, and the declared
// axes it moves off the project's default point.
//
// A collection names ONE channel reference and the structural axes follow from
// it, so this offers the declared references rather than a free-text field that
// can name a channel no profile declares. The axes are the shared
// CoordinatesEditor over the recipe's declarable ones; a recipe that declares
// none leaves the axis free to type.

import {
  Badge,
  CoordinateChip,
  CoordinatesEditor,
  Label,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { Collection } from "../types/api";

const NONE = "__none__";

export interface CollectionPointFieldsProps {
  coll: Collection;
  /** The `profile/channel` references the recipe declares. */
  channels: string[];
  /** Axes a collection may declare — the structural ones are derived. */
  declarableAxes: string[];
  onChange: (patch: Partial<Collection>) => void;
}

export function CollectionPointFields({
  coll,
  channels,
  declarableAxes,
  onChange,
}: CollectionPointFieldsProps) {
  return (
    <div className="space-y-3" data-testid="collection-point">
      <div>
        <Label className="mb-0.5 block text-xs text-muted-foreground">{t("Channel")}</Label>
        {channels.length > 0 ? (
          <Select
            value={coll.channel || NONE}
            onValueChange={(v) => onChange({ channel: v === NONE ? undefined : v })}
          >
            <SelectTrigger className="max-w-xs" aria-label={t("Channel")}>
              <SelectValue placeholder={t("The project's own point")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NONE}>{t("The project's own point")}</SelectItem>
              {channels.map((c) => (
                <SelectItem key={c} value={c}>
                  {c}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t("This project declares no profiles, so its content sits at one point.")}
          </p>
        )}
      </div>

      <CoordinatesEditor
        value={coll.coordinates ?? {}}
        axes={declarableAxes.map((axis) => ({ axis }))}
        allowNewAxis={declarableAxes.length === 0}
        label={t("Coordinates here")}
        emptyText={t("Inherits the project's declared axes.")}
        testId="collection-coordinates"
        onChange={(coordinates) => onChange({ coordinates })}
      />
    </div>
  );
}

/** The point a collection resolves to, for a row that has room for a word. */
export function CollectionPointBadge({
  coll,
  defaults,
}: {
  coll: Collection;
  /** The project's own declared axes, which a collection inherits. */
  defaults?: Record<string, string>;
}) {
  const declared = Object.entries({ ...defaults, ...coll.coordinates });
  if (!coll.channel && declared.length === 0) return null;
  return (
    <span className="inline-flex flex-wrap items-center gap-1" data-testid="collection-point-badge">
      {coll.channel ? (
        <Badge
          variant="outline"
          className="font-mono text-[10px] font-normal text-muted-foreground"
          title={coll.channel}
        >
          {coll.channel}
        </Badge>
      ) : (
        declared.map(([axis, value]) => <CoordinateChip key={axis} axis={axis} value={value} />)
      )}
    </span>
  );
}
