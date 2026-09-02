import { useState } from "react";
import { Badge, Button, Collapsible, CollapsibleContent, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { ChevronRight } from "lucide-react";
import type { AIActivityEntry, AIExchange, AIMessage } from "../types/api";

/** Flatten a message to text. A message is a list of parts, always: a text-only
 *  call is one text part, and a multimodal one carries media parts beside it. */
function messageText(m: AIMessage): string {
  if (!m.parts) return "";
  return m.parts
    .map((p) => (p.kind === "text" ? (p.text ?? "") : `[${p.kind}]`))
    .filter(Boolean)
    .join("\n");
}

function roleLabel(role: string): string {
  switch (role) {
    case "system":
      return t("System");
    case "user":
      return t("Sent");
    case "assistant":
      return t("Model");
    default:
      return role;
  }
}

function tokenSummary(ex: AIExchange): string | null {
  const u = ex.usage;
  if (!u) return null;
  const parts: string[] = [];
  if (u.input_tokens) parts.push(t("{n} in", { n: u.input_tokens }));
  if (u.output_tokens) parts.push(t("{n} out", { n: u.output_tokens }));
  if (u.cache_read_tokens) parts.push(t("{n} cached", { n: u.cache_read_tokens }));
  return parts.length > 0 ? parts.join(" · ") : null;
}

/**
 * One LLM exchange, rendered in full: every message as it went on the wire, the
 * schema when the call constrained its output, and the reply.
 *
 * The text is shown verbatim and never trimmed to a preview. A reviewer reading
 * this is checking whether the model was told the right thing, and a prompt
 * summarised is a prompt they cannot check.
 */
export function AIExchangeView({ exchange }: { exchange: AIExchange }) {
  const tokens = tokenSummary(exchange);
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground">
        <Badge variant="outline" className="font-mono text-[10px]" translate="no">
          {exchange.model || exchange.provider}
        </Badge>
        {exchange.prompt && (
          <span translate="no">
            {exchange.prompt}
            {exchange.prompt_version ? ` ${exchange.prompt_version}` : ""}
          </span>
        )}
        {tokens && <span>{tokens}</span>}
      </div>

      {exchange.messages?.map((m, i) => {
        const text = messageText(m);
        return (
          <div key={i} className="rounded-md border bg-muted/30 p-2">
            <p className="mb-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
              {roleLabel(m.role)}
            </p>
            {text ? (
              <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed">
                {text}
              </pre>
            ) : (
              // A message with no readable text is a defect in this view, not an
              // empty prompt. Saying so beats an empty box that reads as "kapi
              // sent nothing" — which is the one thing it never does.
              <p className="text-[11px] italic text-muted-foreground">
                {t("This message carried no text this view could render.")}
              </p>
            )}
          </div>
        );
      })}

      {exchange.schema && (
        <div className="rounded-md border bg-muted/30 p-2">
          <p className="mb-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
            {t("Required shape")}
          </p>
          <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px]">
            {JSON.stringify(exchange.schema, null, 2)}
          </pre>
        </div>
      )}

      {exchange.response && (
        <div className="rounded-md border border-primary/30 bg-primary/5 p-2">
          <p className="mb-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
            {t("Reply")}
          </p>
          <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed">
            {exchange.response}
          </pre>
        </div>
      )}

      {exchange.error && (
        <p className="rounded-md border border-destructive/40 bg-destructive/5 p-2 text-[11px] text-destructive">
          {exchange.error}
        </p>
      )}
    </div>
  );
}

/**
 * The calls one action made, behind a disclosure.
 *
 * Collapsed by default: a reviewer working through a queue wants the proposal,
 * not the prompt, until the proposal looks wrong. Then this is the first thing
 * they need, and it is one click away rather than absent.
 */
export function AIExchangeDisclosure({
  entries,
  className,
  label,
}: {
  entries: AIActivityEntry[] | undefined;
  className?: string;
  label?: string;
}) {
  const [open, setOpen] = useState(false);
  if (!entries || entries.length === 0) return null;

  return (
    <div className={cn("border-t pt-2", className)} data-slot="ai-exchange-disclosure">
      <Button
        variant="ghost"
        size="xs"
        className="h-6 gap-1 px-1 text-[11px] text-muted-foreground"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <ChevronRight className={cn("size-3 transition-transform", open && "rotate-90")} />
        {label ??
          (entries.length === 1
            ? t("What was sent to the model")
            : t("What was sent to the model ({count} calls)", { count: entries.length }))}
      </Button>
      <Collapsible open={open}>
        <CollapsibleContent className="space-y-3 pt-2">
          {entries.map((e) => (
            <AIExchangeView key={e.id} exchange={e.exchange} />
          ))}
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
