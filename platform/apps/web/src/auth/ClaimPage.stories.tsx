import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
  Button,
  Alert,
  AlertDescription,
  Package,
  CircleCheck,
  Loader2,
} from "@neokapi/ui";

/**
 * ClaimPage depends on useApi/useAuth/useWorkspace hooks, so we render
 * simplified inline versions of each visual state instead.
 */

function ClaimNotAuthenticated() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <div className="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
              <Package className="h-6 w-6 text-primary" />
            </div>
            <CardTitle className="text-xl font-semibold">Claim Project</CardTitle>
            <CardDescription>
              Sign in to claim this project and add it to your workspace.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button className="w-full" size="lg">
              Sign in to claim
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function ClaimClaiming() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Claim Project</CardTitle>
            <CardDescription>Claiming project...</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex justify-center">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function ClaimSuccess({ onClaimed }: { onClaimed: () => void }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <CircleCheck className="mb-2 h-12 w-12 text-emerald-500 dark:text-emerald-400" />
            <CardTitle className="text-xl font-semibold">Project Claimed!</CardTitle>
            <CardDescription>
              The project has been added to workspace <strong>acme-translations</strong>.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button onClick={onClaimed} className="w-full" size="lg">
              Go to workspace
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function ClaimError() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Claim Project</CardTitle>
            <CardDescription>Claim this project</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Alert variant="destructive">
              <AlertDescription>Token has expired or is invalid</AlertDescription>
            </Alert>
            <Button className="w-full" size="lg">
              Try again
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function ClaimChecking() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  );
}

const meta: Meta = {
  title: "Auth/Web/Claim",
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj;

export const NotAuthenticated: Story = {
  render: () => <ClaimNotAuthenticated />,
};

export const CheckingAuth: Story = {
  render: () => <ClaimChecking />,
};

export const Claiming: Story = {
  render: () => <ClaimClaiming />,
};

export const Success: Story = {
  render: () => <ClaimSuccess onClaimed={fn()} />,
};

export const Error: Story = {
  render: () => <ClaimError />,
};
