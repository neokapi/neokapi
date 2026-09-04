// Shared UI primitives — single source of truth for all neokapi UI.
// Used by both framework (kapi-desktop) and platform (bowrain).

// Utility
export { cn } from "./lib/utils";
export { PortalThemeProvider, usePortalThemeClass } from "./lib/portal-theme";
// Relative-time and byte-size formatters, the single reachable copies. Every
// surface that shows "2 hours ago" or "13.4 KB" imports these rather than
// growing its own; see lib/when.ts and preview/download.ts.
export { relativeTime } from "./lib/when";
export { formatSize } from "./components/resource-browser/utils";
export { formatBytes } from "./components/preview/download";

// Hooks
export { useIsMobile } from "./hooks/use-mobile";
export { useDebounced } from "./hooks/use-debounced";

// Writing direction — locale → `dir`/`lang`, plus the bidi-isolation predicate.
// Every surface that renders a Block's or Run's own source or target text
// should render it through DirectionalText (never a bare <span>/<div>/<li>/…)
// so dir/lang land on the element CSS text-align actually resolves against,
// rather than deriving the app's own direction. `directionAttrs`/
// `localeDirection` are DirectionalText's internals, exported for the rare
// non-JSX or already-`useMemo`'d-attrs call site.
export {
  localeDirection,
  isRTLLocale,
  directionAttrs,
  needsIsolation,
  localeOfVariant,
  DirectionalText,
  type TextDirection,
  type DirectionalTextProps,
} from "./lib/text-direction";

// Positions reported by the engine are UTF-8 byte offsets; a renderer slicing
// strings needs them as code-point offsets first.
export { byteToCharOffset, utf8Length } from "./lib/offsets";

// Form layout primitives
export {
  FormItem,
  FormLabel,
  FormDescription,
  FormMessage,
  FormControl,
  FormToggle,
  FormInputAction,
  FormFieldGroup,
  FormHelpText,
} from "./components/ui/form";

// Error contract — friendly-first errors with details-on-demand
export { parseAppError, type ParsedAppError } from "./lib/parse-app-error";
export { ErrorNotice, type ErrorNoticeProps } from "./components/ui/error-notice";
// Classified run failures — headline + remediation actions + affected scope,
// with the raw chain behind a disclosure. Used wherever a flow/converge run can
// fail: the desktop's home and job feed, and the platform's run surfaces.
export {
  RunErrorNotice,
  type RunErrorNoticeProps,
  type RunErrorView,
  type RunErrorActionView,
  type RunErrorActionKind,
} from "./components/ui/run-error-notice";

// List containment — honest render-cap footer for truncated listings
export { ListCapRow, type ListCapRowProps } from "./components/ui/list-cap-row";

