// The command line that runs a tool on files, as a terminal icon that copies
// it: the command-line equivalent pattern in docs/page-patterns.md.

import { useState } from "react";
import { Check, Terminal } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button } from "../ui/button";
import { SimpleTooltip } from "../ui/tooltip";
import { cn } from "../../lib/utils";

export interface ToolCommandCopyProps {
  /** The tool's registry name. */
  tool: string;
  className?: string;
}

export function ToolCommandCopy({ tool, className }: ToolCommandCopyProps) {
  const [copied, setCopied] = useState(false);
  const command = `kapi exec ${tool} <files...>`;
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // A denied clipboard leaves the command in the tooltip to copy by hand.
    }
  };
  return (
    <SimpleTooltip content={copied ? t("Copied") : command}>
      <Button
        variant="ghost"
        size="icon-xs"
        className={cn("text-muted-foreground", className)}
        data-slot="tool-command"
        data-command={command}
        aria-label={t("Copy {command} for the terminal", { command })}
        onClick={() => void copy()}
      >
        {copied ? <Check className="size-3.5" /> : <Terminal className="size-3.5" />}
      </Button>
    </SimpleTooltip>
  );
}
