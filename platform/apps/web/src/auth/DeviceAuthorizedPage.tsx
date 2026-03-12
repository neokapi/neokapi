import { Card, CardContent } from "@neokapi/ui";

function CheckCircleIcon() {
  return (
    <svg
      className="h-12 w-12 text-emerald-500"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <circle cx="12" cy="12" r="10" />
      <path d="m9 12 2 2 4-4" />
    </svg>
  );
}

export function DeviceAuthorizedPage() {
  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-md">
        <Card className="glass-surface">
          <CardContent className="flex flex-col items-center text-center gap-4 py-10">
            <CheckCircleIcon />
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
