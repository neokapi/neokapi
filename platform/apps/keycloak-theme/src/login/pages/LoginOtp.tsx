import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { KeyRound } from "lucide-react";
const logoUrl = `${import.meta.env.BASE_URL}logo.png`;
import { Card, CardHeader, CardContent } from "@neokapi/ui/components/ui/card";
import { Button } from "@neokapi/ui/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@neokapi/ui/components/ui/input-group";
import { Label } from "@neokapi/ui/components/ui/label";

export default function LoginOtp(props: {
  kcContext: Extract<KcContext, { pageId: "login-otp.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, messagesPerField, message } = kcContext;
  const { msg, msgStr } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <img src={logoUrl} width="48" height="48" alt="Bowrain" className="rounded-xl" />
      </div>
      <Card>
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("doLogIn")}</h1>
          <p className="text-sm text-muted-foreground">{msg("loginOtpOneTime")}</p>
        </CardHeader>
        <CardContent>
          {/* Keycloak server-rendered message — same pattern as Login.tsx */}
          {message && message.type !== "warning" && (
            <div
              className={`mb-4 rounded-md px-3 py-2 text-sm ${
                message.type === "error"
                  ? "bg-destructive/15 text-destructive"
                  : message.type === "success"
                    ? "bg-success/15 text-success"
                    : "bg-muted text-muted-foreground"
              }`}
              dangerouslySetInnerHTML={{ __html: message.summary }}
            />
          )}

          <form action={url.loginAction} method="post" className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="otp">{msg("loginOtpOneTime")}</Label>
              <InputGroup>
                <InputGroupAddon>
                  <KeyRound className="size-4" />
                </InputGroupAddon>
                <InputGroupInput
                  id="otp"
                  name="otp"
                  type="text"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  autoFocus
                  aria-invalid={messagesPerField.existsError("totp")}
                />
              </InputGroup>
              {messagesPerField.existsError("totp") && (
                <p className="text-xs text-destructive">{messagesPerField.get("totp")}</p>
              )}
            </div>

            <Button id="kc-login" type="submit" className="w-full">
              {msgStr("doLogIn")}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
