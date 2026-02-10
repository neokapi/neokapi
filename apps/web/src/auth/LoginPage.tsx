export function LoginPage() {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        height: "100vh",
        flexDirection: "column",
        gap: 24,
      }}
    >
      <h1 style={{ fontSize: 32, fontWeight: 700 }}>gokapi</h1>
      <p style={{ color: "var(--text-secondary)", fontSize: 16 }}>
        Sign in to continue
      </p>
      <button
        onClick={() => {
          // Redirect to OIDC authorization endpoint.
          // In production, this would be /api/v1/auth/callback
          window.location.href = "/api/v1/auth/callback";
        }}
        style={{
          padding: "12px 32px",
          fontSize: 16,
          fontWeight: 600,
          backgroundColor: "var(--accent, #58a6ff)",
          color: "#fff",
          border: "none",
          borderRadius: 8,
          cursor: "pointer",
        }}
      >
        Sign in with SSO
      </button>
    </div>
  );
}
