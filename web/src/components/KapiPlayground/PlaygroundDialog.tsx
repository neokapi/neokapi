import React from "react";
import "./explorer.css";

/** Native modal semantics keep keyboard focus and background interaction scoped to the terminal. */
export function PlaygroundDialog({ children }: { children: React.ReactNode }): React.ReactElement {
  const [open, setOpen] = React.useState(false);
  const [launched, setLaunched] = React.useState(false);
  const dialog = React.useRef<HTMLDialogElement>(null);
  const launch = React.useRef<HTMLButtonElement>(null);
  const descriptionId = React.useId();

  React.useEffect(() => {
    const element = dialog.current;
    if (!element) return;
    if (!open) {
      if (element.open) {
        element.close();
        launch.current?.focus();
      }
      return;
    }
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    if (!element.open) element.showModal();
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [open]);

  return (
    <>
      <p>
        Opening the terminal downloads the engine on first use. Your session stays available when
        you close this window. Download files you want to keep before reloading the page.
      </p>
      <button
        ref={launch}
        type="button"
        className="button button--primary"
        onClick={() => {
          setLaunched(true);
          setOpen(true);
        }}
      >
        {launched ? "Resume terminal" : "Open terminal"}
      </button>
      <dialog
        ref={dialog}
        className="kapi-pgx-dialog"
        aria-label="CLI playground"
        aria-describedby={descriptionId}
        onCancel={(event) => {
          event.preventDefault();
          if (dialog.current?.querySelector(".kapi-pg-overlay")) return;
          setOpen(false);
        }}
        onClose={() => setOpen(false)}
      >
        <header className="kapi-pgx-dialog__header">
          <div>
            <h2>CLI playground</h2>
            <p id={descriptionId}>Files and terminal history stay in this browser session.</p>
          </div>
          <button
            type="button"
            className="button button--secondary button--sm"
            onClick={() => setOpen(false)}
          >
            Close terminal
          </button>
        </header>
        <div className="kapi-pgx-dialog__body">{launched && children}</div>
      </dialog>
    </>
  );
}
