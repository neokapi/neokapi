import React, { useEffect, useState } from "react";
import { Check, Copy, Link2 } from "lucide-react";

// Share: a link to this lab at the chapter in view, and when asked, with the
// files the reader changed, so a colleague opens the same sandbox.

export interface ShareMenuProps {
  open: boolean;
  onClose: () => void;
  chapterTitle: string;
  makeLink: (includeFiles: boolean) => { url: string; tooLarge: boolean; bytes: number };
  /** The reader changed files since the lab started. */
  hasChanges: boolean;
}

function kb(bytes: number): string {
  return bytes < 1024 ? `${bytes} B` : `${(bytes / 1024).toFixed(1)} KB`;
}

export default function ShareMenu({
  open,
  onClose,
  chapterTitle,
  makeLink,
  hasChanges,
}: ShareMenuProps): React.ReactElement | null {
  const [includeFiles, setIncludeFiles] = useState(false);
  const [copied, setCopied] = useState(false);
  const [link, setLink] = useState(() => makeLink(false));

  useEffect(() => {
    if (open) setLink(makeLink(includeFiles));
  }, [open, includeFiles, makeLink]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(link.url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    } catch {
      /* the field below can be copied by hand */
    }
  };

  return (
    <div className="kl-share" role="dialog" aria-label="Share this lab">
      <div className="kl-share__head">
        <Link2 size={16} aria-hidden="true" />
        <span>Share</span>
        <button type="button" className="kl-linkbtn" onClick={onClose}>
          Close
        </button>
      </div>
      <p className="kl-share__text">
        Opens this lab at <strong>{chapterTitle}</strong>, with the earlier chapters replayed.
      </p>
      <label className="kl-share__opt">
        <input
          type="checkbox"
          checked={includeFiles}
          onChange={(e) => setIncludeFiles(e.target.checked)}
          disabled={!hasChanges}
        />
        <span>
          Include my files
          {hasChanges ? (
            includeFiles ? (
              ` (${kb(link.bytes)} in the link)`
            ) : (
              ""
            )
          ) : (
            <span className="kl-share__muted"> (nothing changed yet)</span>
          )}
        </span>
      </label>
      {link.tooLarge && (
        <p className="kl-share__warn">
          The changed files are too large for a link. Download them from the files pane instead.
        </p>
      )}
      <div className="kl-share__row">
        <input
          className="kl-share__url"
          readOnly
          value={link.url}
          onFocus={(e) => e.currentTarget.select()}
          aria-label="Shareable link"
        />
        <button type="button" className="kl-btn kl-btn--primary" onClick={() => void copy()}>
          {copied ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
    </div>
  );
}
