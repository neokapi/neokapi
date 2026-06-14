// The op builder (AD-021): the small forms that compose a change-set's
// operations — ban a term, change a preferred term, add a relation, add or
// remove a voice rule, edit a concept — against the live concept graph. Each
// "Add operation" appends to the draft via the real append-op endpoint; the
// caller (WhatIfWizard, or the detail view's add-op dialog) shows the resulting
// ops and the refreshed blast radius. Concept/term/profile pickers read the
// real hooks; only a draft change-set id is required.
import { useMemo, useState } from "react";
import {
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Textarea,
  cn,
} from "@neokapi/ui-primitives";
import { Lock, Sparkles, Network, ScrollText, Trash2, Pencil, Plus } from "../../components/icons";
import type { ConceptInfo } from "../../types/api";
import type {
  AddChangeSetOpRequest,
  RelationType,
  TermStatus,
  VoiceRuleList,
} from "../../types/brand-graph";
import { RELATION_TYPES } from "../../types/brand-graph";
import { relationLabel } from "../shell/atoms";
import { useConcepts } from "../../hooks/useConceptsApi";
import { useBrandProfiles } from "../../hooks/useBrandApi";
import { useAppendChangesetOp } from "../../hooks/useChangesetsApi";

type ActionId = "ban" | "prefer" | "relation" | "voice-add" | "voice-remove" | "edit-concept";

interface ActionDef {
  id: ActionId;
  label: string;
  hint: string;
  icon: React.ReactNode;
}

const ACTIONS: ActionDef[] = [
  { id: "ban", label: "Ban a term", hint: "Forbid it everywhere", icon: <Lock /> },
  { id: "prefer", label: "Prefer a term", hint: "Promote to preferred", icon: <Sparkles /> },
  { id: "relation", label: "Add a relation", hint: "Connect two concepts", icon: <Network /> },
  {
    id: "voice-add",
    label: "Add a voice rule",
    hint: "Preferred / forbidden",
    icon: <ScrollText />,
  },
  { id: "voice-remove", label: "Remove a voice rule", hint: "Drop a rule", icon: <Trash2 /> },
  { id: "edit-concept", label: "Edit a concept", hint: "Definition or domain", icon: <Pencil /> },
];

