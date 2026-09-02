import { useState } from "react";
import {
  Alert,
  AlertDescription,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  cn,
} from "@neokapi/ui";
import type { PostHogConnectorConfig, PostHogConnectorConfigRequest } from "@neokapi/ui";
import { parsePostHogApiError, type PostHogApiError } from "./posthog-demand-source";

// Connect PostHog — phase 0. The personal API key travels once, over the
// save call, and never comes back: the server stores it sealed and returns
// only a •••• + last-4 mask. "Test & save" is a single server round-trip —
// the server runs a live "SELECT 1" against PostHog before persisting, and
// classifies failures (bad_host / bad_key / bad_project) so the card can
// point at the offending field.

export type PostHogHostChoice = "us" | "eu" | "self-hosted";

export interface PostHogConnectFormValues {
  hostChoice: PostHogHostChoice;
  /** Base URL, only meaningful when hostChoice === "self-hosted". */
  selfHostedUrl: string;
  projectId: string;
  apiKey: string;
}

export interface PostHogConnectFormErrors {
  host?: string;
  projectId?: string;
  apiKey?: string;
  general?: string;
}

/** The `host` config value the server expects ("us" | "eu" | https URL). */
export function hostConfigValue(values: PostHogConnectFormValues): string {
  return values.hostChoice === "self-hosted" ? values.selfHostedUrl.trim() : values.hostChoice;
}

/**
 * Client-side validation — pure, unit-tested. `keyOnFile` relaxes the API-key
 * requirement on re-connect: the client never holds the stored key, so an
 * empty field means "keep it".
 */
export function validateConnectForm(
  values: PostHogConnectFormValues,
  keyOnFile: boolean,
): PostHogConnectFormErrors {
  const errors: PostHogConnectFormErrors = {};
  if (values.hostChoice === "self-hosted") {
    const url = values.selfHostedUrl.trim();
    if (!url) {
      errors.host = "Enter your self-hosted PostHog URL.";
    } else if (!/^https?:\/\/.+/.test(url)) {
      errors.host = "The URL must start with http:// or https://.";
    }
  }
  if (!values.projectId.trim()) {
    errors.projectId = "Enter the PostHog project ID.";
  } else if (!/^\d+$/.test(values.projectId.trim())) {
    errors.projectId = "The project ID is the number from your PostHog project URL.";
  }
  if (!values.apiKey && !keyOnFile) {
    errors.apiKey = "Enter a personal API key (created under PostHog → Settings).";
  }
  return errors;
}

/** Map a classified server error onto the field it belongs to — pure, unit-tested. */
export function serverErrorToFormErrors(err: PostHogApiError): PostHogConnectFormErrors {
  switch (err.code) {
    case "bad_host":
      return { host: err.message };
    case "bad_key":
      return { apiKey: err.message };
    case "bad_project":
      return { projectId: err.message };
    default:
      return { general: err.message };
  }
}

export interface PostHogConnectCardProps {
  /** Existing config, when re-connecting; prefills host/project + key mask. */
  config?: PostHogConnectorConfig | null;
  /**
   * Test & save: resolves with the saved config, rejects with the adapter
   * error (classified via parsePostHogApiError).
   */
  onSave: (req: PostHogConnectorConfigRequest) => Promise<PostHogConnectorConfig>;
  onSaved?: (config: PostHogConnectorConfig) => void;
  onCancel?: () => void;
  className?: string;
}

function initialHostChoice(config?: PostHogConnectorConfig | null): PostHogHostChoice {
  if (config?.host === "us" || config?.host === "eu") return config.host;
  if (config?.host) return "self-hosted";
  return "us";
}

/**
 * Connect card for the PostHog locale-demand source. Renders standalone (in a
 * dialog on the Locale-demand page) — no router or query-client dependency.
 */
