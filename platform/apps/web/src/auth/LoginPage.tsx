import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle, Button } from "@neokapi/ui";

export function LoginPage() {
  const [serverUrl, _setServerUrl] = useState("");

  const handleLogin = () => {
    const base = serverUrl || window.location.origin;
    window.location.href = `${base}/api/v1/auth/login`;
  };

  return (
    <div className="flex items-center justify-center h-screen flex-col gap-6 text-foreground">
      <Card className="min-w-[360px] glass-surface">
        <CardHeader className="items-center text-center">
          <CardTitle className="text-3xl font-bold">neokapi</CardTitle>
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
