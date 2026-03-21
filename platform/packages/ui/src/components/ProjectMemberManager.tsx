import { useState, useEffect, useCallback } from "react";
import type { ProjectMembership, RoleTemplate, Workspace } from "../types/api";
import { useApi } from "../context/ApiContext";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Badge } from "./ui/badge";
import { Card } from "./ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "./ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "./ui/select";
import { Alert, AlertDescription } from "./ui/alert";
import { Users, UserPlus, Trash2, Pencil } from "./icons";

interface ProjectMemberManagerProps {
  workspace: Workspace;
  projectId: string;
  projectLanguages: string[];
}

export function ProjectMemberManager({ workspace, projectId, projectLanguages }: ProjectMemberManagerProps) {
  const api = useApi();
  const [members, setMembers] = useState<ProjectMembership[]>([]);
  const [roleTemplates, setRoleTemplates] = useState<RoleTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [showDialog, setShowDialog] = useState(false);
  const [editingMember, setEditingMember] = useState<ProjectMembership | null>(null);
  const [userId, setUserId] = useState("");
  const [roleId, setRoleId] = useState("");
  const [selectedLanguages, setSelectedLanguages] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const loadMembers = useCallback(async () => {
    try {
      const list = await api.listProjectMembers(workspace.slug, projectId);
      setMembers(list);
    } catch {
      setMembers([]);
    } finally {
      setLoading(false);
    }
  }, [api, workspace.slug, projectId]);

  const loadRoleTemplates = useCallback(async () => {
    try {
      const list = await api.listRoleTemplates(workspace.slug);
      setRoleTemplates(list);
    } catch {
      setRoleTemplates([]);
    }
  }, [api, workspace.slug]);

  useEffect(() => {
    void loadMembers();
    void loadRoleTemplates();
  }, [loadMembers, loadRoleTemplates]);

  const handleSave = async () => {
    if (!editingMember && !userId.trim()) return;
    if (!roleId) return;
    setSaving(true);
    setError("");
    try {
      if (editingMember) {
        const updated = await api.updateProjectMember(workspace.slug, projectId, editingMember.user_id, {
          role_id: roleId,
          languages: selectedLanguages.length > 0 ? selectedLanguages : undefined,
        });
        setMembers((prev) => prev.map((m) => (m.user_id === editingMember.user_id ? updated : m)));
      } else {
        const added = await api.addProjectMember(workspace.slug, projectId, {
          user_id: userId.trim(),
          role_id: roleId,
          languages: selectedLanguages.length > 0 ? selectedLanguages : undefined,
        });
        setMembers((prev) => [added, ...prev]);
      }
      resetDialog();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to save member");
    } finally {
      setSaving(false);
    }
  };

  const handleRemove = async (memberId: string) => {
    try {
      await api.removeProjectMember(workspace.slug, projectId, memberId);
      setMembers((prev) => prev.filter((m) => m.user_id !== memberId));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to remove member");
    }
  };

  const handleEdit = (member: ProjectMembership) => {
    setEditingMember(member);
    setUserId(member.user_id);
    setRoleId(member.role_id);
    setSelectedLanguages(member.languages ?? []);
    setError("");
    setShowDialog(true);
  };

  const handleOpenAdd = () => {
    resetDialog();
    setShowDialog(true);
  };

  const resetDialog = () => {
    setEditingMember(null);
    setUserId("");
    setRoleId("");
    setSelectedLanguages([]);
    setError("");
    setShowDialog(false);
  };

  const handleDialogChange = (open: boolean) => {
    if (!open) {
      resetDialog();
    }
    setShowDialog(open);
  };

  const toggleLanguage = (lang: string) => {
    setSelectedLanguages((prev) =>
      prev.includes(lang) ? prev.filter((l) => l !== lang) : [...prev, lang],
    );
  };

  const getRoleName = (rId: string) => {
    const tmpl = roleTemplates.find((r) => r.id === rId);
    return tmpl?.display_name ?? tmpl?.name ?? rId;
  };

  const canManage = workspace.role === "owner" || workspace.role === "admin";

  if (!canManage) return null;

  return (
    <>
      <Card className="p-6" data-testid="project-member-manager">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h3 className="text-lg font-semibold flex items-center gap-2">
              <Users className="h-4 w-4" />
              Project Members
            </h3>
            <p className="text-[13px] text-muted-foreground mt-1">
              Manage members and their roles for this project
            </p>
          </div>
          <Button
            size="sm"
            onClick={handleOpenAdd}
            data-testid="project-member-add-btn"
          >
            <UserPlus className="h-4 w-4 mr-1" />
            Add Member
          </Button>
        </div>

        {error && (
          <Alert variant="destructive" className="mb-4">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {/* Member list */}
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading members...</div>
        ) : members.length === 0 ? (
          <div className="py-8 text-center text-sm text-muted-foreground">
            No project members
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    User
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Role
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Languages
                  </th>
                  <th className="px-4 py-2.5 text-sm font-medium text-muted-foreground w-[100px]">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody data-testid="project-member-list">
                {members.map((member) => (
                  <tr
                    key={member.user_id}
                    className="border-b border-border/50 transition-colors hover:bg-accent/50"
                  >
                    <td className="px-4 py-2.5 text-sm font-medium">
                      {member.user?.name || member.user?.email || member.user_id}
                    </td>
                    <td className="px-4 py-2.5">
                      <Badge variant="secondary" className="text-xs">
                        {getRoleName(member.role_id)}
                      </Badge>
                    </td>
                    <td className="px-4 py-2.5">
                      {member.languages && member.languages.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {member.languages.map((lang) => (
                            <Badge key={lang} variant="outline" className="text-xs">
                              {lang}
                            </Badge>
                          ))}
                        </div>
                      ) : (
                        <span className="text-xs text-muted-foreground">All languages</span>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex gap-1 justify-end">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0"
                          onClick={() => handleEdit(member)}
                          title="Edit member"
                          data-testid="project-member-edit-btn"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0 text-destructive hover:text-destructive"
                          onClick={() => handleRemove(member.user_id)}
                          title="Remove member"
                          data-testid="project-member-remove-btn"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Add/Edit member dialog */}
      <Dialog open={showDialog} onOpenChange={handleDialogChange}>
        <DialogContent
          className="sm:max-w-[480px]"
          onInteractOutside={(e: Event) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>{editingMember ? "Edit Member" : "Add Member"}</DialogTitle>
          </DialogHeader>

          <div className="flex flex-col gap-4 py-2">
            <div>
              <Label className="text-muted-foreground">User ID</Label>
              <Input
                value={userId}
                onChange={(e) => setUserId(e.target.value)}
                placeholder="user-id"
                disabled={!!editingMember}
                autoFocus={!editingMember}
                className="mt-1"
                data-testid="project-member-userid-input"
              />
            </div>
            <div>
              <Label className="text-muted-foreground">Role</Label>
              <Select value={roleId} onValueChange={setRoleId}>
                <SelectTrigger className="mt-1" data-testid="project-member-role-select">
                  <SelectValue placeholder="Select a role" />
                </SelectTrigger>
                <SelectContent>
                  {roleTemplates.map((tmpl) => (
                    <SelectItem key={tmpl.id} value={tmpl.id}>
                      {tmpl.display_name || tmpl.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label className="text-muted-foreground">Languages</Label>
              <p className="text-xs text-muted-foreground mt-0.5 mb-2">
                Leave empty for access to all languages.
              </p>
              <div className="flex flex-wrap gap-1.5">
                {projectLanguages.map((lang) => {
                  const active = selectedLanguages.includes(lang);
                  return (
                    <Badge
                      key={lang}
                      variant={active ? "default" : "outline"}
                      className="cursor-pointer text-xs select-none"
                      onClick={() => toggleLanguage(lang)}
                      data-testid={`project-member-lang-${lang}`}
                    >
                      {lang}
                    </Badge>
                  );
                })}
              </div>
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
              onClick={handleSave}
              disabled={saving || (!editingMember && !userId.trim()) || !roleId}
              data-testid="project-member-save-btn"
            >
              {saving ? "Saving..." : editingMember ? "Update" : "Add Member"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
