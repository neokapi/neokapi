import { useState } from "react";

export function LoginPage() {
  const [serverUrl, setServerUrl] = useState("");

  const handleLogin = () => {
    // For browser-based login, redirect to the OIDC callback endpoint.
    // The server will redirect to Dex for authentication, then back to
    // /api/v1/auth/callback with the authorization code.
    // After exchange, the server redirects to /?token=...&user=...
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
        background: "#0d1117",
        color: "#e6edf3",
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
          background: "#161b22",
          border: "1px solid #30363d",
          minWidth: 360,
        }}
      >
        <h1 style={{ fontSize: 32, fontWeight: 700, margin: 0 }}>gokapi</h1>
        <p
          style={{
            color: "#8b949e",
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
            backgroundColor: "#238636",
            color: "#fff",
            border: "none",
            borderRadius: 6,
            cursor: "pointer",
            transition: "background-color 0.15s",
          }}
          onMouseEnter={(e) =>
            (e.currentTarget.style.backgroundColor = "#2ea043")
          }
          onMouseLeave={(e) =>
            (e.currentTarget.style.backgroundColor = "#238636")
          }
        >
          Sign in with SSO
        </button>
        <p
          style={{
            color: "#484f58",
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
