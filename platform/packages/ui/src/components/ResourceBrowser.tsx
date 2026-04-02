import * as React from "react";
import { DatabaseIcon, FileIcon, SearchIcon } from "lucide-react";

import { cn } from "@neokapi/ui-primitives";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Label } from "@neokapi/ui-primitives/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@neokapi/ui-primitives/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@neokapi/ui-primitives/components/ui/tabs";
import type { ResourceKind } from "./ResourcePicker";

export interface ResourceInfo {
  name: string;
  kind: ResourceKind;
  path: string;
  size?: number;
  lastModified?: string;
  entryCount?: number;
  sourceLocale?: string;
  targetLocales?: string[];
}

export interface ResourceBrowserProps {
  /** Whether the dialog is open. */
  open: boolean;
  /** Called when the dialog should close. */
  onClose: () => void;
  /** Called when a resource is selected. Returns "tm:name" or a file path. */
  onSelect: (ref: string) => void;
  /** Resource kind to browse. */
  resourceKind: ResourceKind;
  /** Available resources to display. */
  resources: ResourceInfo[];
}

const kindLabels: Record<ResourceKind, string> = {
  tm: "Translation Memories",
  termbase: "Termbases",
  srx: "Segmentation Rules",
};

const kindPrefixes: Record<ResourceKind, string> = {
  tm: "tm:",
  termbase: "termbase:",
  srx: "srx:",
};

export function ResourceBrowser({
  open,
  onClose,
  onSelect,
  resourceKind,
  resources,
}: ResourceBrowserProps) {
  const [search, setSearch] = React.useState("");
  const [selected, setSelected] = React.useState<string | null>(null);
  const [filePath, setFilePath] = React.useState("");
  const [tab, setTab] = React.useState<"named" | "file">("named");

  const filtered = resources.filter(
    (r) =>
      r.name.toLowerCase().includes(search.toLowerCase()) ||
      r.path.toLowerCase().includes(search.toLowerCase()),
  );

  const handleSelect = () => {
    if (tab === "named" && selected) {
      onSelect(`${kindPrefixes[resourceKind]}${selected}`);
    } else if (tab === "file" && filePath) {
      onSelect(filePath);
    }
    onClose();
  };

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      onClose();
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[540px]">
        <DialogHeader>
          <DialogTitle>Select {kindLabels[resourceKind]}</DialogTitle>
        </DialogHeader>

        <Tabs value={tab} onValueChange={(v) => setTab(v as "named" | "file")}>
          <TabsList>
            <TabsTrigger value="named" className="gap-1.5">
              <DatabaseIcon className="size-3.5" />
              Named Resources
            </TabsTrigger>
            <TabsTrigger value="file" className="gap-1.5">
              <FileIcon className="size-3.5" />
              File Path
            </TabsTrigger>
          </TabsList>

          <TabsContent value="named">
            <div className="flex flex-col gap-3">
              <div className="relative">
                <SearchIcon className="absolute top-2 left-2.5 size-4 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search resources..."
                  className="pl-8"
                />
              </div>

              <div className="max-h-64 overflow-y-auto rounded-md border">
                {filtered.length === 0 ? (
                  <div className="flex flex-col items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
                    <DatabaseIcon className="size-8 opacity-40" />
                    {resources.length === 0
                      ? `No ${resourceKind}s found. Create one first.`
                      : "No results match your search."}
                  </div>
                ) : (
                  <div className="divide-y">
                    {filtered.map((r) => (
                      <button
                        key={r.name}
                        type="button"
                        onClick={() => setSelected(r.name)}
                        className={cn(
                          "flex w-full items-start gap-3 px-3 py-2.5 text-left transition-colors hover:bg-accent/50",
                          selected === r.name && "bg-accent",
                        )}
                      >
                        <DatabaseIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                        <div className="min-w-0 flex-1">
                          <div className="text-sm font-medium">{r.name}</div>
                          <div className="flex items-center gap-3 text-xs text-muted-foreground">
                            {r.entryCount !== undefined && (
                              <span>{r.entryCount.toLocaleString()} entries</span>
                            )}
                            {r.sourceLocale && (
                              <span>
                                {r.sourceLocale}
                                {r.targetLocales?.length
                                  ? ` → ${r.targetLocales.join(", ")}`
                                  : ""}
                              </span>
                            )}
                          </div>
                          <div className="truncate text-xs text-muted-foreground/60">
                            {r.path}
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </TabsContent>

          <TabsContent value="file">
            <div className="flex flex-col gap-3 py-2">
              <div className="flex flex-col gap-1.5">
                <Label>File path</Label>
                <Input
                  value={filePath}
                  onChange={(e) => setFilePath(e.target.value)}
                  placeholder="Enter absolute or relative path..."
                />
              </div>
              <p className="text-xs text-muted-foreground">
                Relative paths are resolved from the project directory.
              </p>
            </div>
          </TabsContent>
        </Tabs>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={handleSelect}
            disabled={
              (tab === "named" && !selected) || (tab === "file" && !filePath)
            }
          >
            Select
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