export function PostHogConnectCard({
  config,
  onSave,
  onSaved,
  onCancel,
  className,
}: PostHogConnectCardProps) {
  const keyOnFile = !!config?.configured && !!config.api_key_masked;
  const [values, setValues] = useState<PostHogConnectFormValues>({
    hostChoice: initialHostChoice(config),
    selfHostedUrl: config?.host && config.host !== "us" && config.host !== "eu" ? config.host : "",
    projectId: config?.posthog_project_id ?? "",
    apiKey: "",
  });
  const [errors, setErrors] = useState<PostHogConnectFormErrors>({});
  const [saving, setSaving] = useState(false);

  const set = <K extends keyof PostHogConnectFormValues>(
    key: K,
    value: PostHogConnectFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const clientErrors = validateConnectForm(values, keyOnFile);
    setErrors(clientErrors);
    if (Object.keys(clientErrors).length > 0) return;

    setSaving(true);
    try {
      const saved = await onSave({
        host: hostConfigValue(values),
        posthog_project_id: values.projectId.trim(),
        // Empty = keep the stored key (the client never sees it).
        api_key: values.apiKey || undefined,
      });
      onSaved?.(saved);
    } catch (err) {
      setErrors(serverErrorToFormErrors(parsePostHogApiError(err)));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card className={cn("w-full max-w-md", className)} data-testid="posthog-connect-card">
      <CardHeader>
        <CardTitle>Connect PostHog</CardTitle>
        <CardDescription>
          Read-only: language and market demand from your $pageview events. The personal API key
          stays on the server, stored encrypted and never sent back to the browser.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4" noValidate>
          <div className="space-y-1.5">
            <Label htmlFor="posthog-host">PostHog host</Label>
            <Select
              value={values.hostChoice}
              onValueChange={(v) => set("hostChoice", v as PostHogHostChoice)}
            >
              <SelectTrigger id="posthog-host" data-testid="posthog-host-select">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="us">US Cloud (us.posthog.com)</SelectItem>
                <SelectItem value="eu">EU Cloud (eu.posthog.com)</SelectItem>
                <SelectItem value="self-hosted">Self-hosted</SelectItem>
              </SelectContent>
            </Select>
            {values.hostChoice === "self-hosted" && (
              <Input
                aria-label="Self-hosted URL"
                placeholder="https://posthog.example.com"
                value={values.selfHostedUrl}
                onChange={(e) => set("selfHostedUrl", e.target.value)}
                data-testid="posthog-self-hosted-url"
              />
            )}
            {errors.host && (
              <p className="text-xs text-destructive" data-testid="posthog-error-host">
                {errors.host}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="posthog-project-id">Project ID</Label>
            <Input
              id="posthog-project-id"
              inputMode="numeric"
              placeholder="12345"
              value={values.projectId}
              onChange={(e) => set("projectId", e.target.value)}
              data-testid="posthog-project-id"
            />
            {errors.projectId && (
              <p className="text-xs text-destructive" data-testid="posthog-error-project">
                {errors.projectId}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="posthog-api-key">Personal API key</Label>
            <Input
              id="posthog-api-key"
              type="password"
              autoComplete="off"
              placeholder={keyOnFile ? `On file: ${config?.api_key_masked}` : "phx_…"}
              value={values.apiKey}
              onChange={(e) => set("apiKey", e.target.value)}
              data-testid="posthog-api-key"
            />
            <p className="text-xs text-muted-foreground">
              {keyOnFile
                ? "Leave empty to keep the stored key, or paste a new one to rotate it."
                : "Needs Query Read access. Created under PostHog → Settings → Personal API keys."}
            </p>
            {errors.apiKey && (
              <p className="text-xs text-destructive" data-testid="posthog-error-key">
                {errors.apiKey}
              </p>
            )}
          </div>

          {errors.general && (
            <Alert variant="destructive" data-testid="posthog-error-general">
              <AlertDescription>{errors.general}</AlertDescription>
            </Alert>
          )}

          <div className="flex justify-end gap-2">
            {onCancel && (
              <Button type="button" variant="ghost" onClick={onCancel} disabled={saving}>
                Cancel
              </Button>
            )}
            <Button type="submit" disabled={saving} data-testid="posthog-test-save">
              {saving ? "Testing connection…" : "Test & save"}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
