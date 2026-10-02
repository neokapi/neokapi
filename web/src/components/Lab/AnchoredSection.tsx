import React from "react";
import useBrokenLinks from "@docusaurus/useBrokenLinks";

type AnchoredSectionProps = React.ComponentPropsWithoutRef<"section"> & { id: string };

/**
 * A section that other pages link to by id. The docs build checks links to a
 * page's anchors only against ids it has collected, and a plain element's id
 * is not collected, so this section registers its id where it renders it.
 */
export function AnchoredSection({ id, ...props }: AnchoredSectionProps): React.ReactElement {
  useBrokenLinks().collectAnchor(id);
  return <section id={id} {...props} />;
}
