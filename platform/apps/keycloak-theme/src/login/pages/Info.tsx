import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui/components/ui/card";
import { Button } from "@neokapi/ui/components/ui/button";
import { ArrowLeft } from "lucide-react";
const logoUrl = `${import.meta.env.BASE_URL}logo.png`;

export default function Info(props: {
  kcContext: Extract<KcContext, { pageId: "info.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { message, messageHeader, requiredActions, pageRedirectUri, actionUri, skipLink, client } =
    kcContext;
  const { msg, msgStr, advancedMsg } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card>
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">
            {messageHeader ?? msg("emailForgotTitle")}
          </h1>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Keycloak server-provided message HTML, same pattern as Login.tsx / Error.tsx */}
          <div
            className={`rounded-md px-3 py-2 text-sm ${
              message.type === "error"
                ? "bg-destructive/15 text-destructive"
                : message.type === "warning"
                  ? "bg-amber-500/15 text-amber-400"
                  : message.type === "success"
                    ? "bg-success/15 text-success"
                    : "bg-muted text-muted-foreground"
            }`}
            dangerouslySetInnerHTML={{ __html: message.summary }}
          />

          {requiredActions && requiredActions.length > 0 && (
            <div className="text-sm text-muted-foreground">
              <span className="font-medium">{msg("requiredAction" as any)}: </span>
              {requiredActions.map((action, i) => (
                <span key={action}>
                  {advancedMsg(`requiredAction.${action}`)}
                  {i < requiredActions.length - 1 && ", "}
                </span>
              ))}
            </div>
          )}
        </CardContent>
        <CardFooter className="justify-center gap-2">
          {!skipLink && (
            <>
              {pageRedirectUri ? (
                <Button asChild>
                  <a href={pageRedirectUri}>{msg("backToApplication")}</a>
                </Button>
              ) : actionUri ? (
                <Button asChild>
                  <a href={actionUri}>{msgStr("proceedWithAction")}</a>
                </Button>
              ) : client?.baseUrl ? (
                <Button variant="secondary" asChild>
                  <a href={client.baseUrl}>
                    <ArrowLeft />
                    {msg("backToApplication")}
                  </a>
                </Button>
              ) : null}
            </>
          )}
        </CardFooter>
      </Card>
    </div>
  );
}

function BowrainLogo() {
  return <img src={logoUrl} width="48" height="48" alt="Bowrain" className="rounded-xl" />;
}
