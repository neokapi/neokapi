import { Card, Textarea } from "@neokapi/ui-primitives";
import { useEffect, useState } from "react";
import type { VoiceProfile } from "../brand/types";
import type { BrandScanCheckResult } from "../types/api";
import { BrandScoreGauge } from "../brand/BrandScoreGauge";
import { complianceBar } from "../brand/complianceBar";
import { BrandFindingsList } from "../brand/BrandFindingsList";
import { useCheckBrandDraft } from "../hooks/useBrandScanApi";
import { TEST_IDS } from "../test-ids";

export interface BrandScanLiveTesterProps {
  /** The draft under review — checks run against the current edits. */
  profile: VoiceProfile;
  /** Debounce before a check fires. Tests lower this. */
  debounceMs?: number;
}

/**
 * Deterministic live tester for a draft profile: paste sample copy and watch
 * the vocabulary rules score it. The check is stateless and costs no AI
 * credits, so it can run on every pause in typing.
 */
export function BrandScanLiveTester({ profile, debounceMs = 500 }: BrandScanLiveTesterProps) {
  const check = useCheckBrandDraft();
  const { mutate } = check;
  const [text, setText] = useState("");
  const [result, setResult] = useState<BrandScanCheckResult | null>(null);

  useEffect(() => {
    if (!text.trim()) {
      setResult(null);
      return;
    }
    const timer = window.setTimeout(() => {
      mutate({ profile, text }, { onSuccess: (res) => setResult(res) });
    }, debounceMs);
    return () => window.clearTimeout(timer);
  }, [text, profile, debounceMs, mutate]);

  return (
    <Card data-testid={TEST_IDS.brandScan.liveTester} className="p-5 space-y-4">
      <div>
        <h2 className="text-sm font-semibold">Try it on your copy</h2>
        <p className="text-xs text-muted-foreground mt-1">
          Paste a sentence or two; the draft&rsquo;s vocabulary rules score it as you type. No AI
          credits are used.
        </p>
      </div>
      <Textarea
        value={text}
        onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setText(e.target.value)}
        placeholder="e.g. Leverage our synergistic platform to unlock value…"
        rows={4}
        className="text-sm"
      />
      {result && (
        <div className="space-y-3">
          <div className="flex items-center gap-4">
            <BrandScoreGauge score={result.score.overall} bar={complianceBar(profile)} size={72} />
            <p className="text-xs text-muted-foreground">
              {result.findings.length === 0
                ? "No rule violations in this sample."
                : `${result.findings.length} finding${result.findings.length === 1 ? "" : "s"} against the draft rules.`}
            </p>
          </div>
          {result.findings.length > 0 && <BrandFindingsList findings={result.findings} />}
        </div>
      )}
    </Card>
  );
}
