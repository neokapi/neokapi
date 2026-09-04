// The workspace's governance: who may decide, and the teams decisions are
// granted to. Admin and owner only; the API enforces it.
//
// Where content sits and what governs it there are project matters, edited on
// the project's settings with the shared governance forms. What lives here is
// the policy that judges a decision: the separation-of-duties mode a promotion
// passes, the permissions a workspace role carries, the deny rules that always
// win, and the teams a project role can be granted to in bulk.

import {
  Badge,
  Button,
  Card,
  CardContent,
  Input,
  Label,
  PageHeader,
  SectionHeading,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import { useCallback, useEffect, useState } from "react";
import { useApi } from "../context/ApiContext";
import { useAnalytics } from "../context/AnalyticsContext";
import { AnalyticsEvents } from "../analytics-events";
import { useWorkspace } from "../context/WorkspaceContext";
import { ErrorNotice } from "../errors";
import { ShieldCheck, Users } from "./icons";
import type { DenyRule, Group, SoDMode } from "../types/api";

const SOD_DESCRIPTIONS: Record<SoDMode, string> = {
  off: "No separation enforced.",
  warn: "Record a warning when someone approves their own work, but allow it.",
  block: "Prevent anyone from reviewing or approving content they authored.",
};

const WORKSPACE_ROLES = ["owner", "admin", "member", "viewer"] as const;

export function GovernanceSettings() {
  const api = useApi();
  const { capture } = useAnalytics();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [sod, setSod] = useState<SoDMode>("warn");
  const [savingSod, setSavingSod] = useState(false);

  const [groups, setGroups] = useState<Group[]>([]);
  const [newGroup, setNewGroup] = useState("");

  const [denyRules, setDenyRules] = useState<DenyRule[]>([]);
  const [denyForm, setDenyForm] = useState({
    subject_type: "user",
    subject_id: "",
    permissions: "",
  });

  const [overrides, setOverrides] = useState<Record<string, string[]>>({});
  const [overrideEdit, setOverrideEdit] = useState<Record<string, string>>({});

  const [error, setError] = useState<{ title: string; cause?: unknown } | null>(null);

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
      setError({ title: "Couldn't load the governance settings", cause: e });
    }
  }, [api, ws]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    if (activeWorkspace) document.title = `Governance · ${activeWorkspace.name} · Bowrain`;
  }, [activeWorkspace]);

  const changeSod = async (mode: SoDMode) => {
    setSavingSod(true);
    setSod(mode);
    try {
      await api.setSoDMode(ws, mode);
      capture(AnalyticsEvents.settingsSaved, { section: "governance" });
    } catch (e) {
      setError({ title: "Couldn't change the separation-of-duties mode", cause: e });
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
      setError({ title: "Couldn't create the team", cause: e });
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
      setError({ title: "Couldn't add the deny rule", cause: e });
    }
  };

  const saveOverride = async (role: string) => {
    const perms = (overrideEdit[role] ?? "")
      .split(",")
      .map((p) => p.trim())
      .filter(Boolean);
    try {
      await api.setRoleOverride(ws, role, perms);
      capture(AnalyticsEvents.settingsSaved, { section: "governance" });
      await reload();
    } catch (e) {
      setError({ title: "Couldn't save the role override", cause: e });
    }
  };

  if (!activeWorkspace) return null;

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 py-4">
      <PageHeader
        title="Governance"
        subtitle="Who may decide on content, and the teams decisions are granted to."
        className="mb-0"
      />

      {error && <ErrorNotice title={error.title} error={error.cause} variant="panel" />}

      {/* Who may decide: the policy a decision passes, and the permissions behind it. */}
      <section>
        <SectionHeading className="mb-3" icon={<ShieldCheck size={14} />}>
          Who may decide
        </SectionHeading>
        <Card>
          <CardContent className="space-y-5 p-4">
            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">
                Separation of duties
              </Label>
              <div className="flex flex-wrap items-center gap-3">
                <Select
                  value={sod}
                  onValueChange={(v) => void changeSod(v as SoDMode)}
                  disabled={savingSod}
                >
                  <SelectTrigger className="w-40" aria-label="Separation of duties">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="off">Off</SelectItem>
                    <SelectItem value="warn">Warn</SelectItem>
                    <SelectItem value="block">Block</SelectItem>
                  </SelectContent>
                </Select>
                <span className="text-xs text-muted-foreground">{SOD_DESCRIPTIONS[sod]}</span>
              </div>
              <p className="mt-1 text-[10px] text-muted-foreground">
                Whether a translator may approve or sign off their own work.
              </p>
            </div>

            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">
                Workspace role overrides
              </Label>
              <ul className="flex flex-col gap-2">
                {WORKSPACE_ROLES.map((role) => (
                  <li key={role} className="flex flex-wrap items-center gap-2">
                    <span className="w-20 text-sm capitalize">{role}</span>
                    <Input
                      placeholder={overrides[role]?.join(",") || "default permissions"}
                      value={overrideEdit[role] ?? overrides[role]?.join(",") ?? ""}
                      aria-label={`${role} permissions`}
                      onChange={(e) => setOverrideEdit((o) => ({ ...o, [role]: e.target.value }))}
                      className="max-w-md flex-1 font-mono text-[12px]"
                    />
                    <Button variant="outline" size="sm" onClick={() => void saveOverride(role)}>
                      Save
                    </Button>
                  </li>
                ))}
              </ul>
              <p className="mt-1 text-[10px] text-muted-foreground">
                The permissions a workspace role carries; blank keeps the built-in default.
              </p>
            </div>

            <div>
              <Label className="mb-1 block text-xs text-muted-foreground">Deny rules</Label>
              <div className="mb-2 flex flex-wrap items-end gap-2">
                <Select
                  value={denyForm.subject_type}
                  onValueChange={(v) => setDenyForm((f) => ({ ...f, subject_type: v }))}
                >
                  <SelectTrigger className="w-28" aria-label="Subject">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">User</SelectItem>
                    <SelectItem value="role">Role</SelectItem>
                    <SelectItem value="group">Group</SelectItem>
                  </SelectContent>
                </Select>
                <Input
                  placeholder="subject id (user/role/group)"
                  aria-label="Subject id"
                  value={denyForm.subject_id}
                  onChange={(e) => setDenyForm((f) => ({ ...f, subject_id: e.target.value }))}
                  className="max-w-[180px]"
                />
                <Input
                  placeholder="permissions e.g. manage_tm,review"
                  aria-label="Denied permissions"
                  value={denyForm.permissions}
                  onChange={(e) => setDenyForm((f) => ({ ...f, permissions: e.target.value }))}
                  className="max-w-[240px]"
                />
                <Button size="sm" onClick={() => void createDeny()}>
                  Add deny
                </Button>
              </div>
              {denyRules.length === 0 ? (
                <p className="text-xs text-muted-foreground/60">No deny rules.</p>
              ) : (
                <ul className="divide-y divide-border/30">
                  {denyRules.map((r) => (
                    <li key={r.id} className="flex items-center justify-between py-2 text-sm">
                      <span>
                        <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                          {r.subject_type}
                        </Badge>{" "}
                        <span className="font-mono">{r.subject_id}</span>
                        <span className="text-muted-foreground">
                          {" "}
                          denied perms {r.denied_perms}
                        </span>
                        {r.project_id && (
                          <span className="text-muted-foreground/70">
                            {" "}
                            · project {r.project_id}
                          </span>
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
              <p className="mt-1 text-[10px] text-muted-foreground">
                A deny rule always overrides a grant. Its subject is a user, a workspace role or a
                team.
              </p>
            </div>
          </CardContent>
        </Card>
      </section>

      {/* Teams: members grouped so a project role can be granted in bulk. */}
      <section>
        <SectionHeading className="mb-3" icon={<Users size={14} />} count={groups.length}>
          Teams
        </SectionHeading>
        <Card>
          <CardContent className="space-y-3 p-4">
            <div className="flex gap-2">
              <Input
                placeholder="New team name"
                aria-label="New team name"
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
              <p className="text-xs text-muted-foreground/60">No teams yet.</p>
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
            <p className="text-[10px] text-muted-foreground">
              Group members so they can be granted project roles in bulk.
            </p>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