// shadcn/ui primitives
export { Alert, AlertTitle, AlertDescription } from "./components/ui/alert";
export {
  Avatar,
  AvatarImage,
  AvatarFallback,
  AvatarBadge,
  AvatarGroup,
  AvatarGroupCount,
} from "./components/ui/avatar";
export { Button, buttonVariants } from "./components/ui/button";
export {
  Card,
  CardHeader,
  CardTitle,
  CardAction,
  CardDescription,
  CardContent,
  CardFooter,
} from "./components/ui/card";
export {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  type ChartConfig,
} from "./components/ui/chart";
export { Checkbox } from "./components/ui/checkbox";
export { Collapsible, CollapsibleTrigger, CollapsibleContent } from "./components/ui/collapsible";
export {
  Combobox,
  ComboboxInput,
  ComboboxContent,
  ComboboxList,
  ComboboxItem,
  ComboboxEmpty,
  ComboboxChips,
  ComboboxChip,
  ComboboxChipsInput,
  ComboboxGroup,
  ComboboxLabel,
  ComboboxSeparator,
} from "./components/ui/combobox";
export {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
  CommandSeparator,
} from "./components/ui/command";
export {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
  DialogClose,
} from "./components/ui/dialog";
export {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuCheckboxItem,
  DropdownMenuRadioItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuGroup,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuRadioGroup,
} from "./components/ui/dropdown-menu";
export { Input } from "./components/ui/input";
export {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupText,
  InputGroupInput,
  InputGroupTextarea,
} from "./components/ui/input-group";
export { Label } from "./components/ui/label";
export { Markdown, type MarkdownProps } from "./components/ui/markdown";
export { Badge, badgeVariants } from "./components/ui/badge";
// Shared design primitives — the vocabulary Kapi Desktop and the Bowrain
// platform draw from, so a coordinate, a status and a language look the same
// on both. See packages/ui/docs/judgement-colours.md.
export {
  CoordinateChip,
  AXES,
  AXIS_IDS,
  axisMeta,
  unknownAxis,
  type AxisId,
  type AxisMeta,
  type CoordinateChipProps,
} from "./components/ui/coordinate-chip";
export {
  StatusBadge,
  CONTENT_STATUS_LADDER,
  SOURCE_STATUS_LADDER,
  ATTENTION_STATUSES,
  STATUS_LADDERS,
  statusMeta,
  type StatusLadder,
  type ContentStatus,
  type SourceStatus,
  type AttentionStatus,
  type LadderStatus,
  type StatusTone,
  type StatusMeta,
  type StatusBadgeProps,
} from "./components/ui/status-badge";
export { LocaleLabel, type LocaleLabelProps } from "./components/ui/locale-label";
export {
  formatLocale,
  uiLocaleTag,
  type FormatLocaleOptions,
  type FormattedLocale,
  type LocaleNameVariant,
} from "./lib/locale-name";
// Instants, shown the way a language is shown: rendered in the reader's own
// language, exact in the tooltip. `When` is the component, `formatWhen` the
// same resolution outside React, and `relativeTime` its "3 minutes ago" form.
export { When, type WhenProps } from "./components/ui/when";
export {
  formatWhen,
  type FormatWhenOptions,
  type FormattedWhen,
  type WhenFieldStyle,
} from "./lib/when";
// How hard a finding bites, and the one tone that says so, shared by both
// review surfaces. See packages/ui/docs/judgement-colours.md.
export {
  findingSeverityTone,
  findingFails,
  checkIssueTone,
  findingToneBadgeClass,
  findingToneTextClass,
  findingSeverityBadgeClass,
  type FindingTone,
} from "./lib/finding-severity";
// The unit in its document, drawn identically on both review surfaces.
export {
  NeighbourhoodTable,
  type NeighbourhoodTableProps,
  type NeighbourhoodEntry,
} from "./components/ui/neighbourhood-table";
export { RunText, type RunTextProps } from "./components/ui/run-text";
export {
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverAnchor,
  PopoverHeader,
  PopoverTitle,
  PopoverDescription,
} from "./components/ui/popover";
export { ScrollArea, ScrollBar } from "./components/ui/scroll-area";
export {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  SelectGroup,
  SelectLabel,
} from "./components/ui/select";
export { Separator } from "./components/ui/separator";
export {
  Sheet,
  SheetTrigger,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
  SheetClose,
} from "./components/ui/sheet";
export {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarFooter,
  SidebarSeparator,
  SidebarTrigger,
  useSidebar,
} from "./components/ui/sidebar";
export { Skeleton } from "./components/ui/skeleton";
export { Toaster, toast } from "./components/ui/sonner";
export { Switch } from "./components/ui/switch";
export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCaption,
} from "./components/ui/table";
export { Tabs, TabsList, TabsTrigger, TabsContent } from "./components/ui/tabs";
export { Textarea } from "./components/ui/textarea";
export { Toggle, toggleVariants } from "./components/ui/toggle";
export { ToggleGroup, ToggleGroupItem } from "./components/ui/toggle-group";
export {
  TooltipProvider,
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  SimpleTooltip,
} from "./components/ui/tooltip";
export {
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbPage,
  BreadcrumbSeparator,
  BreadcrumbEllipsis,
} from "./components/ui/breadcrumb";
export { Progress } from "./components/ui/progress";

