import { useCallback } from "react";
import { useError } from "../components/ErrorBanner";

/**
 * Hook that wraps async operations with error handling.
 * Returns a function that catches errors and shows them via ErrorContext.
 *
 * Usage:
 *   const run = useAsyncAction();
 *   const handleSave = () => run("Saving project", async () => {
 *     await api.saveProject(tabID);
 *   });
 */
export function useAsyncAction() {
  const { showError } = useError();

  const run = useCallback(
    async <T>(
      label: string,
      fn: () => Promise<T>,
      onError?: (err: unknown) => void,
    ): Promise<T | undefined> => {
      try {
        return await fn();
      } catch (err) {
        showError(label, err);
        onError?.(err);
        return undefined;
      }
    },
    [showError],
  );

  return run;
}
