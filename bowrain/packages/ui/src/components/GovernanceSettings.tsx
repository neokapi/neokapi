import {
  Badge,
  Button,
  Card,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import { useCallback, useEffect, useState } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { DenyRule, Group, SoDMode } from "../types/api";

const SOD_DESCRIPTIONS: Record<SoDMode, string> = {
  off: "No separation enforced.",
  warn: "Record a warning when someone approves their own work, but allow it.",
  block: "Prevent anyone from reviewing or approving content they authored.",
};

const WORKSPACE_ROLES = ["owner", "admin", "member", "viewer"] as const;

/**
 * GovernanceSettings is the workspace admin surface for the access-governance
 * controls: separation-of-duties policy, teams (groups), deny rules, and
 * workspace role-permission overrides. Admin/owner only (the API enforces it).
 */
export function GovernanceSettings() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  // ── Separation of duties ────────────────────────────────────────────────
  const [sod, setSod] = useState<SoDMode>("warn");
  const [savingSod, setSavingSod] = useState(false);

  // ── Groups ──────────────────────────────────────────────────────────────
  const [groups, setGroups] = useState<Group[]>([]);
  const [newGroup, setNewGroup] = useState("");

  // ── Deny rules ──────────────────────────────────────────────────────────
  const [denyRules, setDenyRules] = useState<DenyRule[]>([]);
  const [denyForm, setDenyForm] = useState({
    subject_type: "user",
    subject_id: "",
    permissions: "",
  });

  // ── Role overrides ──────────────────────────────────────────────────────
  const [overrides, setOverrides] = useState<Record<string, string[]>>({});
  const [overrideEdit, setOverrideEdit] = useState<Record<string, string>>({});

  const [error, setError] = useState("");

  const reload = useCallback(async () => {
    if (!ws) return;
    try {
      const [s, g, d, o] = await Promise.all([
        api.getSoDMode(ws),
        api.listGroups(ws),
        api.listDenyRules(ws),
        api.listRoleOverrides(ws),
      ]);
      setSod(s.mode);
      setGroups(g);
      setDenyRules(d);
      setOverrides(o);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [api, ws]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    if (activeWorkspace) document.title = `Governance — ${activeWorkspace.name} — Bowrain`;
  }, [activeWorkspace]);

  const changeSod = async (mode: SoDMode) => {
    setSavingSod(true);
    setSod(mode);
    try {
      await api.setSoDMode(ws, mode);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSavingSod(false);
    }
  };

  const createGroup = async () => {
    if (!newGroup.trim()) return;
    try {
      await api.createGroup(ws, newGroup.trim());
      setNewGroup("");
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const createDeny = async () => {
    if (!denyForm.subject_id.trim() || !denyForm.permissions.trim()) return;
    try {
      await api.createDenyRule(ws, {
        subject_type: denyForm.subject_type as DenyRule["subject_type"],
        subject_id: denyForm.subject_id.trim(),
        permissions: denyForm.permissions
          .split(",")
          .map((p) => p.trim())
          .filter(Boolean),
      });
      setDenyForm({ subject_type: "user", subject_id: "", permissions: "" });
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const saveOverride = async (role: string) => {
    const perms = (overrideEdit[role] ?? "")
      .split(",")
      .map((p) => p.trim())
      .filter(Boolean);
    try {
      await api.setRoleOverride(ws, role, perms);
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  if (!activeWorkspace) return null;

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 py-4">
      <div>
        <h2 className="text-xl font-semibold">Governance</h2>
        <p className="text-[13px] text-muted-foreground">
          Access controls for collaborative work — teams, deny rules, role tuning, and separation of
          duties.
        </p>
      </div>

      {error && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* Separation of duties */}
      <Card className="p-5">
        <h3 className="text-sm font-semibold">Separation of duties</h3>
        <p className="mb-3 text-[12px] text-muted-foreground">
          Whether a translator may approve (publish) their own work.
        </p>
        <div className="flex items-center gap-3">
          <Select
            value={sod}
            onValueChange={(v) => void changeSod(v as SoDMode)}
            disabled={savingSod}
          >
            <SelectTrigger className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="off">Off</SelectItem>
              <SelectItem value="warn">Warn</SelectItem>
              <SelectItem value="block">Block</SelectItem>
            </SelectContent>
          </Select>
          <span className="text-[12px] text-muted-foreground">{SOD_DESCRIPTIONS[sod]}</span>
        </div>
      </Card>

      {/* Teams (groups) */}
      <Card className="p-5">
        <h3 className="text-sm font-semibold">Teams</h3>
        <p className="mb-3 text-[12px] text-muted-foreground">
          Group members so they can be granted project roles in bulk.
        </p>
        <div className="mb-3 flex gap-2">
          <Input
            placeholder="New team name"
            value={newGroup}
            onChange={(e) => setNewGroup(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && void createGroup()}
            className="max-w-xs"
          />
          <Button size="sm" onClick={() => void createGroup()} disabled={!newGroup.trim()}>
            Add team
          </Button>
        </div>
        {groups.length === 0 ? (
          <p className="text-[12px] text-muted-foreground/60">No teams yet.</p>
        ) : (
          <ul className="divide-y divide-border/30">
            {groups.map((g) => (
              <li key={g.id} className="flex items-center justify-between py-2">
                <span className="text-sm">
                  {g.name}
                  <Badge variant="secondary" className="ml-2 px-1.5 py-0 text-[10px]">
                    {g.member_count ?? 0} members
                  </Badge>
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-destructive"
                  onClick={() => void api.deleteGroup(ws, g.id).then(reload)}
                >
                  Delete
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {/* Deny rules */}
      <Card className="p-5">
        <h3 className="text-sm font-semibold">Deny rules</h3>
        <p className="mb-3 text-[12px] text-muted-foreground">
          Negative permissions that always override grants (subject can be a user, workspace role,
          or group).
        </p>
        <div className="mb-3 flex flex-wrap items-end gap-2">
          <div>
            <Label className="text-[11px]">Subject</Label>
            <Select
              value={denyForm.subject_type}
              onValueChange={(v) => setDenyForm((f) => ({ ...f, subject_type: v }))}
            >
              <SelectTrigger className="w-28">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="user">User</SelectItem>
                <SelectItem value="role">Role</SelectItem>
                <SelectItem value="group">Group</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Input
            placeholder="subject id (user/role/group)"
            value={denyForm.subject_id}
            onChange={(e) => setDenyForm((f) => ({ ...f, subject_id: e.target.value }))}
            className="max-w-[180px]"
          />
          <Input
            placeholder="permissions e.g. manage_tm,review"
            value={denyForm.permissions}
            onChange={(e) => setDenyForm((f) => ({ ...f, permissions: e.target.value }))}
            className="max-w-[240px]"
          />
          <Button size="sm" onClick={() => void createDeny()}>
            Add deny
          </Button>
        </div>
        {denyRules.length === 0 ? (
          <p className="text-[12px] text-muted-foreground/60">No deny rules.</p>
        ) : (
          <ul className="divide-y divide-border/30">
            {denyRules.map((r) => (
              <li key={r.id} className="flex items-center justify-between py-2 text-sm">
                <span>
                  <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                    {r.subject_type}
                  </Badge>{" "}
                  <span className="font-mono">{r.subject_id}</span>
                  <span className="text-muted-foreground"> denied perms {r.denied_perms}</span>
                  {r.project_id && (
                    <span className="text-muted-foreground/70"> · project {r.project_id}</span>
                  )}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-destructive"
                  onClick={() => void api.deleteDenyRule(ws, r.id).then(reload)}
                >
                  Delete
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {/* Role overrides */}
      <Card className="p-5">
        <h3 className="text-sm font-semibold">Workspace role overrides</h3>
        <p className="mb-3 text-[12px] text-muted-foreground">
          Tune the default permissions of a workspace role (leave blank to use the built-in
          default).
        </p>
        <ul className="flex flex-col gap-2">
          {WORKSPACE_ROLES.map((role) => (
            <li key={role} className="flex flex-wrap items-center gap-2">
              <span className="w-20 text-sm capitalize">{role}</span>
              <Input
                placeholder={overrides[role]?.join(",") || "default permissions"}
                value={overrideEdit[role] ?? overrides[role]?.join(",") ?? ""}
                onChange={(e) => setOverrideEdit((o) => ({ ...o, [role]: e.target.value }))}
                className="max-w-md flex-1 font-mono text-[12px]"
              />
              <Button variant="outline" size="sm" onClick={() => void saveOverride(role)}>
                Save
              </Button>
            </li>
          ))}
        </ul>
      </Card>
    </div>
  );
}
