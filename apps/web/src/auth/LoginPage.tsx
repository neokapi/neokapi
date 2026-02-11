import { useState } from "react";

export function LoginPage() {
  const [serverUrl, setServerUrl] = useState("");

  const handleLogin = () => {
    const base = serverUrl || window.location.origin;
    window.location.href = `${base}/api/v1/auth/callback`;
  };

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        height: "100vh",
        flexDirection: "column",
        gap: 24,
        background: "var(--bg-primary)",
        color: "var(--text-primary)",
      }}
    >
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          gap: 24,
          padding: 48,
          borderRadius: 12,
          background: "var(--bg-secondary)",
          border: "1px solid var(--border)",
          minWidth: 360,
        }}
      >
        <h1 style={{ fontSize: 32, fontWeight: 700, margin: 0 }}>gokapi</h1>
        <p
          style={{
            color: "var(--text-secondary)",
            fontSize: 14,
            margin: 0,
            textAlign: "center",
          }}
        >
          Sign in to your workspace
        </p>
        <button
          onClick={handleLogin}
          style={{
            width: "100%",
            padding: "12px 32px",
            fontSize: 15,
            fontWeight: 600,
            backgroundColor: "var(--btn-primary-bg)",
            color: "#fff",
            border: "none",
            borderRadius: 6,
            cursor: "pointer",
            transition: "background-color 0.15s",
          }}
          onMouseEnter={(e) =>
            (e.currentTarget.style.backgroundColor = "var(--btn-primary-hover)")
          }
          onMouseLeave={(e) =>
            (e.currentTarget.style.backgroundColor = "var(--btn-primary-bg)")
          }
        >
          Sign in with SSO
        </button>
        <p
          style={{
            color: "var(--text-secondary)",
            fontSize: 12,
            margin: 0,
            textAlign: "center",
          }}
        >
          You will be redirected to your identity provider
        </p>
      </div>
    </div>
  );
}
