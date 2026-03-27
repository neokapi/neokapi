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
  Users,
  CircleCheck,
  Loader2,
} from "@neokapi/ui";

/**
 * JoinPage depends on useApi/useAuth/useWorkspace hooks, so we render
 * simplified inline versions of each visual state instead.
 */

function JoinNotAuthenticated() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <div className="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
              <Users className="h-6 w-6 text-primary" />
            </div>
            <CardTitle className="text-xl font-semibold">Join Workspace</CardTitle>
            <CardDescription>
              You have been invited to join a workspace. Sign in to accept the invitation.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button className="w-full" size="lg">
              Sign in to join
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function JoinAccepting() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Join Workspace</CardTitle>
            <CardDescription>Accepting invitation...</CardDescription>
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

function JoinSuccess({ onJoined }: { onJoined: () => void }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <CircleCheck className="mb-2 h-12 w-12 text-emerald-500 dark:text-emerald-400" />
            <CardTitle className="text-xl font-semibold">Joined!</CardTitle>
            <CardDescription>
              You are now a member of <strong>Acme Translations</strong>
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button onClick={onJoined} className="w-full" size="lg">
              Go to workspace
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function JoinError() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <div className="w-full max-w-md">
        <Card>
          <CardHeader className="items-center text-center">
            <CardTitle className="text-xl font-semibold">Join Workspace</CardTitle>
            <CardDescription>Accept this workspace invitation</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Alert variant="destructive">
              <AlertDescription>Invitation has expired or is invalid</AlertDescription>
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

function JoinChecking() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center p-4">
      <p className="mb-6 text-sm font-medium text-muted-foreground">Bowrain</p>
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  );
}

const meta: Meta = {
  title: "Auth/Web/Join",
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj;

export const NotAuthenticated: Story = {
  render: () => <JoinNotAuthenticated />,
};

export const CheckingAuth: Story = {
  render: () => <JoinChecking />,
};

export const Accepting: Story = {
  render: () => <JoinAccepting />,
};

export const Success: Story = {
  render: () => <JoinSuccess onJoined={fn()} />,
};

export const Error: Story = {
  render: () => <JoinError />,
};
