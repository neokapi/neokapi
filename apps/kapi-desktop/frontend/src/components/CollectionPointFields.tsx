// Where a collection's content sits: the channel it names, and the declared
// axes it moves off the project's default point.
//
// A collection names ONE channel reference and the structural axes follow from
// it, so this offers the declared references rather than a free-text field that
// can name a channel no profile declares.

import { Plus, Trash2 } from "lucide-react";
import {
  Badge,
  Button,
  CoordinateChip,
  Input,
  Label,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { useState } from "react";
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
  const [axis, setAxis] = useState("");
  const coordinates = coll.coordinates ?? {};
  const rows = Object.entries(coordinates);

  const setCoordinates = (next: Record<string, string>) =>
    onChange({ coordinates: Object.keys(next).length ? next : undefined });

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

      <div>
        <Label className="mb-0.5 block text-xs text-muted-foreground">
          {t("Coordinates here")}
        </Label>
        {rows.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t("Inherits the project's declared axes.")}
          </p>
        ) : (
          <ul className="space-y-1">
            {rows.map(([a, value]) => (
              <li key={a} className="flex items-center gap-2">
                <code className="w-24 shrink-0 font-mono text-xs">{a}</code>
                <Input
                  className="max-w-[14rem]"
                  value={value}
                  aria-label={t("{axis} here", { axis: a })}
                  onChange={(e) => setCoordinates({ ...coordinates, [a]: e.target.value })}
                />
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t("Remove {axis}", { axis: a })}
                  onClick={() => {
                    const next = { ...coordinates };
                    delete next[a];
                    setCoordinates(next);
                  }}
                >
                  <Trash2 className="size-3.5" />
                </Button>
              </li>
            ))}
          </ul>
        )}
        <div className="mt-1 flex items-center gap-2">
          {declarableAxes.length > 0 ? (
            <Select value={axis} onValueChange={setAxis}>
              <SelectTrigger className="max-w-[12rem]" aria-label={t("New axis")}>
                <SelectValue placeholder={t("axis")} />
              </SelectTrigger>
              <SelectContent>
                {declarableAxes
                  .filter((a) => !(a in coordinates))
                  .map((a) => (
                    <SelectItem key={a} value={a}>
                      {a}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          ) : (
            <Input
              className="max-w-[12rem]"
              placeholder={t("axis")}
              value={axis}
              aria-label={t("New axis")}
              onChange={(e) => setAxis(e.target.value)}
            />
          )}
          <Button
            variant="ghost"
            size="sm"
            disabled={!axis.trim() || axis.trim() in coordinates}
            onClick={() => {
              setCoordinates({ ...coordinates, [axis.trim()]: "" });
              setAxis("");
            }}
          >
            <Plus className="mr-1 size-3" />
            {t("Add axis")}
          </Button>
        </div>
      </div>
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
