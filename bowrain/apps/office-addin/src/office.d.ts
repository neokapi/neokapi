// Minimal ambient declarations for the slice of the Office JavaScript API
// (Office.js) this add-in uses. Office.js is loaded from the Microsoft CDN via a
// <script> tag in index.html and exposed as the global `Office`; we declare only
// the Common API surface we call so the task pane type-checks without pulling in
// the full @types/office-js package.
//
// Production note: for richer host-specific edits (Word.run/Excel.run comments,
// critiques, track-changes) and Entra SSO via MSAL nested app authentication,
// add @types/office-js and @azure/msal-browser. See README.md.

export {};

declare global {
  interface OfficeAsyncResult<T> {
    status: "succeeded" | "failed";
    value: T;
    error?: { name: string; message: string; code: number };
  }

  interface OfficeDocument {
    getSelectedDataAsync(
      coercionType: string,
      callback: (result: OfficeAsyncResult<string>) => void,
    ): void;
    setSelectedDataAsync(data: string, callback?: (result: OfficeAsyncResult<void>) => void): void;
  }

  interface OfficeContext {
    document: OfficeDocument;
    host?: string;
  }

  interface OfficeAuth {
    getAccessToken(options?: { allowSignInPrompt?: boolean }): Promise<string>;
  }

  interface OfficeNamespace {
    onReady(
      callback?: (info: { host: string; platform: string }) => void,
    ): Promise<{ host: string; platform: string }>;
    context: OfficeContext;
    auth?: OfficeAuth;
    CoercionType: { Text: string; Html: string };
    HostType: { Word: string; Excel: string; PowerPoint: string };
  }

  // Present only inside the Office host runtime.
  const Office: OfficeNamespace | undefined;
}
