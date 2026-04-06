import React, { useState, useEffect } from "react";

const STORAGE_KEY = "neokapi-banner-dismissed";

function ExperimentalBanner() {
  const [dismissed, setDismissed] = useState(true); // Start hidden to avoid flash

  useEffect(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    setDismissed(stored === "true");
  }, []);

  const handleDismiss = () => {
    localStorage.setItem(STORAGE_KEY, "true");
    setDismissed(true);
  };

  if (dismissed) {
    return null;
  }

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
      <ExperimentalBanner />
      {children}
    </>
  );
}
