import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
const logoUrl = `${import.meta.env.BASE_URL}logo.png`;
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui/components/ui/card";
import { Button } from "@neokapi/ui/components/ui/button";
import { Label } from "@neokapi/ui/components/ui/label";
import { useScript } from "keycloakify/login/pages/WebauthnRegister.useScript";

export default function WebauthnRegister(props: {
  kcContext: Extract<KcContext, { pageId: "webauthn-register.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, isSetRetry, isAppInitiatedAction } = kcContext;
  const { msg, msgStr } = i18n;

  const authButtonId = "authenticateWebAuthnButton";
  useScript({ authButtonId, kcContext, i18n });

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card>
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("webauthn-registration-title")}</h1>
          <p className="text-sm text-muted-foreground">
            Touch your security key or use your device biometrics to register a passkey.
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Hidden form for WebAuthn registration data */}
          <form id="register" action={url.loginAction} method="post">
            <input type="hidden" id="clientDataJSON" name="clientDataJSON" />
            <input type="hidden" id="attestationObject" name="attestationObject" />
            <input type="hidden" id="publicKeyCredentialId" name="publicKeyCredentialId" />
            <input type="hidden" id="authenticatorLabel" name="authenticatorLabel" />
            <input type="hidden" id="transports" name="transports" />
            <input type="hidden" id="error" name="error" />

            {/* Logout other sessions checkbox */}
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="logout-sessions"
                name="logout-sessions"
                value="on"
                defaultChecked
                className="h-4 w-4 rounded border-border"
              />
              <Label htmlFor="logout-sessions" className="text-sm font-normal">
                {msg("logoutOtherSessions")}
              </Label>
            </div>
          </form>

          <Button id={authButtonId} type="button" className="w-full">
            {msgStr("doRegisterSecurityKey")}
          </Button>

          {!isSetRetry && isAppInitiatedAction && (
            <form action={url.loginAction} method="post">
              <Button
                type="submit"
                variant="secondary"
                className="w-full"
                name="cancel-aia"
                value="true"
              >
                {msgStr("doCancel")}
              </Button>
            </form>
          )}
        </CardContent>
        <CardFooter />
      </Card>
    </div>
  );
}

function BowrainLogo() {
  return <img src={logoUrl} width="48" height="48" alt="Bowrain" className="rounded-xl" />;
}
