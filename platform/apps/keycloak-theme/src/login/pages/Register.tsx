import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import type { LucideIcon } from "lucide-react";
import { Mail, User, ArrowLeft } from "lucide-react";
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui";
import { Button } from "@neokapi/ui";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@neokapi/ui";
import { Label } from "@neokapi/ui";

export default function Register(props: {
  kcContext: Extract<KcContext, { pageId: "register.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { url, message, profile, messagesPerField } = kcContext;
  const { msg, msgStr, advancedMsg } = i18n;

  const fieldIcons: Record<string, LucideIcon> = {
    username: User,
    email: Mail,
    firstName: User,
    lastName: User,
  };

  return (
    <div className="w-full max-w-md px-4">
      <Card>
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("registerTitle")}</h1>
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
              // Keycloak server-rendered message HTML — trusted content from the IdP server.
              dangerouslySetInnerHTML={{ __html: message.summary }}
            />
          )}

          <form action={url.registrationAction} method="post" className="space-y-4">
            {Object.values(profile.attributesByName).map((attribute) => {
              if (attribute.name === "password" || attribute.name === "password-confirm") {
                return null;
              }
              const displayName = attribute.displayName
                ? advancedMsg(attribute.displayName)
                : attribute.name;
              const Icon = fieldIcons[attribute.name];
              return (
                <div key={attribute.name} className="space-y-2">
                  <Label htmlFor={attribute.name}>
                    {displayName}
                    {attribute.required && <span className="text-destructive ml-1">*</span>}
                  </Label>
                  {Icon ? (
                    <InputGroup>
                      <InputGroupAddon>
                        <Icon className="size-4" />
                      </InputGroupAddon>
                      <InputGroupInput
                        id={attribute.name}
                        name={attribute.name}
                        type={attribute.name === "email" ? "email" : "text"}
                        defaultValue={attribute.value ?? ""}
                        readOnly={attribute.readOnly}
                        autoComplete={attribute.name}
                        aria-invalid={messagesPerField.existsError(attribute.name)}
                      />
                    </InputGroup>
                  ) : (
                    <InputGroupInput
                      id={attribute.name}
                      name={attribute.name}
                      type={attribute.name === "email" ? "email" : "text"}
                      defaultValue={attribute.value ?? ""}
                      readOnly={attribute.readOnly}
                      autoComplete={attribute.name}
                      aria-invalid={messagesPerField.existsError(attribute.name)}
                    />
                  )}
                  {messagesPerField.existsError(attribute.name) && (
                    <p className="text-xs text-destructive">
                      {messagesPerField.get(attribute.name)}
                    </p>
                  )}
                </div>
              );
            })}

            <div className="flex gap-3 pt-2">
              <Button
                type="button"
                variant="secondary"
                className="flex-1"
                onClick={() => {
                  window.location.href = url.loginUrl;
                }}
              >
                <ArrowLeft />
                {msg("backToLogin")}
              </Button>
              <Button type="submit" className="flex-1">
                {msgStr("doRegister")}
              </Button>
            </div>
          </form>
        </CardContent>
        <CardFooter />
      </Card>
    </div>
  );
}
