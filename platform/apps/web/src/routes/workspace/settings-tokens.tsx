import { useEffect } from "react";
import { useWorkspace, ApiTokenManager } from "@neokapi/ui";

export function SettingsTokensRoute() {
  const { activeWorkspace } = useWorkspace();

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `API Tokens — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  if (!activeWorkspace) return null;

  return (
    <div className="mx-auto w-full max-w-3xl py-4">
      <ApiTokenManager workspace={activeWorkspace} />
    </div>
  );
}
