import { useState, useEffect, useCallback } from "react";
import type { ApiToken, CreateApiTokenResponse, Workspace } from "../types/api";
import { useApi } from "../context/ApiContext";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { GlassCard } from "./ui/card";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "./ui/dialog";
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

/** Default expiry: 30 days from now. */
function defaultExpiry(): string {
  const d = new Date();
  d.setDate(d.getDate() + 30);
  return toDateString(d);
}

export function ApiTokenManager({ workspace }: ApiTokenManagerProps) {
  const api = useApi();
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [showDialog, setShowDialog] = useState(false);
  const [name, setName] = useState("");
  const [expiresAt, setExpiresAt] = useState(defaultExpiry);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const [createdToken, setCreatedToken] = useState<CreateApiTokenResponse | null>(null);
  const [copied, setCopied] = useState(false);

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
    loadTokens();
  }, [loadTokens]);

  const handleCreate = async () => {
    if (!name.trim()) return;
    setCreating(true);
    setError("");
    try {
      // Compute days from today to the selected date.
      const target = new Date(expiresAt + "T00:00:00");
      const now = new Date();
      now.setHours(0, 0, 0, 0);
      const diffMs = target.getTime() - now.getTime();
      const days = Math.max(1, Math.ceil(diffMs / (1000 * 60 * 60 * 24)));

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

  const handleDelete = async (tokenId: string) => {
    try {
      await api.deleteApiToken(workspace.slug, tokenId);
      setTokens((prev) => prev.filter((t) => t.id !== tokenId));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to delete token");
    }
  };

  const handleCopyToken = (token: string) => {
    navigator.clipboard.writeText(token);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDialogChange = (open: boolean) => {
    if (!open) {
      setName("");
      setExpiresAt(defaultExpiry());
      setError("");
      setCreatedToken(null);
      setCopied(false);
    }
    setShowDialog(open);
  };

  const isExpired = (t: ApiToken) =>
    t.expires_at != null && new Date(t.expires_at) < new Date();

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
          <AlertGlass variant="destructive" dismissible onDismiss={() => setError("")} className="mb-4">
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
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Name</th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Prefix</th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Last Used</th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">Expires</th>
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
                        {t.last_used_at
                          ? new Date(t.last_used_at).toLocaleDateString()
                          : "Never"}
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
                            onClick={() => handleDelete(t.id)}
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
                <Label className="text-muted-foreground">Expiry Date</Label>
                <Input
                  type="date"
                  value={expiresAt}
                  min={toDateString(new Date())}
                  onChange={(e) => setExpiresAt(e.target.value)}
                  className="mt-1"
                  data-testid="token-expire-input"
                />
              </div>
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
                  disabled={creating || !name.trim() || !expiresAt}
                  data-testid="token-submit-btn"
                >
                  {creating ? "Creating..." : "Create"}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
