import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle, Button } from "@gokapi/ui";

export function LoginPage() {
  const [serverUrl, setServerUrl] = useState("");

  const handleLogin = () => {
    // For browser-based login, redirect to the OIDC callback endpoint.
    // The server will redirect to Dex for authentication, then back to
    // /api/v1/auth/callback with the authorization code.
    // After exchange, the server redirects to /?token=...&user=...
    const base = serverUrl || window.location.origin;
    window.location.href = `${base}/api/v1/auth/login`;
  };

  return (
    <div className="flex items-center justify-center h-screen flex-col gap-6 bg-background text-foreground">
      <Card className="min-w-[360px]">
        <CardHeader className="items-center text-center">
          <CardTitle className="text-3xl font-bold">gokapi</CardTitle>
          <p className="text-sm text-muted-foreground">
            Sign in to your workspace
          </p>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <Button onClick={handleLogin} className="w-full" size="lg">
            Sign in with SSO
          </Button>
          <p className="text-xs text-muted-foreground text-center">
            You will be redirected to your identity provider
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
