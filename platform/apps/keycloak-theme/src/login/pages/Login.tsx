import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { Lock, Mail, User } from "lucide-react";
const logoUrl = `${import.meta.env.BASE_URL}logo.png`;
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui";
import { Button } from "@neokapi/ui";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@neokapi/ui";
import { Label } from "@neokapi/ui";

export default function Login(props: {
  kcContext:
    | Extract<KcContext, { pageId: "login.ftl" }>
    | Extract<KcContext, { pageId: "login-username.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, realm, login, messagesPerField, message } = kcContext;
  const isFullLogin = kcContext.pageId === "login.ftl";
  const social = (kcContext as any).social as
    | {
        displayInfo?: boolean;
        providers?: { loginUrl: string; alias: string; providerId: string; displayName: string }[];
      }
    | undefined;
  const enableWebAuthnConditionalUI = (kcContext as any).enableWebAuthnConditionalUI as
    | boolean
    | undefined;
  const { msg, msgStr } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <div className="flex justify-center mb-8">
        <BowrainLogo />
      </div>
      <Card>
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("loginAccountTitle")}</h1>
          <p className="text-sm text-muted-foreground">
            {msg("loginTitleHtml", realm.displayNameHtml || realm.displayName)}
          </p>
        </CardHeader>
        <CardContent>
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
              <Label htmlFor="username">
                {!realm.loginWithEmailAllowed
                  ? msg("username")
                  : !realm.registrationEmailAsUsername
                    ? msg("usernameOrEmail")
                    : msg("email")}
              </Label>
              <InputGroup>
                <InputGroupAddon>
                  {realm.registrationEmailAsUsername ? (
                    <Mail className="size-4" />
                  ) : (
                    <User className="size-4" />
                  )}
                </InputGroupAddon>
                <InputGroupInput
                  id="username"
                  name="username"
                  type="text"
                  autoFocus
                  autoComplete={enableWebAuthnConditionalUI ? "username webauthn" : "username"}
                  defaultValue={login?.username ?? ""}
                  aria-invalid={messagesPerField.existsError("username")}
                />
              </InputGroup>
              {messagesPerField.existsError("username") && (
                <p className="text-xs text-destructive">{messagesPerField.get("username")}</p>
              )}
            </div>

            {isFullLogin && (
              <div className="space-y-2">
                <Label htmlFor="password">{msg("password")}</Label>
                <InputGroup>
                  <InputGroupAddon>
                    <Lock className="size-4" />
                  </InputGroupAddon>
                  <InputGroupInput
                    id="password"
                    name="password"
                    type="password"
                    autoComplete="current-password"
                    tabIndex={2}
                    aria-invalid={messagesPerField.existsError("password")}
                  />
                </InputGroup>
                {messagesPerField.existsError("password") && (
                  <p className="text-xs text-destructive">{messagesPerField.get("password")}</p>
                )}
                {realm.resetPasswordAllowed && (
                  <div className="text-right">
                    <a
                      href={url.loginResetCredentialsUrl}
                      className="text-xs text-primary hover:underline"
                      tabIndex={5}
                    >
                      {msg("doForgotPassword")}
                    </a>
                  </div>
                )}
              </div>
            )}

            {realm.rememberMe && (
              <div className="flex items-center gap-2">
                <input
                  id="rememberMe"
                  name="rememberMe"
                  type="checkbox"
                  defaultChecked={!!login?.rememberMe}
                  className="h-4 w-4 rounded border-border"
                  tabIndex={3}
                />
                <Label htmlFor="rememberMe" className="text-sm font-normal">
                  {msg("rememberMe")}
                </Label>
              </div>
            )}

            <Button id="kc-login" type="submit" className="w-full" tabIndex={4}>
              {msgStr("doLogIn")}
            </Button>
          </form>

          {social?.providers && social.providers.length > 0 && (
            <div className="mt-6">
              <div className="relative flex items-center gap-4 my-2">
                <div className="flex-1 h-px bg-gradient-to-r from-transparent via-border to-transparent" />
                <span className="text-xs font-medium tracking-wide text-muted-foreground">
                  or sign in with
                </span>
                <div className="flex-1 h-px bg-gradient-to-r from-transparent via-border to-transparent" />
              </div>
              <div className="mt-4 flex justify-center gap-6">
                {social.providers.map((provider) => (
                  <a
                    key={provider.alias}
                    href={provider.loginUrl}
                    className="group flex flex-col items-center gap-2 transition-all duration-300"
                  >
                    <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-border bg-muted transition-all duration-300 group-hover:border-border group-hover:bg-accent">
                      <SocialIcon alias={provider.alias} />
                    </div>
                    <span className="text-xs text-muted-foreground group-hover:text-foreground transition-colors duration-300">
                      {provider.displayName.replace(/^Sign in with /i, "")}
                    </span>
                  </a>
                ))}
              </div>
            </div>
          )}
        </CardContent>
        {realm.registrationAllowed && (
          <CardFooter className="justify-center">
            <p className="text-sm text-muted-foreground">
              {msg("noAccount")}{" "}
              <a href={url.registrationUrl} className="text-primary hover:underline" tabIndex={6}>
                {msg("doRegister")}
              </a>
            </p>
          </CardFooter>
        )}
      </Card>
    </div>
  );
}

function SocialIcon({ alias }: { alias: string }) {
  const cls = "w-5 h-5";
  switch (alias) {
    case "github":
      return (
        <svg className={cls} viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z" />
        </svg>
      );
    case "google":
      return (
        <svg className={cls} viewBox="0 0 24 24" fill="currentColor">
          <path
            d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 01-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"
            fill="#4285F4"
          />
          <path
            d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
            fill="#34A853"
          />
          <path
            d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"
            fill="#FBBC05"
          />
          <path
            d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
            fill="#EA4335"
          />
        </svg>
      );
    case "microsoft":
      return (
        <svg className={cls} viewBox="0 0 24 24" fill="currentColor">
          <path d="M1 1h10.5v10.5H1z" fill="#F25022" />
          <path d="M12.5 1H23v10.5H12.5z" fill="#7FBA00" />
          <path d="M1 12.5h10.5V23H1z" fill="#00A4EF" />
          <path d="M12.5 12.5H23V23H12.5z" fill="#FFB900" />
        </svg>
      );
    case "apple":
      return (
        <svg className={cls} viewBox="0 0 24 24" fill="currentColor">
          <path d="M17.05 20.28c-.98.95-2.05.88-3.08.4-1.09-.5-2.08-.48-3.24 0-1.44.62-2.2.44-3.06-.4C2.79 15.25 3.51 7.59 9.05 7.31c1.35.07 2.29.74 3.08.8 1.18-.24 2.31-.93 3.57-.84 1.51.12 2.65.72 3.4 1.8-3.12 1.87-2.38 5.98.48 7.13-.57 1.5-1.31 2.99-2.54 4.09zM12.03 7.25c-.15-2.23 1.66-4.07 3.74-4.25.29 2.58-2.34 4.5-3.74 4.25z" />
        </svg>
      );
    default:
      return (
        <svg
          className={cls}
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <circle cx="12" cy="12" r="10" />
          <path d="M2 12h20M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10 15.3 15.3 0 014-10z" />
        </svg>
      );
  }
}

function BowrainLogo() {
  return <img src={logoUrl} width="48" height="48" alt="Bowrain" className="rounded-xl" />;
}
