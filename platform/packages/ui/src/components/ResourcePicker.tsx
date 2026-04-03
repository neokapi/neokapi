import * as React from "react";
import { DatabaseIcon, FileIcon, FolderIcon } from "lucide-react";

import { cn } from "@neokapi/ui-primitives";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import { Label } from "@neokapi/ui-primitives/components/ui/label";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@neokapi/ui-primitives/components/ui/tabs";
import {
  Combobox,
  ComboboxInput,
  ComboboxContent,
  ComboboxList,
  ComboboxItem,
  ComboboxEmpty,
} from "@neokapi/ui-primitives/components/ui/combobox";

export type ResourceKind = "tm" | "termbase" | "srx";

export interface ResourceOption {
  name: string;
  path?: string;
  entryCount?: number;
}

export interface ResourcePickerProps {
  /** Current value: "tm:project-memory", "./path/to/file", etc. */
  value: string;
  /** Called when the value changes. */
  onChange: (value: string) => void;
  /** Resource kind — enables Named Resource mode with URI prefix. */
  resourceKind?: ResourceKind;
  /** Path type hint for the file input. */
  pathType?: "file" | "directory";
  /** Role hint — "output" shows auto-placement info. */
  role?: "input" | "output";
  /** Available named resources for the combobox dropdown. */
  resources?: ResourceOption[];
  /** Field label. */
  label?: string;
  /** Placeholder text. */
  placeholder?: string;
  /** Resolved path hint shown below the input. */
  resolvedPath?: string;
  /** Additional class name for the root element. */
  className?: string;
  /** Whether the field is disabled. */
  disabled?: boolean;
}

const kindLabels: Record<ResourceKind, string> = {
  tm: "Named TM",
  termbase: "Named Termbase",
  srx: "Named SRX",
};

const kindIcons: Record<ResourceKind, React.ReactNode> = {
  tm: <DatabaseIcon className="size-3.5" />,
  termbase: <DatabaseIcon className="size-3.5" />,
  srx: <FileIcon className="size-3.5" />,
};

const kindPrefixes: Record<ResourceKind, string> = {
  tm: "tm:",
  termbase: "termbase:",
  srx: "srx:",
};

function parseValue(value: string, resourceKind?: ResourceKind) {
  if (!resourceKind || !value) return { mode: "named" as const, name: "" };
  const prefix = kindPrefixes[resourceKind];
  if (value.startsWith(prefix)) {
    return { mode: "named" as const, name: value.slice(prefix.length) };
  }
  return { mode: "file" as const, name: value };
}

export function ResourcePicker({
  value,
  onChange,
  resourceKind,
  pathType = "file",
  role,
  resources = [],
  label,
  placeholder,
  resolvedPath,
  className,
  disabled = false,
}: ResourcePickerProps) {
  const parsed = parseValue(value, resourceKind);
  const [mode, setMode] = React.useState<"named" | "file">(parsed.mode);

  // When no resourceKind, just render a plain path input.
  if (!resourceKind) {
    return (
      <div className={cn("flex flex-col gap-1.5", className)}>
        {label && <Label>{label}</Label>}
        <div className="flex items-center gap-2">
          {pathType === "directory" ? (
            <FolderIcon className="size-4 shrink-0 text-muted-foreground" />
          ) : (
            <FileIcon className="size-4 shrink-0 text-muted-foreground" />
          )}
          <Input
            value={value}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
            placeholder={placeholder ?? "Enter file path..."}
            disabled={disabled}
          />
        </div>
        {resolvedPath && (
          <p className="truncate pl-6 text-xs text-muted-foreground">
            → {resolvedPath}
          </p>
        )}
      </div>
    );
  }

  const handleNamedChange = (name: string) => {
    if (name) {
      onChange(`${kindPrefixes[resourceKind]}${name}`);
    } else {
      onChange("");
    }
  };

  const handleFileChange = (path: string) => {
    onChange(path);
  };

  const handleModeChange = (newMode: string) => {
    setMode(newMode as "named" | "file");
    onChange("");
  };

  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      {label && <Label>{label}</Label>}
      <Tabs value={mode} onValueChange={handleModeChange}>
        <TabsList className="h-7">
          <TabsTrigger value="named" className="gap-1 text-xs" disabled={disabled}>
            {kindIcons[resourceKind]}
            {kindLabels[resourceKind]}
          </TabsTrigger>
          <TabsTrigger value="file" className="gap-1 text-xs" disabled={disabled}>
            <FileIcon className="size-3.5" />
            File
          </TabsTrigger>
        </TabsList>
        <TabsContent value="named">
          <Combobox
            value={parsed.mode === "named" ? parsed.name : ""}
            onValueChange={(val: string | null) => handleNamedChange(val ?? "")}
          >
            <ComboboxInput
              placeholder={placeholder ?? `Select ${resourceKind}...`}
              disabled={disabled}
            />
            <ComboboxContent>
              <ComboboxList>
                <ComboboxEmpty>No {resourceKind}s found.</ComboboxEmpty>
                {resources.map((r) => (
                  <ComboboxItem key={r.name} value={r.name}>
                    <div className="flex flex-col">
                      <span>{r.name}</span>
                      {r.entryCount !== undefined && (
                        <span className="text-xs text-muted-foreground">
                          {r.entryCount.toLocaleString()} entries
                        </span>
                      )}
                    </div>
                  </ComboboxItem>
                ))}
              </ComboboxList>
            </ComboboxContent>
          </Combobox>
        </TabsContent>
        <TabsContent value="file">
          <div className="flex items-center gap-2">
            {pathType === "directory" ? (
              <FolderIcon className="size-4 shrink-0 text-muted-foreground" />
            ) : (
              <FileIcon className="size-4 shrink-0 text-muted-foreground" />
            )}
            <Input
              value={parsed.mode === "file" ? parsed.name : ""}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => handleFileChange(e.target.value)}
              placeholder="Enter file path..."
              disabled={disabled}
            />
          </div>
        </TabsContent>
      </Tabs>
      {resolvedPath && (
        <p className="truncate text-xs text-muted-foreground">
          → {resolvedPath}
        </p>
      )}
      {role === "output" && !resolvedPath && (
        <p className="text-xs text-muted-foreground/60">
          Output files are auto-placed in the output directory
        </p>
      )}
    </div>
  );
}
