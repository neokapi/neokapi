import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui";
import { Mail } from "lucide-react";
const logoUrl = `${import.meta.env.BASE_URL}logo.png`;

export default function LoginVerifyEmail(props: {
  kcContext: Extract<KcContext, { pageId: "login-verify-email.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, user, message } = kcContext;
  const { msg } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card>
        <CardHeader className="text-center space-y-1 pb-2">
          <div className="flex justify-center mb-2">
            <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-border bg-muted">
              <Mail className="h-6 w-6 text-primary" />
            </div>
          </div>
          <h1 className="text-2xl font-semibold tracking-tight">{msg("emailVerifyTitle")}</h1>
        </CardHeader>
        <CardContent className="space-y-4">
          {message && message.type !== "warning" && (
            <div
              className={`rounded-md px-3 py-2 text-sm ${
                message.type === "error"
                  ? "bg-destructive/15 text-destructive"
                  : message.type === "success"
                    ? "bg-success/15 text-success"
                    : "bg-muted text-muted-foreground"
              }`}
              // Keycloak server-provided message HTML, same pattern as Login.tsx
              dangerouslySetInnerHTML={{ __html: message.summary }}
            />
          )}

          <p className="text-sm text-muted-foreground text-center">
            {msg("emailVerifyInstruction1")}
          </p>

          {user?.email && <p className="text-sm font-medium text-center">{user.email}</p>}

          <p className="text-sm text-muted-foreground text-center">
            {msg("emailVerifyInstruction2")}
            <br />
            <a href={url.loginAction} className="text-primary hover:underline">
              {msg("doClickHere")}
            </a>{" "}
            {msg("emailVerifyInstruction3")}
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
