import { cn } from "@neokapi/ui-primitives";
import { resolveOverlaySpans, segmentText } from "@neokapi/ui-primitives/preview";
import type { ContentNode } from "@neokapi/ui-primitives/preview";

/**
 * The one review atom the platform keeps for itself. The five layers of the
 * review model are the shared cards in `@neokapi/ui-primitives`, fed through
 * `reviewModel.ts`; the desktop opens the document to read the marks a finding
 * left, while the platform's queue draws them on the target in place.
 */

/**
 * The target as the checks marked it. Spans come from the projected node's
 * overlays through the kit's own resolver, so a finding that carries a run
 * anchor lands on the words it names and one that carries none is listed below
 * rather than guessed onto a span.
 */
export function AnchoredTarget({
  node,
  side,
  text,
}: {
  node: ContentNode | null;
  side: string;
  text: string;
}) {
  const spans = node ? resolveOverlaySpans(node.overlays, side, text) : [];
  const segments = segmentText(text, spans);
  if (segments.length === 0) return null;
  return (
    <p className="text-sm leading-relaxed" data-testid="anchored-target">
      {segments.map((segment, i) =>
        segment.overlay ? (
          <mark
            key={i}
            className={cn("rounded-sm px-0.5", segment.overlay.style.className)}
            title={segment.overlay.tooltip}
            data-testid={`anchored-mark-${segment.overlay.type}`}
          >
            {segment.text}
          </mark>
        ) : (
          <span key={i}>{segment.text}</span>
        ),
      )}
    </p>
  );
}
