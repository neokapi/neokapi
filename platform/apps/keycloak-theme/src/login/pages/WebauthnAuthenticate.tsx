import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
const logoUrl = `${import.meta.env.BASE_URL}logo.png`;
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui/components/ui/card";
import { Button } from "@neokapi/ui/components/ui/button";
import { useScript } from "keycloakify/login/pages/WebauthnAuthenticate.useScript";

export default function WebauthnAuthenticate(props: {
  kcContext: Extract<KcContext, { pageId: "webauthn-authenticate.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, authenticators, shouldDisplayAuthenticators, realm } = kcContext;
  const { msg, msgStr, advancedMsg } = i18n;

  const authButtonId = "authenticateWebAuthnButton";
  useScript({ authButtonId, kcContext, i18n });

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card>
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("webauthn-login-title")}</h1>
          <p className="text-sm text-muted-foreground">
            Use your passkey to sign in.
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Hidden form for authenticator selection (required by useScript) */}
          <form id="authn_select">
            {authenticators.authenticators.map((authenticator, i) => (
              <input type="hidden" name="authn_use_chk" readOnly value={authenticator.credentialId} key={i} />
            ))}
          </form>

          {/* Hidden form for WebAuthn response data */}
          <form id="webauth" action={url.loginAction} method="post">
            <input type="hidden" id="clientDataJSON" name="clientDataJSON" />
            <input type="hidden" id="authenticatorData" name="authenticatorData" />
            <input type="hidden" id="signature" name="signature" />
            <input type="hidden" id="credentialId" name="credentialId" />
            <input type="hidden" id="userHandle" name="userHandle" />
            <input type="hidden" id="error" name="error" />
          </form>

          {/* Display registered authenticators */}
          {shouldDisplayAuthenticators && authenticators !== undefined && authenticators.authenticators.length > 0 && (
            <div className="space-y-2">
              {authenticators.authenticators.map((authenticator, i) => (
                <div
                  key={i}
                  className="flex items-center gap-3 rounded-lg border border-border bg-muted p-3"
                >
                  <div className="flex-1">
                    <div className="text-sm font-medium">
                      {advancedMsg(authenticator.label)}
                    </div>
                    {authenticator.transports?.displayNameProperties !== undefined &&
                      authenticator.transports.displayNameProperties.length !== 0 && (
                        <div className="text-xs text-muted-foreground">
                          {authenticator.transports.displayNameProperties.map((nameProperty, j, arr) => (
                            <span key={j}>
                              {advancedMsg(nameProperty)}
                              {j !== arr.length - 1 && ", "}
                            </span>
                          ))}
                        </div>
                      )}
                    <div className="text-xs text-muted-foreground">
                      {msg("passkey-createdAt-label")} {authenticator.createdAt}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}

          <Button id={authButtonId} type="button" className="w-full" autoFocus>
            {msgStr("webauthn-doAuthenticate")}
          </Button>
        </CardContent>
        {realm.registrationAllowed && (
          <CardFooter className="justify-center">
            <p className="text-sm text-muted-foreground">
              {msg("noAccount")}{" "}
              <a href={url.registrationUrl} className="text-primary hover:underline">
                {msg("doRegister")}
              </a>
            </p>
          </CardFooter>
        )}
      </Card>
    </div>
  );
}

function BowrainLogo() {
  return <img src={logoUrl} width="48" height="48" alt="Bowrain" className="rounded-xl" />;
}
