import type { Decorator } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { qk } from "../../lib/queryKeys";

/** A short catalog of the major tags the backend lists, for the language pickers. */
export const KNOWN_LOCALES = [
  { code: "en", display_name: "English" },
  { code: "en-US", display_name: "American English" },
  { code: "de", display_name: "German" },
  { code: "fr", display_name: "French" },
  { code: "es", display_name: "Spanish" },
  { code: "ja", display_name: "Japanese" },
  { code: "nb", display_name: "Norwegian Bokmål" },
  { code: "ar", display_name: "Arabic" },
];

/**
 * The language pickers read the known locales through react-query. Storybook
 * has no backend, so this seeds the cache and the pickers offer languages.
 */
export const WithKnownLocales: Decorator = (Story) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, staleTime: Infinity } },
  });
  client.setQueryData(qk.knownLocales(), KNOWN_LOCALES);
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
};