function newId(prefix: string): string {
  const uuid =
    typeof crypto !== "undefined" && "randomUUID" in crypto
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.round(Math.random() * 1e9)}`;
  return `${prefix}-${uuid}`;
}

export interface OpBuilderProps {
  changesetId: string;
  /** Called after an op is appended, e.g. to nudge the preview or count. */
  onAppended?: () => void;
  className?: string;
}

export function OpBuilder({ changesetId, onAppended, className }: OpBuilderProps) {
  const [action, setAction] = useState<ActionId | null>(null);
  const append = useAppendChangesetOp(changesetId);

  const submit = (req: AddChangeSetOpRequest, reset: () => void) => {
    append.mutate(req, {
      onSuccess: () => {
        reset();
        onAppended?.();
      },
    });
  };

  return (
    <div className={cn("space-y-4", className)}>
      <div>
        <Label className="text-xs text-muted-foreground">Choose an action</Label>
        <div className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-3">
          {ACTIONS.map((a) => (
            <button
              key={a.id}
              type="button"
              onClick={() => setAction(a.id)}
              aria-pressed={action === a.id}
              className={cn(
                "flex flex-col items-start gap-1 rounded-lg border bg-card p-3 text-left transition-colors",
                "hover:border-primary/40 hover:bg-muted/40",
                action === a.id && "border-primary bg-primary/5 ring-1 ring-primary/30",
              )}
            >
              <span className="text-muted-foreground [&_svg]:size-4">{a.icon}</span>
              <span className="text-sm font-medium leading-tight text-foreground">{a.label}</span>
              <span className="text-[11px] text-muted-foreground">{a.hint}</span>
            </button>
          ))}
        </div>
      </div>

      {action && (
        <div className="rounded-lg border bg-muted/20 p-4">
          {action === "ban" && (
            <TermStatusForm
              to="forbidden"
              verb="Ban"
              busy={append.isPending}
              error={append.error}
              onSubmit={submit}
            />
          )}
          {action === "prefer" && (
            <TermStatusForm
              to="preferred"
              verb="Prefer"
              busy={append.isPending}
              error={append.error}
              onSubmit={submit}
            />
          )}
          {action === "relation" && (
            <RelationForm busy={append.isPending} error={append.error} onSubmit={submit} />
          )}
          {action === "voice-add" && (
            <VoiceRuleForm
              mode="add"
              busy={append.isPending}
              error={append.error}
              onSubmit={submit}
            />
          )}
          {action === "voice-remove" && (
            <VoiceRuleForm
              mode="remove"
              busy={append.isPending}
              error={append.error}
              onSubmit={submit}
            />
          )}
          {action === "edit-concept" && (
            <EditConceptForm busy={append.isPending} error={append.error} onSubmit={submit} />
          )}
        </div>
      )}
    </div>
  );
}

// ── Shared form scaffolding ──────────────────────────────────────────────────

type SubmitFn = (req: AddChangeSetOpRequest, reset: () => void) => void;

interface FormProps {
  busy: boolean;
  error: unknown;
  onSubmit: SubmitFn;
}

function FormError({ error }: { error: unknown }) {
  if (!error) return null;
  return (
    <p className="text-sm text-destructive">
      {error instanceof Error ? error.message : "Could not add the operation."}
    </p>
  );
}

function AddButton({ busy, disabled }: { busy: boolean; disabled: boolean }) {
  return (
    <Button type="submit" size="sm" disabled={disabled || busy}>
      <Plus />
      {busy ? "Adding…" : "Add operation"}
    </Button>
  );
}

function useConceptOptions() {
  const { data, isLoading } = useConcepts({ limit: 100 });
  const concepts = data?.concepts ?? [];
  return { concepts, isLoading };
}

function conceptName(c: ConceptInfo): string {
  return c.terms[0]?.text || c.id;
}

// ── Ban / Prefer (term.status) ───────────────────────────────────────────────

function TermStatusForm({
  to,
  verb,
  busy,
  error,
  onSubmit,
}: FormProps & { to: TermStatus; verb: string }) {
  const { concepts, isLoading } = useConceptOptions();
  const [conceptId, setConceptId] = useState("");
  const [termIdx, setTermIdx] = useState("");

  const concept = concepts.find((c) => c.id === conceptId);
  const term = concept?.terms[Number(termIdx)];
  const alreadyAtTarget = term?.status === to;

  const reset = () => {
    setConceptId("");
    setTermIdx("");
  };

  const handle = (e: React.FormEvent) => {
    e.preventDefault();
    if (!concept || !term || alreadyAtTarget) return;
    onSubmit(
      {
        op: "term.status",
        payload: {
          concept_id: concept.id,
          locale: term.locale,
          text: term.text,
          from: term.status as TermStatus,
          to,
        },
      },
      reset,
    );
  };

  return (
    <form onSubmit={handle} className="space-y-3">
      <p className="text-sm font-medium">{verb} a term</p>
      <ConceptPicker
        label="Concept"
        value={conceptId}
        onChange={(v) => {
          setConceptId(v);
          setTermIdx("");
        }}
        concepts={concepts}
        loading={isLoading}
      />
      {concept && (
        <div className="space-y-1.5">
          <Label className="text-xs">Term</Label>
          <Select value={termIdx} onValueChange={setTermIdx}>
            <SelectTrigger size="sm">
              <SelectValue placeholder="Choose a term…" />
            </SelectTrigger>
            <SelectContent>
              {concept.terms.map((t, i) => (
                <SelectItem key={`${t.locale}-${t.text}`} value={String(i)}>
                  {t.text} · {t.locale} · {t.status}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
      {alreadyAtTarget && (
        <p className="text-xs text-muted-foreground">
          This term is already {to}. Pick another term.
        </p>
      )}
      <FormError error={error} />
      <AddButton busy={busy} disabled={!concept || !term || alreadyAtTarget} />
    </form>
  );
}

// ── Add relation (relation.add) ──────────────────────────────────────────────

function RelationForm({ busy, error, onSubmit }: FormProps) {
  const { concepts, isLoading } = useConceptOptions();
  const [sourceId, setSourceId] = useState("");
  const [targetId, setTargetId] = useState("");
  const [type, setType] = useState<RelationType>("RELATED");

  const targets = useMemo(() => concepts.filter((c) => c.id !== sourceId), [concepts, sourceId]);
  const valid = sourceId && targetId && sourceId !== targetId;

  const reset = () => {
    setSourceId("");
    setTargetId("");
    setType("RELATED");
  };

  const handle = (e: React.FormEvent) => {
    e.preventDefault();
    if (!valid) return;
    onSubmit(
      {
        op: "relation.add",
        payload: {
          relation: {
            id: newId("rel"),
            source_id: sourceId,
            target_id: targetId,
            relation_type: type,
            created_at: new Date().toISOString(),
          },
        },
      },
      reset,
    );
  };

  return (
    <form onSubmit={handle} className="space-y-3">
      <p className="text-sm font-medium">Add a relation</p>
      <ConceptPicker
        label="Source concept"
        value={sourceId}
        onChange={(v) => {
          setSourceId(v);
          if (v === targetId) setTargetId("");
        }}
        concepts={concepts}
        loading={isLoading}
      />
      <div className="space-y-1.5">
        <Label className="text-xs">Relation</Label>
        <Select value={type} onValueChange={(v) => setType(v as RelationType)}>
          <SelectTrigger size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {RELATION_TYPES.map((t) => (
              <SelectItem key={t} value={t}>
                {relationLabel(t)}
                {t === "REPLACED_BY" ? " (governed)" : ""}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <ConceptPicker
        label="Target concept"
        value={targetId}
        onChange={setTargetId}
        concepts={targets}
        loading={isLoading}
      />
      <FormError error={error} />
      <AddButton busy={busy} disabled={!valid} />
    </form>
  );
}

// ── Voice rule (voice.rule.add / voice.rule.remove) ──────────────────────────

const VOICE_LISTS: VoiceRuleList[] = ["preferred", "forbidden", "competitor"];

function VoiceRuleForm({ mode, busy, error, onSubmit }: FormProps & { mode: "add" | "remove" }) {
  const { data: profiles, isLoading } = useBrandProfiles();
  const [profileId, setProfileId] = useState("");
  const [list, setList] = useState<VoiceRuleList>(mode === "add" ? "forbidden" : "forbidden");
  const [term, setTerm] = useState("");
  const [replacement, setReplacement] = useState("");

  const valid = profileId && term.trim().length > 0;

  const reset = () => {
    setTerm("");
    setReplacement("");
  };

  const handle = (e: React.FormEvent) => {
    e.preventDefault();
    if (!valid) return;
    if (mode === "add") {
      onSubmit(
        {
          op: "voice.rule.add",
          payload: {
            profile_id: profileId,
            list,
            rule: { term: term.trim(), replacement: replacement.trim() || undefined },
          },
        },
        reset,
      );
    } else {
      onSubmit(
        { op: "voice.rule.remove", payload: { profile_id: profileId, list, term: term.trim() } },
        reset,
      );
    }
  };

  if (!isLoading && (profiles?.length ?? 0) === 0) {
    return (
      <div className="space-y-1">
        <p className="text-sm font-medium">{mode === "add" ? "Add" : "Remove"} a voice rule</p>
        <p className="text-sm text-muted-foreground">
          No voice profiles in this workspace yet. Create one under Brand → Voice first.
        </p>
      </div>
    );
  }

  return (
    <form onSubmit={handle} className="space-y-3">
      <p className="text-sm font-medium">{mode === "add" ? "Add" : "Remove"} a voice rule</p>
      <div className="space-y-1.5">
        <Label className="text-xs">Profile</Label>
        <Select value={profileId} onValueChange={setProfileId}>
          <SelectTrigger size="sm">
            <SelectValue placeholder={isLoading ? "Loading…" : "Choose a profile…"} />
          </SelectTrigger>
          <SelectContent>
            {(profiles ?? []).map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label className="text-xs">List</Label>
        <Select value={list} onValueChange={(v) => setList(v as VoiceRuleList)}>
          <SelectTrigger size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {VOICE_LISTS.map((l) => (
              <SelectItem key={l} value={l} className="capitalize">
                {l}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label className="text-xs" htmlFor="voice-term">
          Term
        </Label>
        <Input
          id="voice-term"
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          placeholder="e.g. utilize"
        />
      </div>
      {mode === "add" && list === "preferred" && (
        <div className="space-y-1.5">
          <Label className="text-xs" htmlFor="voice-repl">
            Replacement (optional)
          </Label>
          <Input
            id="voice-repl"
            value={replacement}
            onChange={(e) => setReplacement(e.target.value)}
            placeholder="e.g. use"
          />
        </div>
      )}
      <FormError error={error} />
      <AddButton busy={busy} disabled={!valid} />
    </form>
  );
}

// ── Edit concept (concept.update) ────────────────────────────────────────────

function EditConceptForm({ busy, error, onSubmit }: FormProps) {
  const { concepts, isLoading } = useConceptOptions();
  const [conceptId, setConceptId] = useState("");
  const [definition, setDefinition] = useState("");
  const [domain, setDomain] = useState("");
  const concept = concepts.find((c) => c.id === conceptId);

  const onPick = (v: string) => {
    setConceptId(v);
    const c = concepts.find((x) => x.id === v);
    setDefinition(c?.definition ?? "");
    setDomain(c?.domain ?? "");
  };

  const changed =
    concept &&
    (definition.trim() !== (concept.definition ?? "") || domain.trim() !== (concept.domain ?? ""));

  const reset = () => {
    setConceptId("");
    setDefinition("");
    setDomain("");
  };

  const handle = (e: React.FormEvent) => {
    e.preventDefault();
    if (!concept || !changed) return;
    onSubmit(
      {
        op: "concept.update",
        payload: {
          concept_id: concept.id,
          definition:
            definition.trim() !== (concept.definition ?? "") ? definition.trim() : undefined,
          domain: domain.trim() !== (concept.domain ?? "") ? domain.trim() : undefined,
        },
      },
      reset,
    );
  };

  return (
    <form onSubmit={handle} className="space-y-3">
      <p className="text-sm font-medium">Edit a concept</p>
      <ConceptPicker
        label="Concept"
        value={conceptId}
        onChange={onPick}
        concepts={concepts}
        loading={isLoading}
      />
      {concept && (
        <>
          <div className="space-y-1.5">
            <Label className="text-xs" htmlFor="edit-domain">
              Domain
            </Label>
            <Input
              id="edit-domain"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="e.g. commerce"
            />
          </div>
          <div className="space-y-1.5">
            <Label className="text-xs" htmlFor="edit-def">
              Definition
            </Label>
            <Textarea
              id="edit-def"
              value={definition}
              onChange={(e) => setDefinition(e.target.value)}
              rows={3}
            />
          </div>
        </>
      )}
      <FormError error={error} />
      <AddButton busy={busy} disabled={!changed} />
    </form>
  );
}

// ── Concept picker ───────────────────────────────────────────────────────────

function ConceptPicker({
  label,
  value,
  onChange,
  concepts,
  loading,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  concepts: ConceptInfo[];
  loading: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs">{label}</Label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger size="sm">
          <SelectValue placeholder={loading ? "Loading…" : "Choose a concept…"} />
        </SelectTrigger>
        <SelectContent>
          {concepts.map((c) => (
            <SelectItem key={c.id} value={c.id}>
              {conceptName(c)}
              {c.domain ? ` · ${c.domain}` : ""}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
