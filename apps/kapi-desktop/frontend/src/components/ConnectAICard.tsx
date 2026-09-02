import { useCallback, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, KeyRound, Loader2, Sparkles } from "lucide-react";
import { Button, ErrorNotice, Input, Label, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { AIDetectionResult, DetectedAIProvider } from "../types/api";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";

/** Key-entry providers offered by the "I have an API key" path. */
const KEY_PROVIDERS = [
  { name: "anthropic", label: "Anthropic", model: "" },
  { name: "openai", label: "OpenAI", model: "" },
  { name: "gemini", label: "Gemini", model: "" },
];

export interface ConnectAICardProps {
  /** Pre-loaded detection for Storybook/tests — skips api.detectAIProviders(). */
  detection?: AIDetectionResult;
  /** Called after a provider is configured (any path). */
  onConfigured?: (providerLabel: string) => void;
}

/**
 * First-open "Connect your AI" card: shown when NO provider is configured
 * anywhere. Detected keyless options (Claude Code, Ollama) are one-click
 * primary buttons; "I have an API key" expands the vault form; a quiet link
 * skips to the demo engine. Hides itself once anything is configured.
 */
export function ConnectAICard({ detection: propDetection, onConfigured }: ConnectAICardProps) {
  const [keyOpen, setKeyOpen] = useState(false);
  const [keyProvider, setKeyProvider] = useState(KEY_PROVIDERS[0].name);
  const [apiKey, setApiKey] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [done, setDone] = useState<string | null>(null);

  const detectionQuery = useQuery({
    queryKey: qk.aiDetection(),
    queryFn: () => api.detectAIProviders(),
    enabled: !propDetection,
  });
  const detection: AIDetectionResult | null = propDetection ?? detectionQuery.data ?? null;
  const loaded = !!propDetection || !detectionQuery.isPending;

  const finish = useCallback(
    (label: string) => {
      setDone(label);
      onConfigured?.(label);
    },
    [onConfigured],
  );

  const selectDetected = async (d: DetectedAIProvider) => {
    setBusy(d.provider);
    setError(null);
    try {
      await api.selectAIProvider(d.provider, d.model);
      finish(d.label);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  const saveKey = async () => {
    const provider = KEY_PROVIDERS.find((p) => p.name === keyProvider) ?? KEY_PROVIDERS[0];
    setBusy("key");
    setError(null);
    try {
      await api.saveProvider({
        id: "",
        name: `${provider.label} key`,
        provider_type: provider.name,
        api_key: apiKey,
      });
      await api.selectAIProvider(provider.name, "");
      setApiKey("");
      finish(provider.label);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  const skipToDemo = async () => {
    setBusy("demo");
    setError(null);
    try {
      await api.selectAIProvider("demo", "");
      finish(t("the demo engine"));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  // Nothing to connect: already configured, or still deriving.
  if (!loaded || !detection || (detection.configured && !done)) return null;

  // Configured just now — the success confirmation ("toast" line).
  if (done) {
    return (
      <section
        className="mb-8 rounded-lg border border-green-500/30 bg-green-500/5 p-4"
        data-testid="connect-ai-done"
      >
        <p className="flex items-center gap-2 text-sm font-medium">
          <CheckCircle2 size={16} className="text-green-500" />
          {t("kapi will translate with {provider}", { provider: done })}
        </p>
      </section>
    );
  }

  const detected = detection.detected ?? [];

  return (
    <section
      className="mb-8 rounded-lg border border-primary/30 bg-primary/5 p-5"
      data-testid="connect-ai-card"
    >
      <div className="mb-1 flex items-center gap-2">
        <Sparkles size={16} className="text-primary" />
        <h2 className="text-sm font-semibold">{t("Connect your AI")}</h2>
      </div>
      <p className="mb-4 text-xs text-muted-foreground">
        {t(
          "Translation and checks run through an AI model of your choice. Pick one to get started. You can change it any time in Settings.",
        )}
      </p>

      {error != null && <ErrorNotice error={error} detailsLabel={t("Details")} className="mb-3" />}

      {detected.length > 0 && (
        <div className="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
          {detected.map((d) => (
            <Button
              key={d.provider}
              variant="outline"
              data-testid={`connect-ai-${d.provider}`}
              onClick={() => void selectDetected(d)}
              disabled={busy !== null}
              className="h-auto whitespace-normal rounded-lg border-primary/40 bg-background p-4 text-left flex-col items-start hover:border-primary/70 hover:bg-primary/10"
            >
              <div className="flex items-center gap-2 text-sm font-semibold">
                {busy === d.provider && <Loader2 size={13} className="animate-spin" />}
                <span translate="no">{d.label}</span>
              </div>
              <div className="mt-1 text-xs text-muted-foreground font-normal">{d.detail}</div>
            </Button>
          ))}
        </div>
      )}

      <div className="space-y-3">
        <button
          type="button"
          data-testid="connect-ai-key-toggle"
          onClick={() => setKeyOpen((o) => !o)}
          className={cn(
            "flex items-center gap-1.5 text-xs font-medium text-primary hover:underline",
          )}
        >
          <KeyRound size={12} />
          {t("I have an API key")}
        </button>

        {keyOpen && (
          <div
            className="rounded-lg border border-border bg-background p-3"
            data-testid="connect-ai-key-form"
          >
            <div
              className="mb-2 flex flex-wrap gap-1.5"
              role="radiogroup"
              aria-label={t("Provider")}
            >
              {KEY_PROVIDERS.map((p) => (
                <button
                  key={p.name}
                  type="button"
                  role="radio"
                  aria-checked={keyProvider === p.name}
                  onClick={() => setKeyProvider(p.name)}
                  className={cn(
                    "rounded-full border px-2.5 py-0.5 text-xs transition-colors",
                    keyProvider === p.name
                      ? "border-primary bg-primary/10 text-primary"
                      : "border-border text-muted-foreground hover:border-primary/40",
                  )}
                >
                  <span translate="no">{p.label}</span>
                </button>
              ))}
            </div>
            <Label htmlFor="connect-ai-key" className="mb-1 block text-xs text-muted-foreground">
              {t("API key (stored in your OS keychain)")}
            </Label>
            <div className="flex gap-2">
              <Input
                id="connect-ai-key"
                type="password"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder="sk-..."
              />
              <Button size="sm" onClick={() => void saveKey()} disabled={!apiKey || busy !== null}>
                {busy === "key" && <Loader2 size={12} className="animate-spin" />}
                {t("Save")}
              </Button>
            </div>
          </div>
        )}

        <button
          type="button"
          data-testid="connect-ai-skip-demo"
          onClick={() => void skipToDemo()}
          disabled={busy !== null}
          className="block text-xs text-muted-foreground/70 hover:text-muted-foreground hover:underline"
        >
          {t("Skip: use the demo engine (illustrative output, no AI)")}
        </button>
      </div>
    </section>
  );
}
