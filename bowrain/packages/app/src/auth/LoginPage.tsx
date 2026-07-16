import { useState } from "react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  LogIn,
  TEST_IDS,
} from "@neokapi/ui";

export function LoginPage() {
  const [serverUrl, _setServerUrl] = useState("");

  const handleLogin = () => {
    const base = serverUrl || window.location.origin;
    window.location.href = `${base}/api/v1/auth/login`;
  };

  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <div className="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
              <LogIn className="h-6 w-6 text-primary" />
            </div>
            <CardTitle className="text-2xl font-bold">Welcome to Bowrain</CardTitle>
            <CardDescription>Govern and steward your team's content</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button
              onClick={handleLogin}
              className="w-full"
              size="lg"
              data-testid={TEST_IDS.auth.loginSsoButton}
            >
              Sign in with SSO
            </Button>
            <p className="text-xs text-muted-foreground text-center">
              You will be redirected to your identity provider
            </p>
          </CardContent>
        </Card>
        <p className="mt-4 text-center text-xs text-muted-foreground">Built on kapi</p>
      </div>
    </div>
  );
}
