import { useEffect } from "react";
import { useWorkspace, InviteManager } from "@neokapi/ui";

export function SettingsMembersRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Members — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) return null;

  return (
    <div className="mx-auto w-full max-w-3xl py-4">
      <InviteManager workspace={activeWorkspace} />
    </div>
  );
}
