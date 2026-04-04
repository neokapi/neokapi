import {
  Alert,
  AlertDescription,
  Badge,
  Button,
  Card,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import { useState, useEffect, useCallback } from "react";
import type { Invite, Workspace } from "../types/api";
import { useApi } from "../context/ApiContext";
import { UserPlus, Trash2, Copy, Clock } from "./icons";

interface InviteManagerProps {
  workspace: Workspace;
}

export function InviteManager({ workspace }: InviteManagerProps) {
  const api = useApi();
  const [invites, setInvites] = useState<Invite[]>([]);
  const [loading, setLoading] = useState(true);
  const [showInviteDialog, setShowInviteDialog] = useState(false);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState<string | null>(null);

  const loadInvites = useCallback(async () => {
    try {
      const list = await api.listInvites(workspace.slug);
      setInvites(list);
    } catch {
      setInvites([]);
    } finally {
      setLoading(false);
    }
  }, [api, workspace.slug]);

  useEffect(() => {
    void loadInvites();
  }, [loadInvites]);

  const handleCreate = async () => {
    if (!email.trim()) return;
    setCreating(true);
    setError("");
    try {
      const invite = await api.createInvite(workspace.slug, email.trim(), role, 1);
      setInvites((prev) => [invite, ...prev]);
      setEmail("");
      setRole("member");
      setShowInviteDialog(false);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to create invite");
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (inviteId: string) => {
    try {
      await api.deleteInvite(workspace.slug, inviteId);
      setInvites((prev) => prev.filter((i) => i.id !== inviteId));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to revoke invite");
    }
  };

  const handleCopyLink = (code: string) => {
    const url = `${window.location.origin}/join/${code}`;
    void navigator.clipboard.writeText(url);
    setCopied(code);
    setTimeout(() => setCopied(null), 2000);
  };

  const handleDialogChange = (open: boolean) => {
    if (!open) {
      setEmail("");
      setRole("member");
      setError("");
    }
    setShowInviteDialog(open);
  };

  const isExpired = (inv: Invite) => new Date(inv.expires_at) < new Date();
  const isUsedUp = (inv: Invite) => inv.max_uses > 0 && inv.use_count >= inv.max_uses;

  const canManage = workspace.role === "owner" || workspace.role === "admin";

  if (!canManage) return null;

  return (
    <>
      <Card className="p-6" data-testid="invite-manager">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h3 className="text-lg font-semibold flex items-center gap-2">
              <UserPlus className="h-4 w-4" />
              Invitations
            </h3>
            <p className="text-[13px] text-muted-foreground mt-1">
              Invite members to this workspace
            </p>
          </div>
          <Button
            size="sm"
            onClick={() => setShowInviteDialog(true)}
            data-testid="invite-open-dialog-btn"
          >
            Invite
          </Button>
        </div>

        {error && (
          <Alert variant="destructive" className="mb-4">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {/* Invite list */}
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading invites...</div>
        ) : invites.length === 0 ? (
          <div className="py-8 text-center text-sm text-muted-foreground">
            No pending invitations
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Recipient
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Role
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Expires
                  </th>
                  <th className="px-4 py-2.5 text-sm font-medium text-muted-foreground w-[100px]">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody data-testid="invite-list">
                {invites.map((inv) => {
                  const expired = isExpired(inv);
                  const usedUp = isUsedUp(inv);
                  const inactive = expired || usedUp;

                  return (
                    <tr
                      key={inv.id}
                      className={`border-b border-border/50 transition-colors hover:bg-accent/50 ${inactive ? "opacity-50" : ""}`}
                    >
                      <td className="px-4 py-2.5 text-sm font-medium">
                        {inv.email || "Anyone with link"}
                        {inv.max_uses > 0 && (
                          <span className="text-xs text-muted-foreground ml-2">
                            ({inv.use_count}/{inv.max_uses} used)
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-2.5">
                        <Badge variant={inactive ? "outline" : "secondary"} className="text-xs">
                          {inv.role}
                        </Badge>
                      </td>
                      <td className="px-4 py-2.5 text-sm text-muted-foreground">
                        <span className="inline-flex items-center gap-1.5">
                          <Clock className="h-3 w-3" />
                          {expired ? "Expired" : new Date(inv.expires_at).toLocaleDateString()}
                        </span>
                      </td>
                      <td className="px-4 py-2.5">
                        <div className="flex gap-1 justify-end">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 w-7 p-0"
                            onClick={() => handleCopyLink(inv.code)}
                            title="Copy invite link"
                            data-testid="invite-copy-link-btn"
                          >
                            {copied === inv.code ? (
                              <span className="text-xs text-success">OK</span>
                            ) : (
                              <Copy className="h-3.5 w-3.5" />
                            )}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 w-7 p-0 text-destructive hover:text-destructive"
                            onClick={() => handleDelete(inv.id)}
                            title="Revoke invite"
                            data-testid="invite-revoke-btn"
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
      </Card>

      {/* Invite dialog */}
      <Dialog open={showInviteDialog} onOpenChange={handleDialogChange}>
        <DialogContent
          className="sm:max-w-[480px]"
          onInteractOutside={(e: Event) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>Invite Member</DialogTitle>
          </DialogHeader>

          <div className="flex flex-col gap-4 py-2">
            <div>
              <Label className="text-muted-foreground">Email</Label>
              <Input
                value={email}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setEmail(e.target.value)}
                placeholder="colleague@example.com"
                autoFocus
                className="mt-1"
                data-testid="invite-email-input"
                onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) =>
                  e.key === "Enter" && handleCreate()
                }
              />
            </div>
            <div>
              <Label className="text-muted-foreground">Role</Label>
              <Select value={role} onValueChange={setRole}>
                <SelectTrigger className="mt-1" data-testid="invite-role-select">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="member">Member</SelectItem>
                  <SelectItem value="admin">Admin</SelectItem>
                  <SelectItem value="viewer">Viewer</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => handleDialogChange(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleCreate}
              disabled={creating || !email.trim()}
              data-testid="invite-submit-btn"
            >
              {creating ? "Sending..." : "Send Invite"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
