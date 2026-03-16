import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui/components/ui/card";
import { Button } from "@neokapi/ui/components/ui/button";
const logoUrl = `${import.meta.env.BASE_URL}logo.png`;

export default function LoginIdpLinkConfirm(props: {
  kcContext: Extract<KcContext, { pageId: "login-idp-link-confirm.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, idpAlias } = kcContext;
  const { msg, msgStr } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card>
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("confirmLinkIdpTitle")}</h1>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground text-center">
            {msg("confirmLinkIdpReviewProfile", idpAlias)}
          </p>

          <form action={url.loginAction} method="post" className="space-y-2">
            <Button type="submit" name="submitAction" value="updateProfile" className="w-full">
              {msgStr("confirmLinkIdpReviewProfile")}
            </Button>
            <Button
              type="submit"
              name="submitAction"
              value="linkAccount"
              variant="secondary"
              className="w-full"
            >
              {msgStr("confirmLinkIdpContinue", idpAlias)}
            </Button>
          </form>
        </CardContent>
        <CardFooter />
      </Card>
    </div>
  );
}

function BowrainLogo() {
  return <img src={logoUrl} width="48" height="48" alt="Bowrain" className="rounded-xl" />;
}
