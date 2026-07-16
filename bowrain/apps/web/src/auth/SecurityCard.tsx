import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Button,
  Alert,
  AlertDescription,
  ErrorNotice,
  Badge,
  ConfirmDialog,
  useApi,
  type Passkey,
} from "@neokapi/ui";
import { KeyRound, Trash2, Plus, ExternalLink, Loader2 } from "lucide-react";
import {
  decodeCreationOptions,
  encodeAttestation,
  isWebAuthnSupported,
  isReauthRequired,
} from "./webauthn";

const LOGIN_URL = "/api/v1/auth/login";

/**
 * SecurityCard — self-service passkey (WebAuthn) management.
 *
 * The server relays the WebAuthn ceremony: it mints a short-lived,
 * user-scoped identity-provider access token from a retained refresh token and
 * calls the provider's WebAuthn APIs. The browser only runs
 * navigator.credentials.create() on the relayed challenge — no provider token
 * is ever exposed here (BFF invariant).
 *
 * On identity providers that manage credentials through their own account
 * console (Keycloak, self-host), the card links out to that console instead.
 */
export function SecurityCard() {
  const api = useApi();
  const queryClient = useQueryClient();

  const { data: security, isLoading: securityLoading } = useQuery({
    queryKey: ["account-security"],
    queryFn: () => api.getAccountSecurity(),
    retry: false,
  });

  const inApp = security?.in_app === true;

  const {
    data: passkeyData,
    isLoading: passkeysLoading,
    error: passkeysError,
  } = useQuery({
    queryKey: ["account-passkeys"],
    queryFn: () => api.listPasskeys(),
    enabled: inApp,
    retry: false,
  });

  const [adding, setAdding] = useState(false);
  const [actionError, setActionError] = useState<{ title: string; cause?: unknown } | null>(null);
  const [toDelete, setToDelete] = useState<Passkey | null>(null);
  const [deleting, setDeleting] = useState(false);

  // While the provider capability is resolving, render nothing to avoid a flash.
  if (securityLoading) {
    return null;
  }
  // Feature unavailable on this deployment (e.g. OIDC not configured → 503).
  if (!security) {
    return null;
  }

  // ── Account-console provider (Keycloak): link out. ──────────────────────
  if (!inApp) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Security</CardTitle>
          <CardDescription>Manage passkeys and sign-in methods for your account.</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline" asChild>
            <a href={security.account_url} target="_blank" rel="noopener noreferrer">
              <ExternalLink className="mr-2 h-4 w-4" />
              Manage in your account console
            </a>
          </Button>
        </CardContent>
      </Card>
    );
  }

  // ── In-app manager (Cognito). ───────────────────────────────────────────
  const passkeys = passkeyData?.passkeys ?? [];
  const needsReauth = isReauthRequired(passkeysError);

  const handleAdd = async () => {
    setActionError(null);
    if (!isWebAuthnSupported()) {
      setActionError({ title: "Passkeys aren't supported in this browser." });
      return;
    }
    setAdding(true);
    try {
      const { options, nonce } = await api.passkeyRegisterStart();
      const credential = (await navigator.credentials.create({
        publicKey: decodeCreationOptions(options),
      })) as PublicKeyCredential | null;
      if (!credential) {
        throw new Error("Registration was cancelled.");
      }
      await api.passkeyRegisterFinish({ nonce, attestation: encodeAttestation(credential) });
      void queryClient.invalidateQueries({ queryKey: ["account-passkeys"] });
    } catch (err: unknown) {
      if (isReauthRequired(err)) {
        window.location.href = LOGIN_URL;
        return;
      }
      setActionError({ title: "Couldn't add a passkey", cause: err });
    } finally {
      setAdding(false);
    }
  };

  const handleDelete = async () => {
    if (!toDelete) {
      return;
    }
    setActionError(null);
    setDeleting(true);
    try {
      await api.deletePasskey(toDelete.id);
      setToDelete(null);
      void queryClient.invalidateQueries({ queryKey: ["account-passkeys"] });
    } catch (err: unknown) {
      setToDelete(null);
      if (isReauthRequired(err)) {
        window.location.href = LOGIN_URL;
        return;
      }
      setActionError({ title: "Couldn't remove the passkey", cause: err });
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Security</CardTitle>
        <CardDescription>
          Passkeys let you sign in with your fingerprint, face, or device PIN instead of a password.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {needsReauth ? (
          <Alert>
            <AlertDescription className="flex items-center justify-between gap-3">
              <span>Please sign in again to manage your passkeys.</span>
              <Button size="sm" onClick={() => (window.location.href = LOGIN_URL)}>
                Sign in
              </Button>
            </AlertDescription>
          </Alert>
        ) : passkeysLoading ? (
          <div className="text-muted-foreground flex items-center gap-2 text-sm">
            <Loader2 className="h-4 w-4 animate-spin" /> Loading passkeys…
          </div>
        ) : passkeys.length === 0 ? (
          <div className="text-muted-foreground text-sm">
            No passkeys yet. Add one to sign in without a password.
          </div>
        ) : (
          <ul className="flex flex-col gap-2">
            {passkeys.map((pk) => (
              <li
                key={pk.id}
                className="flex items-center justify-between gap-3 rounded-md border p-3"
              >
                <div className="flex min-w-0 items-center gap-3">
                  <KeyRound className="text-muted-foreground h-4 w-4 shrink-0" />
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{pk.name || "Passkey"}</div>
                    <div className="text-muted-foreground flex flex-wrap items-center gap-1.5 text-xs">
                      {pk.created_at && (
                        <span>Added {new Date(pk.created_at).toLocaleDateString()}</span>
                      )}
                      {(pk.transports ?? []).map((tr) => (
                        <Badge key={tr} variant="secondary">
                          {tr}
                        </Badge>
                      ))}
                    </div>
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setToDelete(pk)}
                  aria-label={`Remove passkey ${pk.name || pk.id}`}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </li>
            ))}
          </ul>
        )}

        {actionError && (
          <ErrorNotice title={actionError.title} error={actionError.cause} variant="inline" />
        )}

        {!needsReauth && (
          <div>
            <Button onClick={handleAdd} disabled={adding}>
              {adding ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Waiting for your device…
                </>
              ) : (
                <>
                  <Plus className="mr-2 h-4 w-4" /> Add passkey
                </>
              )}
            </Button>
          </div>
        )}
      </CardContent>

      <ConfirmDialog
        open={toDelete !== null}
        onOpenChange={(open) => {
          if (!open) setToDelete(null);
        }}
        title="Remove passkey?"
        description={`This removes "${toDelete?.name || "this passkey"}" from your account. You can add it again later.`}
        confirmLabel="Remove"
        variant="destructive"
        loading={deleting}
        onConfirm={handleDelete}
      />
    </Card>
  );
}
