import { useState, useEffect, useCallback } from "react";
import type { Invite, Workspace } from "../types/api";
import { useApi } from "../context/ApiContext";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Badge } from "./ui/badge";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "./ui/select";
import { UserPlus, Trash2, Copy, Clock } from "./icons";

interface InviteManagerProps {
  workspace: Workspace;
}

export function InviteManager({ workspace }: InviteManagerProps) {
  const api = useApi();
  const [invites, setInvites] = useState<Invite[]>([]);
  const [loading, setLoading] = useState(true);
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
    loadInvites();
  }, [loadInvites]);

  const handleCreate = async () => {
    if (!email.trim()) return;
    setCreating(true);
    setError("");
    try {
      const invite = await api.createInvite(workspace.slug, email.trim(), role, 1);
      setInvites((prev) => [invite, ...prev]);
      setEmail("");
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
    navigator.clipboard.writeText(url);
    setCopied(code);
    setTimeout(() => setCopied(null), 2000);
  };

  const isExpired = (inv: Invite) => new Date(inv.expires_at) < new Date();
  const isUsedUp = (inv: Invite) => inv.max_uses > 0 && inv.use_count >= inv.max_uses;

  const canManage = workspace.role === "owner" || workspace.role === "admin";

  if (!canManage) return null;

  return (
    <div className="mt-6">
      <h3 className="text-sm font-semibold mb-3 flex items-center gap-2">
        <UserPlus className="h-4 w-4" />
        Invitations
      </h3>

      {/* Create invite form */}
      <div className="flex items-end gap-2 mb-4">
        <div className="flex-1">
          <Label className="text-xs text-muted-foreground">Email</Label>
          <Input
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="colleague@example.com"
            className="mt-1"
            onKeyDown={(e) => e.key === "Enter" && handleCreate()}
          />
        </div>
        <div className="w-[120px]">
          <Label className="text-xs text-muted-foreground">Role</Label>
          <Select value={role} onValueChange={setRole}>
            <SelectTrigger className="mt-1">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="member">Member</SelectItem>
              <SelectItem value="admin">Admin</SelectItem>
              <SelectItem value="viewer">Viewer</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <Button onClick={handleCreate} disabled={creating || !email.trim()} size="sm">
          {creating ? "Sending..." : "Invite"}
        </Button>
      </div>

      {error && <div className="text-destructive text-xs mb-3">{error}</div>}

      {/* Invite list */}
      {loading ? (
        <div className="text-xs text-muted-foreground">Loading invites...</div>
      ) : invites.length === 0 ? (
        <div className="text-xs text-muted-foreground">No pending invitations</div>
      ) : (
        <div className="space-y-2">
          {invites.map((inv) => {
            const expired = isExpired(inv);
            const usedUp = isUsedUp(inv);
            const inactive = expired || usedUp;

            return (
              <div
                key={inv.id}
                className={`flex items-center gap-3 rounded-md border border-border px-3 py-2 text-sm ${
                  inactive ? "opacity-50" : ""
                }`}
              >
                <div className="flex-1 min-w-0">
                  <div className="truncate font-medium">
                    {inv.email || "Anyone with link"}
                  </div>
                  <div className="flex items-center gap-2 mt-0.5 text-xs text-muted-foreground">
                    <Clock className="h-3 w-3" />
                    {expired
                      ? "Expired"
                      : `Expires ${new Date(inv.expires_at).toLocaleDateString()}`}
                    {inv.max_uses > 0 && (
                      <span>
                        ({inv.use_count}/{inv.max_uses} used)
                      </span>
                    )}
                  </div>
                </div>
                <Badge variant={inactive ? "outline" : "secondary"} className="text-xs">
                  {inv.role}
                </Badge>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 w-7 p-0"
                  onClick={() => handleCopyLink(inv.code)}
                  title="Copy invite link"
                >
                  {copied === inv.code ? (
                    <span className="text-xs text-green-500">OK</span>
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
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
