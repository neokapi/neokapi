// Host wrappers over the Office.js Common API. The Common API (getSelectedDataAsync
// / setSelectedDataAsync) works uniformly across Word, Excel, and PowerPoint, so
// one implementation drives all three hosts. Each function resolves to a plain
// value/Promise and tolerates running outside Office (returns empty / no-op) so
// the UI can render in a browser during development.

const TEXT_COERCION = "text"; // Office.CoercionType.Text

function officeReady(): boolean {
  return typeof Office !== "undefined" && !!Office && !!Office.context;
}

/** waitForOffice resolves once the Office host has initialized (or immediately
 *  in a plain browser). */
export async function waitForOffice(): Promise<string | null> {
  if (typeof Office === "undefined" || !Office) return null;
  const info = await Office.onReady();
  return info?.host ?? null;
}

/** getSelectedText returns the text the user has selected in the document. */
export function getSelectedText(): Promise<string> {
  return new Promise((resolve, reject) => {
    if (!officeReady()) {
      resolve("");
      return;
    }
    Office!.context.document.getSelectedDataAsync(TEXT_COERCION, (result) => {
      if (result.status === "succeeded") {
        resolve(result.value ?? "");
      } else {
        reject(new Error(result.error?.message ?? "Could not read selection"));
      }
    });
  });
}

/** replaceSelection overwrites the current selection with new text (e.g. a
 *  translation). */
export function replaceSelection(text: string): Promise<void> {
  return new Promise((resolve, reject) => {
    if (!officeReady()) {
      resolve();
      return;
    }
    Office!.context.document.setSelectedDataAsync(text, (result) => {
      if (result.status === "succeeded") {
        resolve();
      } else {
        reject(new Error(result.error?.message ?? "Could not write to the document"));
      }
    });
  });
}

/** getAccessToken attempts Office SSO (Entra ID) to authenticate the user to the
 *  Bowrain backend. Returns undefined when SSO is unavailable; production
 *  deployments add MSAL nested app authentication as the primary path (README). */
export async function getAccessToken(): Promise<string | undefined> {
  if (!officeReady() || !Office!.auth) return undefined;
  try {
    return await Office!.auth.getAccessToken({ allowSignInPrompt: true });
  } catch {
    return undefined;
  }
}
