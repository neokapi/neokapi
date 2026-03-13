import { Card, CardContent, CardHeader, Button, Input, Label } from "@neokapi/ui";

export function DeviceVerifyPage() {
  const params = new URLSearchParams(window.location.search);
  const prefillCode = params.get("user_code") ?? "";
  const errorMsg = params.get("error") ?? "";

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-md">
        <Card className="glass-surface">
          <CardHeader className="text-center space-y-1 pb-2">
            <h1 className="text-2xl font-semibold tracking-tight">Authorize Device</h1>
            <p className="text-sm text-muted-foreground">Enter the code shown in your terminal</p>
          </CardHeader>
          <CardContent>
            {errorMsg && (
              <div className="mb-4 rounded-md bg-destructive/10 border border-destructive/20 px-4 py-3 text-sm text-destructive text-center">
                {errorMsg}
              </div>
            )}
            <form method="POST" action="/api/v1/auth/device/verify" className="flex flex-col gap-4">
              <div className="space-y-2">
                <Label htmlFor="user_code">Device Code</Label>
                <Input
                  id="user_code"
                  name="user_code"
                  placeholder="xxxx-xxxx"
                  defaultValue={prefillCode}
                  required
                  autoFocus
                  autoComplete="off"
                  className="text-center font-mono text-lg tracking-[0.25em]"
                />
              </div>
              <Button type="submit" size="lg" className="w-full">
                Authorize
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
