import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui/components/ui/card";
import { Button } from "@neokapi/ui/components/ui/button";
import logoUrl from "../assets/logo.png";

export default function LogoutConfirm(props: {
  kcContext: Extract<KcContext, { pageId: "logout-confirm.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, logoutConfirm, client } = kcContext;
  const { msg, msgStr } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card className="glass-surface">
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("logoutConfirmTitle")}</h1>
          <p className="text-sm text-muted-foreground">{msg("logoutConfirmHeader")}</p>
        </CardHeader>
        <CardContent>
          <form action={url.logoutConfirmAction} method="post" className="space-y-4">
            <input type="hidden" name="session_code" value={logoutConfirm.code} />
            <Button type="submit" name="confirmLogout" className="w-full">
              {msgStr("doLogout")}
            </Button>
          </form>
        </CardContent>
        {!logoutConfirm.skipLink && client?.baseUrl && (
          <CardFooter className="justify-center">
            <p className="text-sm text-muted-foreground">
              <a href={client.baseUrl} className="text-primary hover:underline">
                {msg("backToApplication")}
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
