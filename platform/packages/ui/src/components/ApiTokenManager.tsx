import { useState, useEffect, useCallback } from "react";
import type { ApiToken, CreateApiTokenResponse, Workspace } from "../types/api";
import { useApi } from "../context/ApiContext";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { GlassCard } from "./ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "./ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "./ui/select";
import { AlertGlass, AlertGlassDescription } from "./ui/alert";
import { KeyRound, Trash2, Copy, Clock } from "./icons";

interface ApiTokenManagerProps {
  workspace: Workspace;
}

/** Format a Date as YYYY-MM-DD for the date input value. */
function toDateString(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

/** Format a Date as "Mon DD, YYYY" for display. */
function formatShortDate(d: Date): string {
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
}

/** Add days to today and return as YYYY-MM-DD. */
function addDays(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return toDateString(d);
}

type ExpiryPreset = "7" | "30" | "60" | "90" | "custom" | "never";

const PRESETS: { value: ExpiryPreset; days: number | null; label: string }[] = [
  { value: "7", days: 7, label: "7 days" },
  { value: "30", days: 30, label: "30 days" },
  { value: "60", days: 60, label: "60 days" },
  { value: "90", days: 90, label: "90 days" },
  { value: "custom", days: null, label: "Custom" },
  { value: "never", days: null, label: "No expiration" },
];

export function ApiTokenManager({ workspace }: ApiTokenManagerProps) {
  const api = useApi();
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [showDialog, setShowDialog] = useState(false);
  const [name, setName] = useState("");
  const [expiryPreset, setExpiryPreset] = useState<ExpiryPreset>("30");
  const [customDate, setCustomDate] = useState(addDays(30));
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const [createdToken, setCreatedToken] = useState<CreateApiTokenResponse | null>(null);
  const [copied, setCopied] = useState(false);
  const [deleteTokenId, setDeleteTokenId] = useState<string | null>(null);

  const loadTokens = useCallback(async () => {
    try {
      const list = await api.listApiTokens(workspace.slug);
      setTokens(list);
    } catch {
      setTokens([]);
    } finally {
      setLoading(false);
    }
  }, [api, workspace.slug]);

  useEffect(() => {
    void loadTokens();
  }, [loadTokens]);

  /** Compute expire_days from the current expiry selection. */
  const getExpireDays = (): number => {
    if (expiryPreset === "never") return 0;
    if (expiryPreset === "custom") {
      const target = new Date(customDate + "T00:00:00");
      const now = new Date();
      now.setHours(0, 0, 0, 0);
      return Math.max(1, Math.ceil((target.getTime() - now.getTime()) / (1000 * 60 * 60 * 24)));
    }
    return parseInt(expiryPreset, 10);
  };

  /** Display label for the currently selected expiry. */
  const getExpiryLabel = (): string => {
    if (expiryPreset === "never") return "No expiration";
    if (expiryPreset === "custom") {
      const d = new Date(customDate + "T00:00:00");
      return isNaN(d.getTime()) ? "Custom" : `Custom (${formatShortDate(d)})`;
    }
    const days = parseInt(expiryPreset, 10);
    const d = new Date();
    d.setDate(d.getDate() + days);
    return `${days} days (${formatShortDate(d)})`;
  };

  const handleCreate = async () => {
    if (!name.trim()) return;
    setCreating(true);
    setError("");
    try {
      const days = getExpireDays();
      const resp = await api.createApiToken(workspace.slug, name.trim(), days);
      setCreatedToken(resp);
      setTokens((prev) => [
        {
          id: resp.id,
          user_id: "",
          workspace_id: "",
          name: resp.name,
          token_prefix: resp.token_prefix,
          scopes: resp.scopes,
          last_used_at: null,
          expires_at: resp.expires_at,
          created_at: resp.created_at,
        },
        ...prev,
      ]);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to create token");
    } finally {
      setCreating(false);
    }
  };

  const handleConfirmDelete = async () => {
    if (!deleteTokenId) return;
    try {
      await api.deleteApiToken(workspace.slug, deleteTokenId);
      setTokens((prev) => prev.filter((t) => t.id !== deleteTokenId));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to delete token");
    } finally {
      setDeleteTokenId(null);
    }
  };

  const handleCopyToken = (token: string) => {
    void navigator.clipboard.writeText(token);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDialogChange = (open: boolean) => {
    if (!open) {
      setName("");
      setExpiryPreset("30");
      setCustomDate(addDays(30));
      setError("");
      setCreatedToken(null);
      setCopied(false);
    }
    setShowDialog(open);
  };

  const handlePresetChange = (value: string) => {
    const preset = value as ExpiryPreset;
    setExpiryPreset(preset);
    // When switching to custom, seed with 30 days from now
    if (preset === "custom") {
      setCustomDate(addDays(30));
    }
  };

  const isExpired = (t: ApiToken) => t.expires_at != null && new Date(t.expires_at) < new Date();

  const formatExpiry = (expiresAtVal: string | null): string => {
    if (expiresAtVal == null) return "Never";
    const d = new Date(expiresAtVal);
    if (isNaN(d.getTime())) return "Never";
    if (d < new Date()) return "Expired";
    return d.toLocaleDateString();
  };

  const canManage = workspace.role === "owner" || workspace.role === "admin";

  if (!canManage) return null;

  return (
    <>
      <GlassCard intensity="subtle" className="p-6" data-testid="api-token-manager">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h3 className="text-lg font-semibold flex items-center gap-2">
              <KeyRound className="h-4 w-4" />
              API Tokens
            </h3>
            <p className="text-[13px] text-muted-foreground mt-1">
              Manage API tokens for programmatic access
            </p>
          </div>
          <Button size="sm" onClick={() => setShowDialog(true)} data-testid="token-open-dialog-btn">
            Create Token
          </Button>
        </div>

        {error && (
          <AlertGlass
            variant="destructive"
            dismissible
            onDismiss={() => setError("")}
            className="mb-4"
          >
            <AlertGlassDescription>{error}</AlertGlassDescription>
          </AlertGlass>
        )}

        {/* Token list */}
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading tokens...</div>
        ) : tokens.length === 0 ? (
          <div className="py-8 text-center text-sm text-muted-foreground">No API tokens</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Name
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Prefix
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Last Used
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Expires
                  </th>
                  <th className="px-4 py-2.5 text-sm font-medium text-muted-foreground w-[60px]"></th>
                </tr>
              </thead>
              <tbody data-testid="token-list">
                {tokens.map((t) => {
                  const expired = isExpired(t);
                  return (
                    <tr
                      key={t.id}
                      className={`border-b border-border/50 transition-colors hover:bg-accent/50 ${expired ? "opacity-50" : ""}`}
                    >
                      <td className="px-4 py-2.5 text-sm font-medium">{t.name}</td>
                      <td className="px-4 py-2.5 text-sm font-mono text-muted-foreground">
                        {t.token_prefix}...
                      </td>
                      <td className="px-4 py-2.5 text-sm text-muted-foreground">
                        {t.last_used_at ? new Date(t.last_used_at).toLocaleDateString() : "Never"}
                      </td>
                      <td className="px-4 py-2.5 text-sm text-muted-foreground">
                        <span className="inline-flex items-center gap-1.5">
                          <Clock className="h-3 w-3" />
                          {formatExpiry(t.expires_at)}
                        </span>
                      </td>
                      <td className="px-4 py-2.5">
                        <div className="flex gap-1 justify-end">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 w-7 p-0 text-destructive hover:text-destructive"
                            onClick={() => setDeleteTokenId(t.id)}
                            title="Delete token"
                            data-testid="token-delete-btn"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </GlassCard>

      {/* Create token dialog */}
      <Dialog open={showDialog} onOpenChange={handleDialogChange}>
        <DialogContent size="sm" onInteractOutside={(e: Event) => e.preventDefault()}>
          <DialogHeader>
            <DialogTitle>{createdToken ? "Token Created" : "Create API Token"}</DialogTitle>
          </DialogHeader>

          {createdToken ? (
            <div className="flex flex-col gap-4 py-2">
              <AlertGlass variant="default">
                <AlertGlassDescription>
                  Copy this token now. You won't be able to see it again.
                </AlertGlassDescription>
              </AlertGlass>
              <div>
                <Label className="text-muted-foreground">Token</Label>
                <div className="mt-1 flex gap-2">
                  <Input
                    value={createdToken.token}
                    readOnly
                    className="font-mono text-xs"
                    data-testid="token-plaintext"
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleCopyToken(createdToken.token)}
                    data-testid="token-copy-btn"
                  >
                    {copied ? "Copied" : <Copy className="h-4 w-4" />}
                  </Button>
                </div>
              </div>
            </div>
          ) : (
            <div className="flex flex-col gap-4 py-2">
              <div>
                <Label className="text-muted-foreground">Name</Label>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. CI/CD Pipeline"
                  autoFocus
                  className="mt-1"
                  data-testid="token-name-input"
                  onKeyDown={(e) => e.key === "Enter" && handleCreate()}
                />
              </div>
              <div>
                <Label className="text-muted-foreground">Expiration</Label>
                <Select value={expiryPreset} onValueChange={handlePresetChange}>
                  <SelectTrigger className="mt-1" data-testid="token-expiry-select">
                    <SelectValue>{getExpiryLabel()}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {PRESETS.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        {p.days != null
                          ? `${p.label} (${formatShortDate(
                              (() => {
                                const d = new Date();
                                d.setDate(d.getDate() + p.days);
                                return d;
                              })(),
                            )})`
                          : p.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {expiryPreset !== "never" && (
                  <p className="text-xs text-muted-foreground mt-1.5">
                    The token will expire on the selected date
                  </p>
                )}
              </div>
              {expiryPreset === "custom" && (
                <div>
                  <Label className="text-muted-foreground">Select date</Label>
                  <Input
                    type="date"
                    value={customDate}
                    min={toDateString(new Date())}
                    onChange={(e) => setCustomDate(e.target.value)}
                    className="mt-1"
                    data-testid="token-custom-date-input"
                  />
                </div>
              )}
              {error && (
                <AlertGlass variant="destructive">
                  <AlertGlassDescription>{error}</AlertGlassDescription>
                </AlertGlass>
              )}
            </div>
          )}

          <DialogFooter>
            {createdToken ? (
              <Button onClick={() => handleDialogChange(false)} data-testid="token-done-btn">
                Done
              </Button>
            ) : (
              <>
                <Button variant="outline" onClick={() => handleDialogChange(false)}>
                  Cancel
                </Button>
                <Button
                  onClick={handleCreate}
                  disabled={creating || !name.trim() || (expiryPreset === "custom" && !customDate)}
                  data-testid="token-submit-btn"
                >
                  {creating ? "Creating..." : "Create"}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirmation dialog */}
      <Dialog
        open={deleteTokenId !== null}
        onOpenChange={(open: boolean) => {
          if (!open) setDeleteTokenId(null);
        }}
      >
        <DialogContent size="sm">
          <DialogHeader>
            <DialogTitle>Delete API Token</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground py-2">
            Are you sure you want to delete this token? Any applications using it will lose access
            immediately.
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTokenId(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleConfirmDelete}
              data-testid="token-confirm-delete-btn"
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
