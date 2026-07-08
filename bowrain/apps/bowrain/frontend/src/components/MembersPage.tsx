import { useCallback, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useApi,
  useWorkspace,
  InviteManager,
  Card,
  Badge,
  Button,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Alert,
  AlertDescription,
} from "@neokapi/ui";
import type { Membership } from "@neokapi/ui";
import { Trash2, Users } from "lucide-react";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";

const ROLES = ["owner", "admin", "member", "viewer"] as const;

/**
 * MembersPage is the desktop governance surface for workspace membership. It
 * mirrors the web members route — reusing the shared InviteManager for pending
 * invitations — and adds a members roster with inline role changes and removal,
 * driven through the WailsApiAdapter (which proxies the server's REST
 * /members governance endpoints).
 */
export function MembersPage() {
  const api = useApi();
  const qc = useQueryClient();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  // Members are a server/team feature — the personal (disconnected) workspace
  // has none, so the query stays disabled there.
  const isTeam = !!ws && activeWorkspace?.type !== "personal";

  const [busyUser, setBusyUser] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const canManage = activeWorkspace?.role === "owner" || activeWorkspace?.role === "admin";

  const membersQuery = useQuery({
    queryKey: qk.members(ws),
    enabled: isTeam,
    queryFn: () => api.listMembers(ws),
  });

  // Membership changes pushed from the server (working-copy freshness).
  useInvalidateOnEvent("membership-changed", [qk.members(ws)]);

  const members = membersQuery.data ?? [];
  const loading = isTeam && membersQuery.isLoading;
  const error =
    actionError ??
    (membersQuery.error
      ? membersQuery.error instanceof Error
        ? membersQuery.error.message
        : "Failed to load members"
      : null);

  const handleRoleChange = useCallback(
    async (userId: string, role: string) => {
      setBusyUser(userId);
      setActionError(null);
      try {
        await api.updateMemberRole(ws, userId, role);
        // Optimistically reflect the new role in the cached roster.
        qc.setQueryData<Membership[]>(qk.members(ws), (prev) =>
          (prev ?? []).map((m) =>
            m.user_id === userId ? { ...m, role: role as Membership["role"] } : m,
          ),
        );
      } catch (e) {
        setActionError(e instanceof Error ? e.message : "Failed to update role");
      } finally {
        setBusyUser(null);
      }
    },
    [api, ws, qc],
  );

  const handleRemove = useCallback(
    async (userId: string) => {
      setBusyUser(userId);
      setActionError(null);
      try {
        await api.removeMember(ws, userId);
        // Optimistically drop the removed member from the cached roster.
        qc.setQueryData<Membership[]>(qk.members(ws), (prev) =>
          (prev ?? []).filter((m) => m.user_id !== userId),
        );
      } catch (e) {
        setActionError(e instanceof Error ? e.message : "Failed to remove member");
      } finally {
        setBusyUser(null);
      }
    },
    [api, ws, qc],
  );

  if (!activeWorkspace || activeWorkspace.type === "personal") {
    return (
      <div className="p-6 text-sm text-muted-foreground" data-testid="members-empty">
        Connect to a Bowrain server and select a team workspace to manage members.
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6 p-6" data-testid="members-page">
      <header className="space-y-1">
        <h1 className="flex items-center gap-2 text-xl font-semibold">
          <Users className="h-5 w-5" /> Members
        </h1>
        <p className="text-sm text-muted-foreground">
          Manage who can access {activeWorkspace.name} and what they can do.
        </p>
      </header>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card className="p-6">
        <h3 className="mb-4 text-lg font-semibold">Workspace members</h3>
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading members…</div>
        ) : members.length === 0 ? (
          <div
            className="py-8 text-center text-sm text-muted-foreground"
            data-testid="members-list-empty"
          >
            No members yet.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Member
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Role
                  </th>
                  {canManage && (
                    <th className="w-[60px] px-4 py-2.5 text-sm font-medium text-muted-foreground">
                      Actions
                    </th>
                  )}
                </tr>
              </thead>
              <tbody data-testid="members-list">
                {members.map((m) => (
                  <tr
                    key={m.user_id}
                    className="border-b border-border/50 transition-colors hover:bg-accent/50"
                  >
                    <td className="px-4 py-2.5 text-sm">
                      <div className="font-medium">
                        {m.user?.name || m.user?.email || m.user_id}
                      </div>
                      {m.user?.email && m.user?.name && (
                        <div className="text-xs text-muted-foreground">{m.user.email}</div>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      {canManage && m.role !== "owner" ? (
                        <Select
                          value={m.role}
                          onValueChange={(v: string) => handleRoleChange(m.user_id, v)}
                          disabled={busyUser === m.user_id}
                        >
                          <SelectTrigger
                            className="h-8 w-[130px]"
                            data-testid={`member-role-${m.user_id}`}
                          >
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {ROLES.filter((r) => r !== "owner").map((r) => (
                              <SelectItem key={r} value={r}>
                                {r}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      ) : (
                        <Badge variant="secondary" className="capitalize">
                          {m.role}
                        </Badge>
                      )}
                    </td>
                    {canManage && (
                      <td className="px-4 py-2.5">
                        {m.role !== "owner" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 w-7 p-0 text-destructive hover:text-destructive"
                            disabled={busyUser === m.user_id}
                            onClick={() => handleRemove(m.user_id)}
                            title="Remove member"
                            data-testid={`member-remove-${m.user_id}`}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        )}
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <InviteManager workspace={activeWorkspace} />
    </div>
  );
}
