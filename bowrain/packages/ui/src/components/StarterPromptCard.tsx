import { Button, Card, CardContent, cn } from "@neokapi/ui-primitives";
import { useCallback, useState } from "react";
import type { ReactNode } from "react";
import { ArrowRight, Check, Copy, Sparkles } from "./icons";
import { TEST_IDS } from "../test-ids";

/** Docs entry point for the assistant-driven starter-pack flow. */
export const STARTER_PACK_DOCS_URL = "https://bowrain.cloud/docs/quickstart";

export interface StarterPromptProps {
  /** Named in the prompt, so the assistant pushes into the right workspace. */
  workspaceName?: string;
  /** Server origin the assistant connects to (web shells). */
  serverUrl?: string;
  /** Repository the assistant is pointed at, where one is already known. */
  repoUrl?: string;
  className?: string;
}

/**
 * The prompt text a user hands to their assistant. A known repository is folded
 * in so the assistant has the material to read without being told where it is.
 */
export function starterPromptText({ workspaceName, serverUrl, repoUrl }: StarterPromptProps) {
  const target = workspaceName ? `the ${workspaceName} workspace` : "this workspace";
  const server = serverUrl ? ` at ${serverUrl}` : "";
  if (repoUrl) {
    return `Install the kapi skill, then: read ${repoUrl}, set up a brand starter pack for ${target}, and connect this repository to Bowrain${server}.`;
  }
  return `Install the kapi skill, then: set up a brand starter pack for ${target} and connect this project to Bowrain${server}.`;
}

/** Copyable prompt for the user's AI assistant (starter-pack onboarding). */
export function StarterPrompt({
  workspaceName,
  serverUrl,
  repoUrl,
  className,
}: StarterPromptProps) {
  const [copied, setCopied] = useState(false);
  const prompt = starterPromptText({ workspaceName, serverUrl, repoUrl });

  const handleCopy = useCallback(() => {
    void navigator.clipboard?.writeText(prompt).then(
      () => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 2000);
      },
      () => {},
    );
  }, [prompt]);

  return (
    <div
      className={cn(
        "flex flex-col gap-2 rounded-lg border border-border bg-muted/40 p-4",
        className,
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-muted-foreground">Prompt for your assistant</span>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 gap-1 text-xs"
          onClick={handleCopy}
          data-testid={TEST_IDS.onboarding.copyStarterPrompt}
        >
          {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <pre
        data-testid={TEST_IDS.onboarding.starterPrompt}
        className="whitespace-pre-wrap font-mono text-xs leading-relaxed text-foreground/90"
      >
        {prompt}
      </pre>
    </div>
  );
}

export interface StarterPromptCardProps extends StarterPromptProps {
  /** Rendered under the docs link — e.g. the hosted-scan fallback. */
  footer?: ReactNode;
}

/**
 * The assistant lane, as one card: what the kapi skill does with the user's own
 * material, and the prompt that starts it. The single onboarding idiom shared by
 * the blank-workspace landing, the Context hub's front door, and the moment a
 * repository is connected.
 */
export function StarterPromptCard({
  workspaceName,
  serverUrl,
  repoUrl,
  footer,
  className,
}: StarterPromptCardProps) {
  return (
    <Card
      className={cn("relative overflow-hidden border-primary/20 md:min-h-[300px]", className)}
      data-testid={TEST_IDS.onboarding.starterPromptCard}
    >
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 bg-gradient-to-br from-primary/[0.06] to-transparent"
      />
      <CardContent className="relative grid items-start gap-8 px-6 py-6 md:grid-cols-[1.15fr_1fr] md:px-8 md:py-8">
        <div>
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10">
              <Sparkles className="h-5 w-5 text-primary" />
            </div>
            <h3 className="text-base font-semibold">Build your brand starter pack with your AI</h3>
          </div>
          {repoUrl ? (
            <p className="mb-4 text-sm leading-relaxed text-muted-foreground">
              Point Claude, or any AI assistant with the kapi skill, at{" "}
              <span className="font-medium text-foreground">{repoUrl}</span>. It reads the
              repository, drafts a voice profile and terminology, and pushes both to this workspace,
              so every translation starts on brand.
            </p>
          ) : (
            <p className="mb-4 text-sm leading-relaxed text-muted-foreground">
              Point Claude, or any AI assistant with the kapi skill, at your website or repository.
              It reads your material, drafts a voice profile and terminology, and pushes both to
              this workspace, so every translation starts on brand.
            </p>
          )}
          <a
            href={STARTER_PACK_DOCS_URL}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline"
          >
            How the starter pack works
            <ArrowRight className="h-3.5 w-3.5" />
          </a>
          {footer}
        </div>
        <StarterPrompt workspaceName={workspaceName} serverUrl={serverUrl} repoUrl={repoUrl} />
      </CardContent>
    </Card>
  );
}
