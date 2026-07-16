import { useEffect, useState } from "react";
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
import { KeyRound, Trash2, Plus, ExternalLink, Loader2, ShieldCheck } from "lucide-react";
import {
  decodeCreationOptions,
  encodeAttestation,
  isWebAuthnSupported,
  isElevationRequired,
  beginElevation,
} from "./webauthn";

/**
 * SecurityCard — self-service passkey (WebAuthn) management.
 *
 * The server relays the WebAuthn ceremony (BFF invariant: no identity-provider
 * token reaches the browser). Because Cognito's credential APIs are broadly
 * scoped, managing passkeys requires a step-up ("elevation"): the server sends
 * the browser through a fresh re-authentication that yields a short-lived,
 * self-service-scoped token it holds server-side. Until the user elevates, the
 * section is gated behind a "Confirm your identity" prompt.
 *
 * On identity providers that manage credentials through their own account
 * console (Keycloak, self-host), the card links out to that console instead.
 */
export function SecurityCard() {
  const api = useApi();
  const queryClient = useQueryClient();

  // Surface the outcome of a step-up round-trip (?elevated=1|0), then strip the
  // param so a refresh doesn't re-show it.
  const [elevateFailed, setElevateFailed] = useState(false);
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const outcome = params.get("elevated");
    if (outcome === "1" || outcome === "0") {
      setElevateFailed(outcome === "0");
      params.delete("elevated");
      const qs = params.toString();
      window.history.replaceState(null, "", window.location.pathname + (qs ? `?${qs}` : ""));
    }
  }, []);

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
  const needsElevation = isElevationRequired(passkeysError);

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
      // The elevation window can expire mid-flow — send the user back through it.
      if (isElevationRequired(err)) {
        beginElevation();
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
      if (isElevationRequired(err)) {
        beginElevation();
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
        {elevateFailed && (
          <Alert variant="destructive">
            <AlertDescription>
              We couldn&rsquo;t confirm your identity. Please try again.
            </AlertDescription>
          </Alert>
        )}
        {needsElevation ? (
          <Alert>
            <AlertDescription className="flex items-center justify-between gap-3">
              <span>Confirm your identity to view and manage your sign-in methods.</span>
              <Button size="sm" onClick={() => beginElevation()}>
                <ShieldCheck className="mr-2 h-4 w-4" />
                Confirm identity
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

        {!needsElevation && (
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