// Code editor (CodeMirror 6)
export { CodeInput, type CodeInputProps, type CodeLanguage } from "./components/ui/code-input";
export { GlobInput, type GlobInputProps } from "./components/composites/glob-input";
export {
  TargetPathInput,
  type TargetPathInputProps,
} from "./components/composites/target-path-input";
export {
  SelectableList,
  type SelectableListProps,
  type SelectableListColumn,
  type SelectableListAction,
} from "./components/composites/selectable-list";
export { ItemCard, type ItemCardProps } from "./components/ui/item-card";
export {
  ConfirmDeleteButton,
  type ConfirmDeleteButtonProps,
} from "./components/composites/confirm-delete-button";
export { ActionCard, type ActionCardProps } from "./components/composites/action-card";
export {
  FilterBar,
  type FilterBarProps,
  type FilterToken,
  type FilterField,
  type FilterPreset,
} from "./components/composites/filter-bar";
export {
  FormatSelect,
  type FormatInfo as FormatSelectInfo,
  type FormatSelectProps,
} from "./components/composites/format-select";
export {
  LocaleSelect,
  MultiLocaleSelect,
  resolveLocaleName,
  localeLabel,
  type LocaleInfo,
  type LocaleSelectProps,
  type MultiLocaleSelectProps,
} from "./components/composites/locale-select";

// Tag input (chip-based)
export { TagInput, type TagInputProps } from "./components/ui/tag-input";

// Layout components
export { PageHeader } from "./components/PageHeader";
export { SectionHeading } from "./components/SectionHeading";
export { PanelHeader } from "./components/PanelHeader";
export { LoadingSpinner } from "./components/LoadingSpinner";
export { EmptyState } from "./components/EmptyState";
export { SkeletonCard } from "./components/SkeletonCard";

// Types
export type { SpanInfo } from "./types/span";

// Run content model (re-exported from @neokapi/kapi-format so consumers
// of the resource browser can reference the wire shape without taking a
// direct dependency on the format package).
export type {
  Run,
  TextRun,
  PlaceholderRun,
  PcOpenRun,
  PcCloseRun,
  SubRun,
  PluralRun,
  SelectRun,
  RunConstraints,
} from "@neokapi/kapi-format";

// Vocabulary registry
export { VocabularyRegistry, getDefaultRegistry } from "./vocabularies";
export type { SpanTypeInfo, ColorScheme, SpanConstraints } from "./vocabularies";

// Editor primitives — inline code rendering
export { TagChipComponent } from "./components/editor/TagChipComponent";
export { parseCodedSegments, segmentsToCodedText, spanLabel } from "./components/editor/codedText";
export type { CodedSegment } from "./components/editor/codedText";
export { codedToRuns, runsToCoded } from "./components/editor/runsCodedBridge";
export { parsePluralFormForChips } from "./components/editor/pluralCellPreview";
export type { PluralCellPreview } from "./components/editor/pluralCellPreview";
export {
  tagNameFromData,
  tagColors,
  semanticLabel,
  semanticTooltip,
  semanticCategory,
  buildPairs,
  validateTags,
  codedTextToHtml,
} from "./components/editor/tagSemantics";
export type {
  SemanticCategory,
  TagColorScheme,
  SpanPairInfo,
  TagValidationResult,
  TagValidationIssue,
} from "./components/editor/tagSemantics";
export { resolveConstraints, isDeletable, isCloneable } from "./components/editor/tagConstraints";
export type { ResolvedConstraints } from "./components/editor/tagConstraints";

// Inline code editor — Lexical-based rich text editing with visual tag chips
export { InlineCodeEditor } from "./components/editor/InlineCodeEditor";
export type {
  InlineCodeEditorProps,
  InlineCodeEditorHandle,
} from "./components/editor/InlineCodeEditor";
export { TagChipNode, $createTagChipNode, $isTagChipNode } from "./components/editor/TagChipNode";
export { TagPalette } from "./components/editor/TagPalette";
export { TagValidationBar } from "./components/editor/TagValidationBar";
export { InlineCodeLegend } from "./components/editor/InlineCodeLegend";
export { InlinePreview } from "./components/editor/InlinePreview";

