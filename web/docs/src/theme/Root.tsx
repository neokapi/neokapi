import React, { useState } from "react";
import BrowserOnly from "@docusaurus/BrowserOnly";

const STORAGE_KEY = "neokapi-banner-dismissed";

function ExperimentalBanner() {
  // Rendered only on the client (via BrowserOnly), after hydration completes,
  // so it never participates in hydration — reading localStorage in the state
  // initializer is safe and there is no server markup to mismatch. (Revealing
  // the banner from a useEffect during hydration tripped React 19's recoverable
  // hydration error #418 in production.)
  const [dismissed, setDismissed] = useState(() => localStorage.getItem(STORAGE_KEY) === "true");

  if (dismissed) {
    return null;
  }

  const handleDismiss = () => {
    localStorage.setItem(STORAGE_KEY, "true");
    setDismissed(true);
  };

  return (
    <div
      style={{
        backgroundColor: "#fef3c7",
        color: "#92400e",
        padding: "12px 16px",
        textAlign: "center",
        position: "relative",
        fontSize: "14px",
        borderBottom: "1px solid #fcd34d",
      }}
    >
      <span>
        <strong>Experimental:</strong> Neokapi is, like some current government administrations, an
        ongoing experiment and should not be used in production.
      </span>
      <button
        onClick={handleDismiss}
        aria-label="Dismiss banner"
        style={{
          position: "absolute",
          right: "16px",
          top: "50%",
          transform: "translateY(-50%)",
          background: "none",
          border: "none",
          cursor: "pointer",
          fontSize: "18px",
          color: "#92400e",
          padding: "4px 8px",
        }}
      >
        &times;
      </button>
    </div>
  );
}

export default function Root({ children }: { children: React.ReactNode }) {
  return (
    <>
      <BrowserOnly>{() => <ExperimentalBanner />}</BrowserOnly>
      {children}
    </>
  );
}
