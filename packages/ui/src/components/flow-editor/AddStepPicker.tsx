// The add-step picker: the same tool list the tools surface shows, in a dialog.
//
// Picking a tool appends it as a step. It is the one place a flow grows, so it
// reuses the tool list rather than inventing a second one.

import { useMemo, useState } from "react";
import { Plus, Search, Wrench } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button } from "../ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "../ui/dialog";
import { Markdown } from "../ui/markdown";
import { ScrollArea } from "../ui/scroll-area";
import type { FlowTool } from "./types";

export interface AddStepPickerProps {
  tools: FlowTool[];
  onAdd: (toolName: string) => void;
  /** Injected in tests and stories to open the dialog without a click. */
  defaultOpen?: boolean;
}

export function AddStepPicker({ tools, onAdd, defaultOpen }: AddStepPickerProps) {
  const [open, setOpen] = useState(defaultOpen ?? false);
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return tools;
    return tools.filter(
      (tool) =>
        tool.name.toLowerCase().includes(q) ||
        (tool.display_name?.toLowerCase().includes(q) ?? false) ||
        tool.description.toLowerCase().includes(q),
    );
  }, [tools, search]);

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)} data-testid="add-step">
        <Plus className="mr-1 size-3" />
        {t("Add step")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("Add a step")}</DialogTitle>
          </DialogHeader>
          <div className="relative">
            <Search
              size={13}
              className="absolute top-1/2 left-2.5 -translate-y-1/2 text-muted-foreground"
            />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("Search tools...")}
              aria-label={t("Search tools")}
              autoFocus
              className="w-full rounded-md border border-input bg-transparent py-1.5 pr-3 pl-8 text-xs outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
          <ScrollArea className="max-h-80">
            <ul className="flex flex-col gap-1">
              {filtered.map((tool) => (
                <li key={tool.name}>
                  <button
                    type="button"
                    data-testid="add-step-tool"
                    onClick={() => {
                      onAdd(tool.name);
                      setOpen(false);
                      setSearch("");
                    }}
                    className="flex w-full items-start gap-2.5 rounded-lg border border-transparent p-2.5 text-left hover:border-primary/20 hover:bg-accent/50"
                  >
                    <Wrench className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
                    <div className="min-w-0 flex-1">
                      <div className="text-xs font-semibold text-foreground">
                        {tool.display_name || tool.name}
                      </div>
                      <div className="line-clamp-2 text-[11px] text-muted-foreground">
                        <Markdown inline>{tool.description}</Markdown>
                      </div>
                    </div>
                  </button>
                </li>
              ))}
              {filtered.length === 0 && (
                <li className="px-2.5 py-4 text-center text-xs text-muted-foreground">
                  {t("No tools match your search.")}
                </li>
              )}
            </ul>
          </ScrollArea>
        </DialogContent>
      </Dialog>
    </>
  );
}
