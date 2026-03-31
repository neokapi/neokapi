import { useState } from "react";
import {
  BookOpen,
  ChevronDown,
  ChevronRight,
  AlertTriangle,
  Lightbulb,
  Code2,
  ExternalLink,
  Info,
  GitBranch,
} from "lucide-react";
import type { FilterDoc, StepDoc, ParameterDoc } from "../types/api";

type DocEntry = FilterDoc | StepDoc;

interface DocsPanelProps {
  /** Documentation entry for a filter or step. */
  doc: DocEntry;
  /** If provided, only show help for these parameter keys (contextual mode). */
  visibleParams?: string[];
  /** Render in compact inline mode (no card wrapper). */
  inline?: boolean;
}

/**
 * Rich documentation panel for format filters and pipeline steps.
 *
 * Displays overview, parameter documentation with dependency indicators,
 * examples with input/output, limitations, and processing notes.
 * Supports both full-page and compact inline modes.
 */
export function DocsPanel({ doc, visibleParams, inline }: DocsPanelProps) {
  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set(["overview"]),
  );

  const toggleSection = (id: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const isExpanded = (id: string) => expandedSections.has(id);

  const params = doc.parameters ? Object.entries(doc.parameters) : [];
  const filteredParams = visibleParams
    ? params.filter(([key]) => visibleParams.some((vp) => key === vp || key.startsWith(vp + ".")))
    : params;

  // Separate top-level params from nested (dot-path) params
  const topLevelParams = filteredParams.filter(([key]) => !key.includes("."));
  const nestedParams = filteredParams.filter(([key]) => key.includes("."));

  // Group nested params by their parent
  const paramGroups = new Map<string, Array<[string, ParameterDoc]>>();
  for (const [key, paramDoc] of nestedParams) {
    const parent = key.split(".")[0];
    if (!paramGroups.has(parent)) paramGroups.set(parent, []);
    paramGroups.get(parent)!.push([key, paramDoc]);
  }

  const hasExamples = doc.examples && doc.examples.length > 0;
  const hasLimitations = doc.limitations && doc.limitations.length > 0;
  const hasNotes = doc.processingNotes && doc.processingNotes.length > 0;
  const wikiUrl = doc.wikiUrl;

  const Wrapper = inline ? "div" : CardWrapper;

  return (
    <Wrapper>
      <div className="flex flex-col gap-0.5">
        {/* Overview */}
        <CollapsibleSection
          id="overview"
          icon={<BookOpen size={13} className="text-primary" />}
          title="Overview"
          expanded={isExpanded("overview")}
          onToggle={toggleSection}
        >
          <p className="text-[13px] leading-relaxed text-foreground/85">
            {doc.overview}
          </p>
          {wikiUrl && (
            <a
              href={wikiUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="mt-2 inline-flex items-center gap-1 text-[11px] text-primary/70 hover:text-primary transition-colors"
            >
              <ExternalLink size={10} />
              View full documentation
            </a>
          )}
        </CollapsibleSection>

        {/* Parameters */}
        {filteredParams.length > 0 && (
          <CollapsibleSection
            id="parameters"
            icon={<Code2 size={13} className="text-chart-2" />}
            title="Parameters"
            badge={filteredParams.length}
            expanded={isExpanded("parameters")}
            onToggle={toggleSection}
          >
            <div className="flex flex-col gap-3">
              {/* Top-level params */}
              {topLevelParams.map(([key, paramDoc]) => (
                <ParameterEntry
                  key={key}
                  name={key}
                  doc={paramDoc}
                  children={paramGroups.get(key)}
                />
              ))}

              {/* Nested params without a top-level parent entry */}
              {Array.from(paramGroups.entries())
                .filter(([parent]) => !topLevelParams.some(([k]) => k === parent))
                .map(([parent, children]) => (
                  <div key={parent}>
                    <div className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider mb-1.5">
                      {parent}
                    </div>
                    {children.map(([key, paramDoc]) => (
                      <ParameterEntry
                        key={key}
                        name={key.split(".").slice(1).join(".")}
                        doc={paramDoc}
                      />
                    ))}
                  </div>
                ))}
            </div>
          </CollapsibleSection>
        )}

        {/* Examples */}
        {hasExamples && (
          <CollapsibleSection
            id="examples"
            icon={<Lightbulb size={13} className="text-chart-4" />}
            title="Examples"
            badge={doc.examples!.length}
            expanded={isExpanded("examples")}
            onToggle={toggleSection}
          >
            <div className="flex flex-col gap-3">
              {doc.examples!.map((example, i) => (
                <ExampleEntry key={i} example={example} />
              ))}
            </div>
          </CollapsibleSection>
        )}

        {/* Limitations */}
        {hasLimitations && (
          <CollapsibleSection
            id="limitations"
            icon={<AlertTriangle size={13} className="text-chart-5" />}
            title="Limitations"
            expanded={isExpanded("limitations")}
            onToggle={toggleSection}
          >
            <ul className="flex flex-col gap-1.5">
              {doc.limitations!.map((lim, i) => (
                <li
                  key={i}
                  className="flex items-start gap-2 text-[12px] text-foreground/80"
                >
                  <span className="mt-1.5 h-1 w-1 rounded-full bg-chart-5/60 shrink-0" />
                  {lim}
                </li>
              ))}
            </ul>
          </CollapsibleSection>
        )}

        {/* Processing Notes */}
        {hasNotes && (
          <CollapsibleSection
            id="notes"
            icon={<Info size={13} className="text-chart-3" />}
            title="Processing Notes"
            expanded={isExpanded("notes")}
            onToggle={toggleSection}
          >
            <ul className="flex flex-col gap-1.5">
              {doc.processingNotes!.map((note, i) => (
                <li
                  key={i}
                  className="flex items-start gap-2 text-[12px] text-foreground/80"
                >
                  <span className="mt-1.5 h-1 w-1 rounded-full bg-chart-3/60 shrink-0" />
                  {note}
                </li>
              ))}
            </ul>
          </CollapsibleSection>
        )}
      </div>
    </Wrapper>
  );
}

// --- Sub-components ---

function CardWrapper({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-card overflow-hidden">
      {children}
    </div>
  );
}

function CollapsibleSection({
  id,
  icon,
  title,
  badge,
  expanded,
  onToggle,
  children,
}: {
  id: string;
  icon: React.ReactNode;
  title: string;
  badge?: number;
  expanded: boolean;
  onToggle: (id: string) => void;
  children: React.ReactNode;
}) {
  return (
    <div className="border-b border-border last:border-b-0">
      <button
        onClick={() => onToggle(id)}
        className="flex items-center gap-2 w-full px-4 py-2.5 text-left hover:bg-accent/40 transition-colors"
      >
        {expanded ? (
          <ChevronDown size={12} className="text-muted-foreground" />
        ) : (
          <ChevronRight size={12} className="text-muted-foreground" />
        )}
        {icon}
        <span className="text-xs font-semibold text-foreground">{title}</span>
        {badge !== undefined && (
          <span className="ml-auto text-[10px] px-1.5 py-px rounded-full bg-muted text-muted-foreground tabular-nums">
            {badge}
          </span>
        )}
      </button>
      {expanded && (
        <div className="px-4 pb-3 pt-0.5 ml-[22px]">{children}</div>
      )}
    </div>
  );
}

function ParameterEntry({
  name,
  doc,
  children,
}: {
  name: string;
  doc: ParameterDoc;
  children?: Array<[string, ParameterDoc]>;
}) {
  const [expanded, setExpanded] = useState(false);
  const hasChildren = children && children.length > 0;
  const hasDeps = doc.dependsOn && doc.dependsOn.length > 0;

  return (
    <div className="group">
      <div className="rounded-md border border-border/60 bg-background/50 overflow-hidden">
        {/* Parameter header */}
        <div
          className={`flex items-start gap-2 px-3 py-2 ${hasChildren ? "cursor-pointer hover:bg-accent/30 transition-colors" : ""}`}
          onClick={hasChildren ? () => setExpanded((v) => !v) : undefined}
        >
          {hasChildren && (
            <span className="mt-0.5 shrink-0">
              {expanded ? (
                <ChevronDown size={11} className="text-muted-foreground" />
              ) : (
                <ChevronRight size={11} className="text-muted-foreground" />
              )}
            </span>
          )}
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <code className="text-[11px] font-semibold text-primary bg-primary/8 px-1.5 py-0.5 rounded">
                {name}
              </code>
              {doc.introducedIn && (
                <span className="text-[9px] px-1 py-px rounded bg-chart-2/15 text-chart-2 font-medium">
                  {doc.introducedIn}
                </span>
              )}
            </div>
            <p className="mt-1 text-[11px] leading-relaxed text-foreground/75">
              {doc.description}
            </p>

            {/* Notes */}
            {doc.notes && doc.notes.length > 0 && (
              <div className="mt-1.5 flex flex-col gap-1">
                {doc.notes.map((note, i) => (
                  <div
                    key={i}
                    className="flex items-start gap-1.5 text-[10px] text-chart-4"
                  >
                    <Lightbulb size={9} className="mt-0.5 shrink-0" />
                    <span>{note}</span>
                  </div>
                ))}
              </div>
            )}

            {/* Dependencies */}
            {hasDeps && (
              <div className="mt-1.5 flex flex-wrap gap-1">
                {doc.dependsOn!.map((dep, i) => (
                  <span
                    key={i}
                    className="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-chart-3/10 text-chart-3"
                  >
                    <GitBranch size={8} />
                    <code className="font-medium">{dep.property}</code>
                    <span className="opacity-75">{dep.condition}</span>
                  </span>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Nested children */}
        {expanded && hasChildren && (
          <div className="border-t border-border/40 px-3 py-2 ml-4 flex flex-col gap-2">
            {children.map(([key, childDoc]) => (
              <ParameterEntry
                key={key}
                name={key.split(".").pop() || key}
                doc={childDoc}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function ExampleEntry({
  example,
}: {
  example: { title: string; description?: string; input?: string; output?: string };
}) {
  return (
    <div className="rounded-md border border-border/60 bg-background/50 overflow-hidden">
      <div className="px-3 py-2">
        <h4 className="text-[11px] font-semibold text-foreground">
          {example.title}
        </h4>
        {example.description && (
          <p className="mt-0.5 text-[11px] text-foreground/70">
            {example.description}
          </p>
        )}
      </div>
      {(example.input || example.output) && (
        <div className="border-t border-border/40 grid grid-cols-1 gap-0 divide-y divide-border/40">
          {example.input && (
            <div className="px-3 py-2">
              <span className="text-[9px] font-semibold text-muted-foreground uppercase tracking-wider">
                Input
              </span>
              <pre className="mt-1 text-[10px] text-foreground/80 bg-muted/40 rounded p-2 whitespace-pre-wrap break-words max-h-28 overflow-auto font-mono leading-relaxed">
                {example.input}
              </pre>
            </div>
          )}
          {example.output && (
            <div className="px-3 py-2">
              <span className="text-[9px] font-semibold text-muted-foreground uppercase tracking-wider">
                Output
              </span>
              <pre className="mt-1 text-[10px] text-foreground/80 bg-muted/40 rounded p-2 whitespace-pre-wrap break-words max-h-28 overflow-auto font-mono leading-relaxed">
                {example.output}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// --- Inline parameter help tooltip ---

interface ParamHelpProps {
  /** Parameter key to look up in docs. */
  paramKey: string;
  /** Full docs entry to search in. */
  doc?: DocEntry;
}

/**
 * Compact inline help for a single parameter.
 * Renders as a small info icon that expands the doc description on click.
 */
export function ParamHelp({ paramKey, doc }: ParamHelpProps) {
  const [open, setOpen] = useState(false);

  if (!doc?.parameters) return null;
  const paramDoc = doc.parameters[paramKey];
  if (!paramDoc) return null;

  return (
    <span className="relative inline-flex">
      <button
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        className="inline-flex items-center justify-center w-3.5 h-3.5 rounded-full bg-primary/10 text-primary hover:bg-primary/20 transition-colors"
        title="Show parameter documentation"
      >
        <Info size={8} />
      </button>
      {open && (
        <div className="absolute left-0 top-5 z-50 w-64 rounded-md border border-border bg-popover p-3 shadow-lg">
          <p className="text-[11px] leading-relaxed text-popover-foreground">
            {paramDoc.description}
          </p>
          {paramDoc.notes?.map((note, i) => (
            <p
              key={i}
              className="mt-1.5 text-[10px] text-chart-4 flex items-start gap-1"
            >
              <Lightbulb size={9} className="mt-0.5 shrink-0" />
              {note}
            </p>
          ))}
          {paramDoc.dependsOn?.map((dep, i) => (
            <p
              key={i}
              className="mt-1 text-[10px] text-chart-3 flex items-center gap-1"
            >
              <GitBranch size={8} />
              Requires <code className="font-semibold">{dep.property}</code>{" "}
              {dep.condition}
            </p>
          ))}
        </div>
      )}
    </span>
  );
}
