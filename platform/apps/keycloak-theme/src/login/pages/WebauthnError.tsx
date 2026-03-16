import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui/components/ui/card";
import { Button } from "@neokapi/ui/components/ui/button";
const logoUrl = `${import.meta.env.BASE_URL}logo.png`;

export default function WebauthnError(props: {
  kcContext: Extract<KcContext, { pageId: "webauthn-error.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, message, isAppInitiatedAction } = kcContext;
  const { msg, msgStr } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card>
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("webauthn-error-title")}</h1>
        </CardHeader>
        <CardContent className="space-y-4">
          {message && (
            <div
              className={`rounded-md px-3 py-2 text-sm ${
                message.type === "error"
                  ? "bg-destructive/15 text-destructive"
                  : "bg-muted text-muted-foreground"
              }`}
              // Keycloak server-provided message HTML, same pattern as Error.tsx
              dangerouslySetInnerHTML={{ __html: message.summary }}
            />
          )}

          <form action={url.loginAction} method="post">
            <input type="hidden" id="executionValue" name="authenticationExecution" />
            <input type="hidden" id="isSetRetry" name="isSetRetry" value="retry" />
            <Button type="submit" className="w-full">
              {msgStr("doTryAgain")}
            </Button>
          </form>

          {isAppInitiatedAction && (
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
