import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui/components/ui/card";
import { Mail } from "lucide-react";
import logoUrl from "../assets/logo.png";

export default function LoginIdpLinkEmail(props: {
  kcContext: Extract<KcContext, { pageId: "login-idp-link-email.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, brokerContext, idpAlias } = kcContext;
  const { msg } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card className="glass-surface">
        <CardHeader className="text-center space-y-1 pb-2">
          <div className="flex justify-center mb-2">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-[var(--semantic-border)] bg-[var(--semantic-surface)]">
              <Mail className="h-6 w-6 text-primary" />
            </div>
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">{msg("emailLinkIdpTitle", idpAlias)}</h1>
        </CardHeader>
        <CardContent className="space-y-4 text-center">
          <p className="text-sm text-muted-foreground">
            {msg("emailLinkIdp1", idpAlias, brokerContext.username)}
          </p>
          <p className="text-sm text-muted-foreground">
            {msg("emailLinkIdp2")}{" "}
            <a href={url.loginAction} className="text-primary hover:underline">
              {msg("doClickHere")}
            </a>{" "}
            {msg("emailLinkIdp3")}
          </p>
          <p className="text-sm text-muted-foreground">
            {msg("emailLinkIdp4")}{" "}
            <a href={url.loginAction} className="text-primary hover:underline">
              {msg("doClickHere")}
            </a>{" "}
            {msg("emailLinkIdp5")}
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
