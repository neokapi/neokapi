import { useState } from "react";
import { RefreshCw, ChevronDown, ChevronRight } from "lucide-react";
import { cn } from "../../../lib/utils";
import { Input } from "../../ui/input";
import { Label } from "../../ui/label";

export function RetryPolicySection({
  values,
  onChange,
  compact,
}: {
  values: Record<string, unknown>;
  onChange: (values: Record<string, unknown>) => void;
  compact?: boolean;
}) {
  const [collapsed, setCollapsed] = useState(true);
  const retry = (values.__retry as Record<string, unknown>) ?? {};

  const update = (key: string, value: unknown) => {
    onChange({ ...values, __retry: { ...retry, [key]: value } });
  };

  return (
    <div className="mt-4">
      <button
        type="button"
        className="flex items-center gap-1.5 w-full text-left pb-1.5 border-b"
        onClick={() => setCollapsed(!collapsed)}
      >
        {collapsed ? (
          <ChevronRight className="size-3 text-muted-foreground" />
        ) : (
          <ChevronDown className="size-3 text-muted-foreground" />
        )}
        <RefreshCw className="size-3 text-muted-foreground" />
        <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
          Retry Policy
        </span>
      </button>

      {!collapsed && (
        <div className={cn("space-y-3 mt-3", compact && "space-y-2")}>
          <div className="space-y-1">
            <Label className="text-xs">Max Retries</Label>
            <Input
              type="number"
              min={0}
              max={10}
              value={retry.maxRetries != null ? String(retry.maxRetries) : ""}
              placeholder="0"
              className="h-8 text-xs"
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                update("maxRetries", e.target.value === "" ? undefined : parseInt(e.target.value))
              }
            />
          </div>

          <div className="space-y-1">
            <Label className="text-xs">Backoff (ms)</Label>
            <Input
              type="number"
              min={0}
              value={retry.backoffMs != null ? String(retry.backoffMs) : ""}
              placeholder="1000"
              className="h-8 text-xs"
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                update("backoffMs", e.target.value === "" ? undefined : parseInt(e.target.value))
              }
            />
          </div>

          <div className="space-y-1">
            <Label className="text-xs">Retry On</Label>
            <Input
              value={String(retry.retryOn ?? "")}
              placeholder="Error pattern..."
              className="h-8 text-xs"
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                update("retryOn", e.target.value || undefined)
              }
            />
          </div>
        </div>
      )}
    </div>
  );
}
