// WebAuthn ceremony helpers.
//
// The server relays the WebAuthn registration ceremony (BFF invariant: no
// identity-provider token reaches the browser). It returns standard
// PublicKeyCredentialCreationOptions with base64url-encoded binary fields; the
// browser needs those as ArrayBuffers to call navigator.credentials.create(),
// and the resulting attestation must be re-encoded to base64url JSON to post
// back. These are the two conversions — no library needed.

/** Decode a base64url string to an ArrayBuffer. */
export function base64urlToArrayBuffer(value: string): ArrayBuffer {
  const base64 = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = base64.padEnd(Math.ceil(base64.length / 4) * 4, "=");
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
}

/** Encode an ArrayBuffer as an unpadded base64url string. */
export function arrayBufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (const b of bytes) {
    binary += String.fromCharCode(b);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

// The server's creation options are opaque JSON to the rest of the app; these
// narrow types describe only the binary fields we must convert.
interface CreationOptionsJSON {
  challenge: string;
  user: { id: string; name: string; displayName: string };
  excludeCredentials?: Array<{ id: string; type: string; transports?: string[] }>;
  [key: string]: unknown;
}

/**
 * Convert the server's PublicKeyCredentialCreationOptionsJSON into the browser
 * shape (ArrayBuffer challenge / user.id / excludeCredentials[].id), passing
 * every other field through untouched.
 */
export function decodeCreationOptions(options: unknown): PublicKeyCredentialCreationOptions {
  const o = options as CreationOptionsJSON;
  return {
    ...(o as unknown as PublicKeyCredentialCreationOptions),
    challenge: base64urlToArrayBuffer(o.challenge),
    user: {
      ...o.user,
      id: base64urlToArrayBuffer(o.user.id),
    },
    excludeCredentials: (o.excludeCredentials ?? []).map((c) => ({
      id: base64urlToArrayBuffer(c.id),
      type: c.type as PublicKeyCredentialType,
      ...(c.transports ? { transports: c.transports as AuthenticatorTransport[] } : {}),
    })),
  };
}

/**
 * Serialize the credential returned by navigator.credentials.create() into the
 * RegistrationResponseJSON shape the server relays back to the identity
 * provider (base64url binary fields).
 */
export function encodeAttestation(credential: PublicKeyCredential): unknown {
  const response = credential.response as AuthenticatorAttestationResponse;
  const attestation: Record<string, unknown> = {
    id: credential.id,
    rawId: arrayBufferToBase64url(credential.rawId),
    type: credential.type,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      clientDataJSON: arrayBufferToBase64url(response.clientDataJSON),
      attestationObject: arrayBufferToBase64url(response.attestationObject),
      ...(typeof response.getTransports === "function"
        ? { transports: response.getTransports() }
        : {}),
    },
  };
  const attachment = credential.authenticatorAttachment;
  if (attachment) {
    attestation.authenticatorAttachment = attachment;
  }
  return attestation;
}

/** Feature-detect platform WebAuthn support. */
export function isWebAuthnSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.PublicKeyCredential !== "undefined" &&
    typeof navigator.credentials?.create === "function"
  );
}

/** The server's step-up entry point (top-level GET navigation). */
const ELEVATE_URL = "/api/v1/account/security/elevate";

/**
 * True when the thrown fetch error signals the 409 elevation_required contract:
 * credential management needs a fresh, self-service-scoped token, obtained via a
 * step-up re-authentication.
 */
export function isElevationRequired(err: unknown): boolean {
  return err instanceof Error && err.message.includes("elevation_required");
}

/**
 * Send the browser through a step-up re-auth. This is a full-page navigation to
 * the server, which redirects to the identity provider and back to `returnPath`
 * (defaults to the current path) with ?elevated=1 on success (0 on failure).
 */
export function beginElevation(returnPath?: string): void {
  const ret = returnPath ?? window.location.pathname;
  window.location.href = `${ELEVATE_URL}?return=${encodeURIComponent(ret)}`;
}
