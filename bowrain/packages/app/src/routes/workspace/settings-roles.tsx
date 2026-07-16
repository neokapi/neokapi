import { useEffect } from "react";
import { useWorkspace, RoleTemplateManager } from "@neokapi/ui";

export function SettingsRolesRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Roles — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) return null;

  return (
    <div className="mx-auto w-full max-w-3xl py-4">
      <RoleTemplateManager workspace={activeWorkspace} />
    </div>
  );
}