// Plural / Select target editor — flat ↔ per-form upgrade affordance
export { PluralTargetEditor } from "./components/plural/PluralTargetEditor";
export type { PluralTargetEditorProps } from "./components/plural/PluralTargetEditor";
export { runsToText, textToRuns } from "./components/plural/runs-text";

// Resource browser — content memory and Terms management
export {
  MemoryBrowser,
  TermsBrowser,
  MemorySearchBar,
  MemoryFacetSidebar,
  EMPTY_FACETS,
  type FacetSelection,
  MemoryGroupedEntry,
  OriginsPopover,
  EntityAnnotationDialog,
  CodedTextDisplay,
  MatchScoreBar,
  ConceptCard,
  type ConceptCardProps,
  LocalePill,
  TermStatusBadge,
  BulkActionBar,
  ResourceCard,
  ImportProgress,
  Pagination,
  ENTITY_TYPES,
} from "./components/resource-browser";
export type {
  MemoryAdapter,
  TermsAdapter,
  MemoryEntryDTO,
  MemoryPointDTO,
  VariantDTO,
  VariantInputDTO,
  EntityMappingDTO,
  EntityValueDTO,
  MemorySearchResult,
  MemoryStats,
  MemoryFacets,
  LocaleFacet,
  ProjectFacet,
  EntityTypeFacet,
  ImportSessionFacet,
  ImportSessionDTO,
  MemorySearchFilter,
  EntityValueFilter,
  OriginDTO,
  MemoryMatchDTO,
  EntityAdaptationDTO,
  EntityAnnotationDTO,
  LookupMemoryRequest,
  AddMemoryEntryRequest,
  UpdateMemoryEntryRequest,
  AnnotateEntitiesRequest,
  EntityPatternRequest,
  AnnotateResult,
  ConceptDTO,
  TermDTO,
  TermSearchResult,
  TermsStats,
  AddConceptRequest,
  UpdateConceptRequest,
  ImportResult,
  ResourceInfo,
} from "./components/resource-browser";

// CodeFinder editor
export {
  CodeFinderEditor,
  type CodeFinderEditorProps,
  type CodeFinderRulesValue,
} from "./components/ui/code-finder-editor";

// Schema-driven form (canonical form renderer for filters, tools, formats)
export { SchemaForm, SchemaFormHostProvider, useSchemaFormHost } from "./components/schema-form";
export type {
  SchemaFormProps,
  ComponentSchema,
  PropertySchema,
  ParameterGroup,
  ConditionExpr,
  LayoutHints,
  FormatMeta,
  ToolMeta,
  ToolDoc,
  ToolDocParam,
  SchemaFormHost,
  SchemaFormBrowseRequest,
  SchemaFormFileFilter,
  SchemaFormCredential,
} from "./components/schema-form";

// Linear flow editor (shared, surface-agnostic: an ordered list of tool steps)
export { LinearFlowEditor, StepRow, AddStepPicker } from "./components/flow-editor";
export type {
  LinearFlowEditorProps,
  StepRowProps,
  AddStepPickerProps,
  FlowSpec as LinearFlowSpec,
  FlowStep as LinearFlowStep,
  FlowTool as LinearFlowTool,
} from "./components/flow-editor";

// Flow list card + empty state (shared, outcome-first: name + outcome + step chips)
export { FlowCard, StepChips, FlowsEmptyState } from "./components/flows";
export type { FlowCardItem } from "./components/flows";

// Filter config editor (legacy — prefer SchemaForm)
export { FilterConfigEditor, SchemaConfigEditor } from "./components/filter";
export type { FormatSchema, FormatParamsValue } from "./components/filter/types";
