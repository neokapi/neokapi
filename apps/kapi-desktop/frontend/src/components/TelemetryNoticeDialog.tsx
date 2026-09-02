import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import { markTelemetryNoticeShown, setTelemetryEnabled } from "../analytics";

/**
 * One-time first-run notice for the opt-out desktop analytics (decision D1).
 * Only mounted in builds that carry an analytics key (App gates on the
 * initAnalytics result). The off-switch is inline; the same toggle lives in
 * Settings afterwards.
 */
export function TelemetryNoticeDialog({ onClose }: { onClose: () => void }) {
  const close = () => {
    void markTelemetryNoticeShown();
    onClose();
  };

  return (
    <Dialog
      open
      onOpenChange={(o) => {
        if (!o) close();
      }}
    >
      <DialogContent showCloseButton={false} className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>{t("Anonymous usage statistics", "Telemetry")}</DialogTitle>
          <DialogDescription>
            {t(
              "Kapi collects anonymous usage statistics: screen views and feature usage, never your content, project names, or file paths. This helps prioritize improvements. You can turn it off now, or at any time in Settings.",
              "Telemetry",
            )}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              void setTelemetryEnabled(false);
              close();
            }}
          >
            {t("Disable", "Telemetry")}
          </Button>
          <Button onClick={close}>{t("OK", "Telemetry")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
