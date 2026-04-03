import { Card, CardContent, CircleCheck } from "@neokapi/ui";

export function DeviceAuthorizedPage() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardContent className="flex flex-col items-center text-center gap-4 py-10">
            <CircleCheck className="h-12 w-12 text-emerald-500 dark:text-emerald-400" />
            <h1 className="text-2xl font-semibold tracking-tight">Device Authorized!</h1>
            <p className="text-sm text-muted-foreground">
              You can close this window and return to your terminal.
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
