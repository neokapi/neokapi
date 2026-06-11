import { LocaleSelect, type LocaleInfo } from "../ui/locale-select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "../ui/dialog";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";

interface TMAddEntryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  source: string;
  onSourceChange: (value: string) => void;
  target: string;
  onTargetChange: (value: string) => void;
  srcLocale: string;
  onSrcLocaleChange: (value: string) => void;
  tgtLocale: string;
  onTgtLocaleChange: (value: string) => void;
  /** Known locales for the locale selectors. If empty, plain text inputs are used. */
  locales: LocaleInfo[];
  onSubmit: () => void;
}

/** Add-entry dialog of the TM browser. Form state is owned by the parent. */
export function TMAddEntryDialog({
  open,
  onOpenChange,
  source,
  onSourceChange,
  target,
  onTargetChange,
  srcLocale,
  onSrcLocaleChange,
  tgtLocale,
  onTgtLocaleChange,
  locales,
  onSubmit,
}: TMAddEntryDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Add TM Entry</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div>
            <Label className="text-[12px]">Source</Label>
            <Input
              value={source}
              onChange={(e) => onSourceChange(e.target.value)}
              placeholder="Source text"
              autoFocus
              className="mt-1"
            />
          </div>
          <div>
            <Label className="text-[12px]">Target</Label>
            <Input
              value={target}
              onChange={(e) => onTargetChange(e.target.value)}
              placeholder="Target text"
              className="mt-1"
            />
          </div>
          <div className="flex gap-3">
            <div className="flex-1">
              <Label className="text-[12px]">Source locale</Label>
              {locales.length > 0 ? (
                <LocaleSelect
                  value={srcLocale}
                  onChange={onSrcLocaleChange}
                  locales={locales}
                  placeholder="Select source..."
                />
              ) : (
                <Input
                  value={srcLocale}
                  onChange={(e) => onSrcLocaleChange(e.target.value)}
                  className="mt-1"
                />
              )}
            </div>
            <div className="flex-1">
              <Label className="text-[12px]">Target locale</Label>
              {locales.length > 0 ? (
                <LocaleSelect
                  value={tgtLocale}
                  onChange={onTgtLocaleChange}
                  locales={locales}
                  placeholder="Select target..."
                />
              ) : (
                <Input
                  value={tgtLocale}
                  onChange={(e) => onTgtLocaleChange(e.target.value)}
                  className="mt-1"
                />
              )}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={onSubmit} disabled={!source.trim() || !target.trim()}>
            Add
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
