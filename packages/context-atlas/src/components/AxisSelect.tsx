import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
  cn,
} from "@neokapi/ui-primitives";
import { axisLabel } from "../labels";
import type { Atlas } from "../model/atlas";

export interface AxisSelectProps {
  atlas: Atlas;
  value: string;
  onChange: (axis: string) => void;
  /** Accessible name for the picker. */
  label: string;
  /** Leave out the axes the view derives from time. */
  dataOnly?: boolean;
  /** Only axes of this kind. */
  kind?: string;
  className?: string;
}

/** Pick an axis of the atlas: the data's own, then the ones read over time. */
export function AxisSelect({
  atlas,
  value,
  onChange,
  label,
  dataOnly,
  kind,
  className,
}: AxisSelectProps) {
  const data = atlas.axes.filter((a) => a.origin !== "view" && (!kind || a.kind === kind));
  const view = dataOnly || kind ? [] : atlas.axes.filter((a) => a.origin === "view");
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger size="sm" aria-label={label} className={cn("h-7 text-[13px]", className)}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectLabel>Axes</SelectLabel>
          {data.map((a) => (
            <SelectItem key={a.id} value={a.id}>
              {axisLabel(atlas, a.id)}
            </SelectItem>
          ))}
        </SelectGroup>
        {view.length > 0 && (
          <SelectGroup>
            <SelectLabel>Over time</SelectLabel>
            {view.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {axisLabel(atlas, a.id)}
              </SelectItem>
            ))}
          </SelectGroup>
        )}
      </SelectContent>
    </Select>
  );
}
