/**
 * NetworkError is a request that never reached the server: the connection
 * failed, so there is no status and no answer to show. The REST adapter
 * throws it for a failed fetch, and parseAppError renders it the same way as
 * the desktop's report of a request that could not be sent.
 */
export class NetworkError extends Error {
  constructor(cause: unknown, message = "The server could not be reached") {
    super(message);
    this.name = "NetworkError";
    // Assigned directly: this lib target's Error constructor takes no options.
    (this as { cause?: unknown }).cause = cause;
  }
}

/**
 * The error a failed fetch rejects with, as the adapter's callers see it. A
 * request the caller aborted stays an AbortError, because it was stopped on
 * purpose and is not a failure to report.
 */
export function asNetworkError(e: unknown): unknown {
  if (e instanceof NetworkError) return e;
  if (e instanceof DOMException && e.name === "AbortError") return e;
  if (e instanceof Error && e.name === "AbortError") return e;
  return new NetworkError(e);
}
