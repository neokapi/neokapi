// EndpointInspectorPanel — the right-overlay panel shown when a Source / Sink
// endpoint's Inspect affordance is used. The flow editor owns only the chrome
// (header, accent, close); the body comes from the host's renderEndpointPanel,
// because what an endpoint "contains" is host knowledge — e.g. the content
// model a reader produced from the bound input, or the bytes a writer emitted.

import type { ReactNode } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { Button, PanelHeader } from "@neokapi/ui-primitives";

export interface EndpointInspectorPanelProps {
  role: "source" | "sink";
  onClose: () => void;
  children: ReactNode;
}

// `get` accessors defer the t() dictionary lookup to render time, so
// translations loaded after module evaluation still apply.
const ROLE_META = {
  // Match the endpoint pill accents (reader green / writer amber).
  source: {
    get title() {
      return t("Source", "flow endpoint");
    },
    get subtitle() {
      return t("What enters the flow");
    },
    accent: "oklch(0.7 0.17 145)",
  },
  sink: {
    get title() {
      return t("Sink", "flow endpoint");
    },
    get subtitle() {
      return t("What the flow wrote");
    },
    accent: "oklch(0.7 0.13 85)",
  },
} as const;

export function EndpointInspectorPanel({ role, onClose, children }: EndpointInspectorPanelProps) {
  const meta = ROLE_META[role];
  return (
    <div
      className="flex h-full flex-col overflow-hidden border-l border-border bg-background"
      style={{ width: "min(400px, calc(100vw - 2rem))" }}
    >
      <PanelHeader className="flex-col items-start gap-0.5 py-2.5">
        <div className="flex w-full items-center justify-between">
          <div className="flex items-center gap-1.5 text-[11px] font-semibold text-foreground">
            <span className="size-2 rounded-full" style={{ background: meta.accent }} />
            {meta.title}
          </div>
          <Button variant="ghost" size="xs" className="h-5 px-1.5 text-[9px]" onClick={onClose}>
            Close
          </Button>
        </div>
        <div className="text-[10px] text-muted-foreground">{meta.subtitle}</div>
      </PanelHeader>

      {/* Native scrolling (not Radix ScrollArea): its display:table viewport
          sizes to the widest tree row, which stops prose from wrapping on
          phones. Here text wraps at the panel width and only genuinely wide
          content (the content tree) scrolls horizontally. */}
      <div className="flex-1 overflow-auto">
        <div className="px-3 py-2">{children}</div>
      </div>
    </div>
  );
}
