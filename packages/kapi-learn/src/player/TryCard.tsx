import React from "react";
import { Play } from "lucide-react";

// What to try, under a playground's stage or after a lab's last chapter. In a
// terminal the items are commands: pressing one types it at the prompt and
// runs it. In an explorer they are prompts in prose.

export interface TryCardProps {
  items: readonly string[];
  /** Commands the terminal runs when pressed, or prose. */
  mode: "commands" | "prose";
  ready: boolean;
  busy?: boolean;
  onRun?: (command: string) => void;
  title?: string;
  /** A sentence above the list. */
  lead?: string;
}

export default function TryCard({
  items,
  mode,
  ready,
  busy = false,
  onRun,
  title = "Try",
  lead,
}: TryCardProps): React.ReactElement | null {
  if (items.length === 0) return null;
  return (
    <section className="kl-try-card" aria-label={title}>
      <h2 className="kl-try-card__title">{title}</h2>
      {lead && <p className="kl-try-card__lead">{lead}</p>}
      <ul className={`kl-try-card__list kl-try-card__list--${mode}`}>
        {items.map((item) =>
          mode === "commands" ? (
            <li key={item}>
              <button
                type="button"
                className="kl-chip"
                onClick={() => onRun?.(item)}
                disabled={!ready || busy}
                title="Run this in the terminal"
              >
                <Play size={12} aria-hidden="true" fill="currentColor" />
                <code>{item}</code>
              </button>
            </li>
          ) : (
            <li key={item}>{item}</li>
          ),
        )}
      </ul>
    </section>
  );
}
