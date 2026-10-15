// The dialog behind Set up sharing: choose where the project's context is
// shared, fill in the place, and save it as the `context:` block of kapi.yaml.
// Once saved, the dialog offers the first sync, and closes.
//
// The three places are the backends kapi has: the project's own git
// repository, a folder every machine can reach, or an S3 bucket. Credentials
// never go into the recipe, so the dialog asks for none.

import { useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  ErrorNotice,
  Input,
  Label,
  cn,
} from "@neokapi/ui-primitives";
import { api, type ContextBackend, type ProjectSharing } from "./contextTypes";

/** The places a recipe can share a project's context through. */
export type ShareKind = "git" | "file" | "s3";

/** What the person has typed, one field per `context:` key. */
export interface ShareFields {
  ref: string;
  remote: string;
  path: string;
  bucket: string;
  prefix: string;
  region: string;
  endpoint: string;
}

const EMPTY: ShareFields = {
  ref: "",
  remote: "",
  path: "",
  bucket: "",
  prefix: "",
  region: "",
  endpoint: "",
};

/** The `context:` block for a kind, with every empty field left out. */
export function shareSpec(kind: ShareKind, f: ShareFields): ContextBackend {
  const v = (s: string) => s.trim() || undefined;
  switch (kind) {
    case "git":
      return { backend: "git", ref: v(f.ref), remote: v(f.remote) };
    case "file":
      return { backend: "file", path: v(f.path) };
    case "s3":
      return {
        backend: "s3",
        bucket: v(f.bucket),
        prefix: v(f.prefix),
        region: v(f.region),
        endpoint: v(f.endpoint),
      };
  }
}

/** The field a kind needs and the person left empty, or null. */
export function missingField(kind: ShareKind, f: ShareFields): "path" | "bucket" | null {
  if (kind === "file" && !f.path.trim()) return "path";
  if (kind === "s3" && !f.bucket.trim()) return "bucket";
  return null;
}

export interface ShareSetupDialogProps {
  tabID: string;
  open: boolean;
  onClose: () => void;
  /** Called with the backend's answer once kapi.yaml names the place. */
  onSaved?: (sharing: ProjectSharing) => void;
  /** Sync now, offered once saved. Absent syncs through the backend here. */
  onSync?: () => void;
}

export function ShareSetupDialog({ tabID, open, onClose, onSaved, onSync }: ShareSetupDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg" data-slot="share-setup-dialog">
        {/* Mounted only while open, so every opening starts from an empty form. */}
        {open && <SetupForm tabID={tabID} onClose={onClose} onSaved={onSaved} onSync={onSync} />}
      </DialogContent>
    </Dialog>
  );
}

