// The breadcrumbs ARE the resolution ladder (Apache-2.0).
//
// Not navigation chrome that happens to sit above a view: each rung is the
// `context://` address of a real point, and selecting it asks the same three
// questions there. A pinned rung renders as text — the reader sees where the
// mount stands without being offered a move it cannot make.

import {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
  SimpleTooltip,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { useScope } from "./ScopeProvider";
import { AddressChip } from "./atoms";
import type { LadderRung } from "./scope";

export interface LadderBreadcrumbsProps {
  /** Rungs beyond this many collapse into an ellipsis at the head. */
  maxRungs?: number;
  className?: string;
}

/** The label a rung shows: its value, with the root named after its dimension. */
function rungLabel(rung: LadderRung): string {
  return rung.value;
}

export function LadderBreadcrumbs({ maxRungs = 6, className }: LadderBreadcrumbsProps) {
  const { ladder, setScope, address } = useScope();

  const collapsed = ladder.length > maxRungs;
  const shown = collapsed ? ladder.slice(ladder.length - maxRungs) : ladder;
  const hidden = collapsed ? ladder.slice(0, ladder.length - maxRungs) : [];

  return (
    <div className={className} data-testid="context-ladder">
      <Breadcrumb>
        <BreadcrumbList>
          {hidden.length > 0 && (
            <>
              <BreadcrumbItem>
                <SimpleTooltip content={hidden.map(rungLabel).join(" / ")}>
                  <BreadcrumbEllipsis />
                </SimpleTooltip>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
            </>
          )}
          {shown.map((rung, i) => {
            const last = i === shown.length - 1;
            return (
              <span key={`${rung.dimension}:${rung.address}`} className="contents">
                <BreadcrumbItem>
                  {last || !rung.navigable ? (
                    <BreadcrumbPage
                      data-dimension={rung.dimension}
                      data-navigable={rung.navigable}
                      title={rung.address}
                    >
                      {rungLabel(rung)}
                    </BreadcrumbPage>
                  ) : (
                    <BreadcrumbLink
                      asChild
                      data-dimension={rung.dimension}
                      data-navigable={rung.navigable}
                    >
                      <button
                        type="button"
                        title={rung.address}
                        onClick={() => setScope(rung.scope)}
                      >
                        {rungLabel(rung)}
                      </button>
                    </BreadcrumbLink>
                  )}
                </BreadcrumbItem>
                {!last && <BreadcrumbSeparator />}
              </span>
            );
          })}
          {shown.length === 0 && (
            <BreadcrumbItem>
              <BreadcrumbPage data-dimension="root">{t("Everything in scope")}</BreadcrumbPage>
            </BreadcrumbItem>
          )}
        </BreadcrumbList>
      </Breadcrumb>
      <AddressChip address={address} className="mt-1.5" />
    </div>
  );
}
