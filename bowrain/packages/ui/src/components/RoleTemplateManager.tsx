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
  Textarea,
} from "@neokapi/ui-primitives";
import { useState, useEffect, useCallback } from "react";
import type { RoleTemplate, Workspace, PermissionName } from "../types/api";
import { ALL_PERMISSIONS, PERMISSION_LABELS } from "../types/api";
import { useApi } from "../context/ApiContext";
import { Shield, Pencil, Trash2, Plus } from "./icons";

interface RoleTemplateManagerProps {
  workspace: Workspace;
}

interface RoleFormData {
  name: string;
  display_name: string;
  description: string;
  permissions: PermissionName[];
}

const emptyForm: RoleFormData = {
  name: "",
  display_name: "",
  description: "",
  permissions: [],
};

export function RoleTemplateManager({ workspace }: RoleTemplateManagerProps) {
  const api = useApi();
  const [roles, setRoles] = useState<RoleTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [showDialog, setShowDialog] = useState(false);
  const [editingRole, setEditingRole] = useState<RoleTemplate | null>(null);
  const [form, setForm] = useState<RoleFormData>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const loadRoles = useCallback(async () => {
    try {
      const list = await api.listRoleTemplates(workspace.slug);
      setRoles(list);
    } catch {
      setRoles([]);
    } finally {
      setLoading(false);
    }
  }, [api, workspace.slug]);

  useEffect(() => {
    void loadRoles();
  }, [loadRoles]);

  const handleCreate = () => {
    setEditingRole(null);
    setForm(emptyForm);
    setError("");
    setShowDialog(true);
  };

  const handleEdit = (role: RoleTemplate) => {
    setEditingRole(role);
    setForm({
      name: role.name,
      display_name: role.display_name,
      description: role.description,
      permissions: [...role.permission_names],
    });
    setError("");
    setShowDialog(true);
  };

  const handleDelete = async (roleId: string) => {
    try {
      await api.deleteRoleTemplate(workspace.slug, roleId);
      setRoles((prev) => prev.filter((r) => r.id !== roleId));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to delete role");
    }
  };

  const handleSave = async () => {
    if (!form.name.trim() || !form.display_name.trim()) return;
    setSaving(true);
    setError("");
    try {
      if (editingRole) {
        const updated = await api.updateRoleTemplate(workspace.slug, editingRole.id, {
          name: form.name.trim(),
          display_name: form.display_name.trim(),
          description: form.description.trim(),
          permissions: form.permissions,
        });
        setRoles((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
      } else {
        const created = await api.createRoleTemplate(workspace.slug, {
          name: form.name.trim(),
          display_name: form.display_name.trim(),
          description: form.description.trim(),
          permissions: form.permissions,
        });
        setRoles((prev) => [...prev, created]);
      }
      setShowDialog(false);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to save role");
    } finally {
      setSaving(false);
    }
  };

  const handleDialogChange = (open: boolean) => {
    if (!open) {
      setEditingRole(null);
      setForm(emptyForm);
      setError("");
    }
    setShowDialog(open);
  };

  const togglePermission = (perm: PermissionName) => {
    setForm((prev) => ({
      ...prev,
      permissions: prev.permissions.includes(perm)
        ? prev.permissions.filter((p) => p !== perm)
        : [...prev.permissions, perm],
    }));
  };

  const canManage = workspace.role === "owner" || workspace.role === "admin";

  if (!canManage) return null;

  return (
    <>
      <Card className="p-6" data-testid="role-template-manager">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div>
            <h3 className="text-lg font-semibold flex items-center gap-2">
              <Shield className="h-4 w-4" />
              Role Templates
            </h3>
            <p className="text-[13px] text-muted-foreground mt-1">
              Manage permission bundles for project members
            </p>
          </div>
          <Button size="sm" onClick={handleCreate} data-testid="role-create-btn">
            <Plus className="h-3.5 w-3.5 mr-1" />
            Create Role
          </Button>
        </div>

        {error && (
          <Alert variant="destructive" className="mb-4">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {/* Role list */}
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading roles...</div>
        ) : roles.length === 0 ? (
          <div className="py-8 text-center text-sm text-muted-foreground">
            No role templates defined
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Name
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Description
                  </th>
                  <th className="px-4 py-2.5 text-left text-sm font-medium text-muted-foreground">
                    Permissions
                  </th>
                  <th className="px-4 py-2.5 text-sm font-medium text-muted-foreground w-[100px]">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody data-testid="role-list">
                {roles.map((role) => (
                  <tr
                    key={role.id}
                    className="border-b border-border/50 transition-colors hover:bg-accent/50"
                  >
                    <td className="px-4 py-2.5 text-sm font-medium">
                      {role.display_name}
                      {role.is_builtin && (
                        <Badge variant="outline" className="ml-2 text-xs">
                          built-in
                        </Badge>
                      )}
                    </td>
                    <td className="px-4 py-2.5 text-sm text-muted-foreground">
                      {role.description}
                    </td>
                    <td className="px-4 py-2.5">
                      <Badge variant="secondary" className="text-xs">
                        {role.permission_names.length} permission
                        {role.permission_names.length !== 1 ? "s" : ""}
                      </Badge>
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex gap-1 justify-end">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0"
                          onClick={() => handleEdit(role)}
                          title="Edit role"
                          data-testid="role-edit-btn"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        {!role.is_builtin && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 w-7 p-0 text-destructive hover:text-destructive"
                            onClick={() => handleDelete(role.id)}
                            title="Delete role"
                            data-testid="role-delete-btn"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Create / Edit dialog */}
      <Dialog open={showDialog} onOpenChange={handleDialogChange}>
        <DialogContent
          className="sm:max-w-[540px]"
          onInteractOutside={(e: Event) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>{editingRole ? "Edit Role" : "Create Role"}</DialogTitle>
          </DialogHeader>

          <div className="flex flex-col gap-4 py-2">
            <div>
              <Label className="text-muted-foreground">Name</Label>
              <Input
                value={form.name}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setForm((prev) => ({ ...prev, name: e.target.value }))
                }
                placeholder="translator"
                autoFocus
                className="mt-1"
                data-testid="role-name-input"
              />
            </div>
            <div>
              <Label className="text-muted-foreground">Display Name</Label>
              <Input
                value={form.display_name}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setForm((prev) => ({ ...prev, display_name: e.target.value }))
                }
                placeholder="Translator"
                className="mt-1"
                data-testid="role-display-name-input"
              />
            </div>
            <div>
              <Label className="text-muted-foreground">Description</Label>
              <Textarea
                value={form.description}
                onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
                  setForm((prev) => ({ ...prev, description: e.target.value }))
                }
                placeholder="Describe what this role can do..."
                className="mt-1"
                rows={2}
                data-testid="role-description-input"
              />
            </div>
            <div>
              <Label className="text-muted-foreground">Permissions</Label>
              <div className="grid grid-cols-2 gap-2 mt-2" data-testid="role-permissions-grid">
                {ALL_PERMISSIONS.map((perm) => (
                  <label
                    key={perm}
                    className="flex items-center gap-2 text-sm cursor-pointer rounded px-2 py-1.5 hover:bg-accent/50"
                  >
                    <input
                      type="checkbox"
                      checked={form.permissions.includes(perm)}
                      onChange={() => togglePermission(perm)}
                      className="rounded border-border"
                    />
                    {PERMISSION_LABELS[perm]}
                  </label>
                ))}
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
              disabled={saving || !form.name.trim() || !form.display_name.trim()}
              data-testid="role-save-btn"
            >
              {saving ? "Saving..." : editingRole ? "Save Changes" : "Create Role"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
