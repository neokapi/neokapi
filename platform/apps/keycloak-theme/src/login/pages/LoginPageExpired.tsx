import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui/components/ui/card";
import logoUrl from "../assets/logo.png";

export default function LoginPageExpired(props: {
  kcContext: Extract<KcContext, { pageId: "login-page-expired.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url } = kcContext;
  const { msg } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card className="glass-surface">
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("pageExpiredTitle")}</h1>
        </CardHeader>
        <CardContent className="space-y-4 text-center">
          <p className="text-sm text-muted-foreground">
            {msg("pageExpiredMsg1")}{" "}
            <a href={url.loginRestartFlowUrl} className="text-primary hover:underline">
              {msg("doClickHere")}
            </a>
          </p>
          <p className="text-sm text-muted-foreground">
            {msg("pageExpiredMsg2")}{" "}
            <a href={url.loginAction} className="text-primary hover:underline">
              {msg("doClickHere")}
            </a>
          </p>
        </CardContent>
        <CardFooter />
      </Card>
    </div>
  );
}

function BowrainLogo() {
  return <img src={logoUrl} width="48" height="48" alt="Bowrain" className="rounded-xl" />;
}
