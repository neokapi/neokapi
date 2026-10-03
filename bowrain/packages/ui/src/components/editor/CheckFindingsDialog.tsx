import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import type { FindingsDialogState } from "../../hooks/useContentChanges";
import { useLocales } from "../../hooks/useLocales";
import { PlaceholderText } from "./PlaceholderText";

export interface CheckFindingsDialogProps {
  /** The open prompt (`useContentChanges().findingsDialog`), or null when none is open. */
  state: FindingsDialogState | null;
}

/**
 * Asks what to do when the project's checks refuse a translation a person
 * saved: it lists what the checks found, and the person either goes back to the
 * wording or saves it anyway. Saving anyway records the findings on the change
 * as their override, where history and review show them.
 */
export function CheckFindingsDialog({ state }: CheckFindingsDialogProps) {
  const { getDisplayName } = useLocales();
  const language = state ? getDisplayName(state.locale) || state.locale : "";
  const findings = state?.findings ?? [];

  return (
    <Dialog open={state !== null} onOpenChange={(open) => !open && state?.onRevise()}>
      <DialogContent className="sm:max-w-[560px]" data-testid="check-findings-dialog">
        <DialogHeader>
          <DialogTitle>The checks found problems in this translation</DialogTitle>
          <DialogDescription>
            Your {language} translation has not been saved. Change the wording, or save it as it is.
            Saving it anyway records these findings with the change.
          </DialogDescription>
        </DialogHeader>

        <ul className="space-y-1.5 text-sm" data-testid="check-findings">
          {findings.length === 0 ? (
            <li className="text-muted-foreground">The checks did not say what they found.</li>
          ) : (
            findings.map((f, i) => (
              <li
                key={`${f.rule}-${i}`}
                className="rounded-md border border-border bg-muted/20 p-2"
                data-testid="check-finding"
              >
                <span className="mr-2 font-mono text-[11px] text-muted-foreground">{f.rule}</span>
                {f.message}
              </li>
            ))
          )}
        </ul>
        {state?.mine !== undefined && (
          <section className="space-y-1">
            <h3 className="text-xs font-medium text-muted-foreground">Your version</h3>
            <p
              className="rounded-md border border-primary/40 bg-primary/5 p-2 text-sm"
              data-testid="check-findings-mine"
            >
              <PlaceholderText text={state.mine} />
            </p>
          </section>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => state?.onRevise()} data-testid="findings-revise">
            Don't save
          </Button>
          <Button onClick={() => state?.onOverride()} data-testid="findings-override">
            Save anyway
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
