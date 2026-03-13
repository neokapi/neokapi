import type { KcContext } from "../KcContext";
import type { I18n } from "../i18n";
import { Card, CardHeader, CardContent, CardFooter } from "@neokapi/ui/components/ui/card";
import { Button } from "@neokapi/ui/components/ui/button";
import { ArrowLeft } from "lucide-react";

export default function ErrorPage(props: {
  kcContext: Extract<KcContext, { pageId: "error.ftl" }>;
  i18n: I18n;
}) {
  const { kcContext, i18n } = props;
  const { message, client } = kcContext;
  const { msg } = i18n;

  return (
    <div className="w-full max-w-md px-4">
      <Card className="glass-surface">
        <CardHeader className="text-center space-y-1 pb-2">
          <h1 className="text-2xl font-semibold tracking-tight">{msg("errorTitle")}</h1>
        </CardHeader>
        <CardContent>
          {message && (
            <div
              className="rounded-md bg-destructive/15 px-3 py-2 text-sm text-destructive"
              dangerouslySetInnerHTML={{ __html: message.summary }}
            />
          )}
        </CardContent>
        {client?.baseUrl && (
          <CardFooter className="justify-center">
            <Button
              variant="secondary"
              icon={ArrowLeft}
              onClick={() => {
                window.location.href = client.baseUrl!;
              }}
            >
              {msg("backToApplication")}
            </Button>
          </CardFooter>
        )}
      </Card>
    </div>
  );
}
