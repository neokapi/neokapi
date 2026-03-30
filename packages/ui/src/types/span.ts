/** Inline span info — shared between framework (kapi-desktop) and platform (bowrain). */
export interface SpanInfo {
  span_type: "opening" | "closing" | "placeholder";
  type: string;
  sub_type?: string;
  id: string;
  data: string;
  display_text?: string;
  equiv_text?: string;
  deletable?: boolean;
  cloneable?: boolean;
  can_reorder?: boolean;
}
