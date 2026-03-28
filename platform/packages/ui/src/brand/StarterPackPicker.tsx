import { Dialog, DialogContent, DialogHeader, DialogTitle } from "../components/ui/dialog";
import { Card } from "../components/ui/card";
import { Wand2 } from "../components/icons";
import { StarterPackCard } from "./StarterPackCard";
import { starterPacks, type StarterPackMeta } from "./data/starter-packs";

interface StarterPackPickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (pack: StarterPackMeta) => void;
  onScratch: () => void;
}

export function StarterPackPicker({
  open,
  onOpenChange,
  onSelect,
  onScratch,
}: StarterPackPickerProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>Choose a Starting Point</DialogTitle>
          <p className="text-sm text-muted-foreground">
            Pick a template to get started quickly, or create from scratch.
          </p>
        </DialogHeader>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-2">
          {starterPacks.map((pack) => (
            <StarterPackCard key={pack.name} pack={pack} onClick={onSelect} />
          ))}

          {/* Start from scratch card */}
          <Card
            className="group cursor-pointer overflow-hidden transition-all hover:shadow-md hover:border-foreground/20 border-dashed"
            onClick={onScratch}
          >
            <div className="h-1 bg-border/50" />
            <div className="p-5 flex flex-col items-center justify-center text-center min-h-[140px] gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-muted">
                <Wand2 className="h-4.5 w-4.5 text-muted-foreground" />
              </div>
              <div>
                <h3 className="text-sm font-semibold">Start from Scratch</h3>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Build your brand voice from a blank canvas
                </p>
              </div>
            </div>
          </Card>
        </div>
      </DialogContent>
    </Dialog>
  );
}