function SetupForm({ tabID, onClose, onSaved, onSync }: Omit<ShareSetupDialogProps, "open">) {
  const [kind, setKind] = useState<ShareKind>("git");
  const [fields, setFields] = useState<ShareFields>(EMPTY);
  const [missing, setMissing] = useState<"path" | "bucket" | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState(false);

  const set = (key: keyof ShareFields) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setFields((f) => ({ ...f, [key]: e.target.value }));
    setMissing(null);
  };

  const save = async () => {
    const gap = missingField(kind, fields);
    setMissing(gap);
    if (gap) return;
    setSaving(true);
    setError(null);
    try {
      const sharing = await api.setProjectSharing(tabID, shareSpec(kind, fields));
      setSaved(true);
      if (sharing) onSaved?.(sharing);
    } catch (e) {
      setError(e);
    } finally {
      setSaving(false);
    }
  };

  const syncNow = () => {
    if (onSync) onSync();
    else void api.syncProjectContext(tabID);
    onClose();
  };

  if (saved) {
    return (
      <>
        <DialogHeader>
          <DialogTitle>Set up sharing</DialogTitle>
        </DialogHeader>
        <DialogDescription data-slot="share-setup-saved">
          Saved to <span translate="no">kapi.yaml</span>. Sync now shares the context this Mac
          holds, and reads what other machines have shared.
        </DialogDescription>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
          <Button data-slot="share-setup-sync" onClick={syncNow}>
            Sync now
          </Button>
        </DialogFooter>
      </>
    );
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>Set up sharing</DialogTitle>
      </DialogHeader>
      <DialogDescription>
        Choose a place every machine that syncs the project can reach. Saving writes it into{" "}
        <span translate="no">kapi.yaml</span>.
      </DialogDescription>

      <div role="radiogroup" aria-label={t("Where to share")} className="space-y-2">
        <KindChoice kind="git" current={kind} onChoose={setKind}>
          <Label htmlFor="share-kind-git" className="text-sm font-medium">
            Git
          </Label>
          <p id="share-kind-git-desc" className="text-[11px] text-muted-foreground">
            The project&apos;s own repository, a ref outside every branch; whoever can push a branch
            can push it.
          </p>
        </KindChoice>
        <KindChoice kind="file" current={kind} onChoose={setKind}>
          <Label htmlFor="share-kind-file" className="text-sm font-medium">
            A folder
          </Label>
          <p id="share-kind-file-desc" className="text-[11px] text-muted-foreground">
            A directory every machine can reach, such as a mounted team share.
          </p>
        </KindChoice>
        <KindChoice kind="s3" current={kind} onChoose={setKind}>
          <Label htmlFor="share-kind-s3" className="text-sm font-medium">
            S3
          </Label>
          <p id="share-kind-s3-desc" className="text-[11px] text-muted-foreground">
            A bucket on S3 or a compatible service. Access comes from your AWS environment and
            profile.
          </p>
        </KindChoice>
      </div>

      <div data-slot="share-setup-fields" className="space-y-3">
        {kind === "git" && (
          <>
            <Field id="share-ref" label="Ref">
              <Input
                id="share-ref"
                value={fields.ref}
                onChange={set("ref")}
                placeholder="refs/kapi/context"
                className="font-mono"
                translate="no"
              />
            </Field>
            <Field id="share-remote" label="Remote">
              <Input
                id="share-remote"
                value={fields.remote}
                onChange={set("remote")}
                placeholder="origin"
                className="font-mono"
                translate="no"
              />
            </Field>
          </>
        )}
        {kind === "file" && (
          <Field id="share-path" label="Path">
            <Input
              id="share-path"
              value={fields.path}
              onChange={set("path")}
              aria-invalid={missing === "path" || undefined}
              className="font-mono"
              translate="no"
            />
          </Field>
        )}
        {kind === "s3" && (
          <>
            <Field id="share-bucket" label="Bucket">
              <Input
                id="share-bucket"
                value={fields.bucket}
                onChange={set("bucket")}
                aria-invalid={missing === "bucket" || undefined}
                className="font-mono"
                translate="no"
              />
            </Field>
            <Field id="share-prefix" label="Prefix">
              <Input
                id="share-prefix"
                value={fields.prefix}
                onChange={set("prefix")}
                className="font-mono"
                translate="no"
              />
            </Field>
            <Field id="share-region" label="Region">
              <Input
                id="share-region"
                value={fields.region}
                onChange={set("region")}
                className="font-mono"
                translate="no"
              />
            </Field>
            <Field id="share-endpoint" label="Endpoint">
              <Input
                id="share-endpoint"
                value={fields.endpoint}
                onChange={set("endpoint")}
                className="font-mono"
                translate="no"
              />
            </Field>
          </>
        )}
      </div>

      {missing && (
        <p data-slot="share-setup-missing" role="alert" className="text-sm text-destructive">
          {missing === "path" ? <>A folder needs a path.</> : <>S3 needs a bucket.</>}
        </p>
      )}
      {error != null && <ErrorNotice error={error} title="kapi.yaml could not be saved" />}

      <DialogFooter>
        <Button variant="ghost" onClick={onClose} disabled={saving}>
          Cancel
        </Button>
        <Button data-slot="share-setup-save" onClick={() => void save()} disabled={saving}>
          Save to <span translate="no">kapi.yaml</span>
        </Button>
      </DialogFooter>
    </>
  );
}

/** One place to share through: a radio, with its name and what it is. */
function KindChoice({
  kind,
  current,
  onChoose,
  children,
}: {
  kind: ShareKind;
  current: ShareKind;
  onChoose: (kind: ShareKind) => void;
  children: React.ReactNode;
}) {
  const id = `share-kind-${kind}`;
  return (
    <div
      data-slot="share-kind"
      data-kind={kind}
      className={cn(
        "flex items-start gap-2.5 rounded-md border px-3 py-2",
        current === kind ? "border-primary/60" : "border-border",
      )}
    >
      <input
        id={id}
        type="radio"
        name="share-kind"
        value={kind}
        checked={current === kind}
        onChange={() => onChoose(kind)}
        aria-describedby={`${id}-desc`}
        className="mt-1 size-3.5 accent-primary"
      />
      <div className="min-w-0">{children}</div>
    </div>
  );
}

/** A labelled field. */
function Field({ id, label, children }: { id: string; label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <Label htmlFor={id} className="text-[11px] text-muted-foreground">
        {label}
      </Label>
      {children}
    </div>
  );
}
