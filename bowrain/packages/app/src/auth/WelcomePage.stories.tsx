import type { Meta, StoryObj, Decorator } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiProvider, type ApiAdapter, type SlugCheckResponse, type Workspace } from "@neokapi/ui";
import { WelcomePage } from "./WelcomePage";

// Welcome — the first-run handle picker shown after OIDC sign-in. The stories
// fake the onboarding endpoints the page calls (status, slug check, complete).

const personalWorkspace: Workspace = {
  id: "ws-1",
  name: "Ada Lovelace",
  slug: "ada",
  description: "",
  logo_url: "",
  type: "personal",
  role: "owner",
};

function fakeAdapter(overrides: Partial<ApiAdapter> = {}): ApiAdapter {
  return {
    getOnboardingStatus: async () => ({
      needs_onboarding: true,
      email: "ada@example.com",
      display_name: "Ada Lovelace",
      suggested_slug: "ada",
    }),
    checkSlug: async (): Promise<SlugCheckResponse> => ({ available: true }),
    completeOnboarding: async () => personalWorkspace,
    ...overrides,
  } as unknown as ApiAdapter;
}

const withApi = (overrides?: Partial<ApiAdapter>): Decorator => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (Story) => (
    <QueryClientProvider client={queryClient}>
      <ApiProvider adapter={fakeAdapter(overrides)}>
        <Story />
      </ApiProvider>
    </QueryClientProvider>
  );
};

const meta: Meta<typeof WelcomePage> = {
  title: "Auth/Web/Welcome",
  component: WelcomePage,
  parameters: {
    layout: "fullscreen",
  },
  args: { onComplete: () => {} },
};

export default meta;
type Story = StoryObj<typeof WelcomePage>;

/** Suggested handle pre-filled and available. */
export const Default: Story = {
  decorators: [withApi()],
};

/** The suggested handle collides with an active workspace. */
export const HandleTaken: Story = {
  decorators: [withApi({ checkSlug: async () => ({ available: false, reason: "taken" }) })],
};

/** The suggested handle sits inside a recent rename's reservation window. */
export const HandleReserved: Story = {
  decorators: [withApi({ checkSlug: async () => ({ available: false, reason: "reserved" }) })],
};

/** Submit fails server-side (e.g. signups closed) — the error stays on the page. */
export const SubmitFails: Story = {
  decorators: [
    withApi({
      completeOnboarding: async () => {
        await new Promise((r) => setTimeout(r, 300));
        throw new Error('403: {"error":"signups are closed","code":"signups_closed"}');
      },
    }),
  ],
};
