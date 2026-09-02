import { Button, Card, CardContent } from "@neokapi/ui-primitives";
import { StarterPromptCard } from "../../components/StarterPromptCard";
import { Sparkles } from "../../components/icons";
import { TEST_IDS } from "../../test-ids";

export interface ContextOnboardingProps {
  /** Named in the assistant prompt. */
  workspaceName?: string;
  /** Server origin folded into the assistant prompt (web shells). */
  serverUrl?: string;
  /** Opens the hosted context scan. Absent when the server runs no scan jobs. */
  onScanVoice?: () => void;
}

/**
 * The Context hub's front door before anything governs anything: the two ways a
 * workspace acquires its first voice profile and vocabulary. The assistant lane
 * reads the user's own material with the kapi skill and pushes the result; the
 * hosted lane drafts one from pasted text, links, or uploaded files.
 */
export function ContextOnboarding({
  workspaceName,
  serverUrl,
  onScanVoice,
}: ContextOnboardingProps) {
  return (
    <div className="space-y-5" data-testid={TEST_IDS.onboarding.contextLanes}>
      <StarterPromptCard workspaceName={workspaceName} serverUrl={serverUrl} />

      {onScanVoice && (
        <Card data-testid={TEST_IDS.onboarding.contextScanCard}>
          <CardContent className="flex flex-col gap-4 px-6 py-6 md:flex-row md:items-center md:justify-between">
            <div className="min-w-0">
              <div className="mb-2 flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10">
                  <Sparkles className="h-5 w-5 text-primary" />
                </div>
                <h3 className="text-base font-semibold">Scan your brand instead</h3>
              </div>
              <p className="max-w-xl text-sm leading-relaxed text-muted-foreground">
                Paste text, add links, or drop your marketing files. The scan drafts a voice profile
                and candidate terms for your review. No assistant required.
              </p>
            </div>
            <Button
              className="shrink-0"
              onClick={onScanVoice}
              data-testid={TEST_IDS.onboarding.contextScanButton}
            >
              <Sparkles className="mr-1.5 size-3.5" /> Scan your brand
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
