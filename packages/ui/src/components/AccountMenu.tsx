import { useState } from "react";
import type { User } from "../types/api";

interface AccountMenuProps {
  user: User;
  onSignOut: () => void;
}

export function AccountMenu({ user, onSignOut }: AccountMenuProps) {
  const [open, setOpen] = useState(false);

  return (
    <div style={{ position: "relative" }}>
      <button
        onClick={() => setOpen(!open)}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "6px 12px",
          background: "none",
          border: "1px solid var(--border)",
          borderRadius: 8,
          cursor: "pointer",
          color: "var(--text-primary)",
        }}
      >
        <div
          style={{
            width: 28,
            height: 28,
            borderRadius: "50%",
            backgroundColor: "var(--accent, #4A90D9)",
            color: "#fff",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            fontSize: 12,
            fontWeight: 700,
            backgroundImage: user.avatar_url ? `url(${user.avatar_url})` : undefined,
            backgroundSize: "cover",
          }}
        >
          {!user.avatar_url && (user.name || "?")[0].toUpperCase()}
        </div>
        <span style={{ fontSize: 13 }}>{user.name || user.email}</span>
      </button>

      {open && (
        <div
          style={{
            position: "absolute",
            top: "100%",
            right: 0,
            marginTop: 4,
            backgroundColor: "var(--bg-secondary)",
            border: "1px solid var(--border)",
            borderRadius: 8,
            padding: 4,
            minWidth: 160,
            zIndex: 100,
            boxShadow: "0 4px 12px rgba(0,0,0,0.15)",
          }}
        >
          <div style={{ padding: "8px 12px", fontSize: 12, color: "var(--text-secondary)" }}>
            {user.email}
          </div>
          <hr style={{ border: "none", borderTop: "1px solid var(--border)", margin: "4px 0" }} />
          <button
            onClick={() => { onSignOut(); setOpen(false); }}
            style={{
              width: "100%",
              padding: "8px 12px",
              background: "none",
              border: "none",
              textAlign: "left",
              cursor: "pointer",
              fontSize: 13,
              color: "var(--text-primary)",
              borderRadius: 4,
            }}
          >
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}
