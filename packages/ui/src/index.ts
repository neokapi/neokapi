// Shared UI primitives — single source of truth for all neokapi UI.
// Used by both framework (kapi-desktop) and platform (bowrain).

// Utility
export { cn } from "./lib/utils";
export { PortalThemeProvider, usePortalThemeClass } from "./lib/portal-theme";

// Hooks
export { useIsMobile } from "./hooks/use-mobile";

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
export { Badge, badgeVariants } from "./components/ui/badge";
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
export { Toaster } from "./components/ui/sonner";
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
export { TooltipProvider, Tooltip, TooltipTrigger, TooltipContent } from "./components/ui/tooltip";
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
export { GlobInput, type GlobInputProps } from "./components/ui/glob-input";
export { TargetPathInput, type TargetPathInputProps } from "./components/ui/target-path-input";
export {
  SelectableList,
  type SelectableListProps,
  type SelectableListColumn,
  type SelectableListAction,
} from "./components/ui/selectable-list";
export { ItemCard, type ItemCardProps } from "./components/ui/item-card";
export {
  ConfirmDeleteButton,
  type ConfirmDeleteButtonProps,
} from "./components/ui/confirm-delete-button";
export { ActionCard, type ActionCardProps } from "./components/ui/action-card";
export {
  FilterBar,
  type FilterBarProps,
  type FilterToken,
  type FilterField,
  type FilterPreset,
} from "./components/ui/filter-bar";
export {
  FormatSelect,
  type FormatInfo as FormatSelectInfo,
  type FormatSelectProps,
} from "./components/ui/format-select";
export {
  LocaleSelect,
  MultiLocaleSelect,
  resolveLocaleName,
  type LocaleInfo,
  type LocaleSelectProps,
  type MultiLocaleSelectProps,
} from "./components/ui/locale-select";

// Tag input (chip-based)
export { TagInput, type TagInputProps } from "./components/ui/tag-input";

// Layout components
export { PageHeader } from "./components/PageHeader";
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
export type { InlineCodeEditorProps } from "./components/editor/InlineCodeEditor";
export { TagChipNode, $createTagChipNode, $isTagChipNode } from "./components/editor/TagChipNode";
export { TagPalette } from "./components/editor/TagPalette";
export { TagValidationBar } from "./components/editor/TagValidationBar";
export { InlineCodeLegend } from "./components/editor/InlineCodeLegend";
export { InlinePreview } from "./components/editor/InlinePreview";

// Plural / Select target editor — flat ↔ per-form upgrade affordance
export { PluralTargetEditor } from "./components/plural/PluralTargetEditor";
export type { PluralTargetEditorProps } from "./components/plural/PluralTargetEditor";
export { runsToText, textToRuns } from "./components/plural/runs-text";

// Resource browser — TM and Termbase management
export {
  TMBrowser,
  TermbaseBrowser,
  TMSearchBar,
  TMFacetSidebar,
  EMPTY_FACETS,
  type FacetSelection,
  TMGroupedEntry,
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
  TMAdapter,
  TermbaseAdapter,
  TMEntryDTO,
  VariantDTO,
  VariantInputDTO,
  EntityMappingDTO,
  EntityValueDTO,
  TMSearchResult,
  TMStats,
  TMFacets,
  LocaleFacet,
  ProjectFacet,
  EntityTypeFacet,
  ImportSessionFacet,
  ImportSessionDTO,
  TMSearchFilter,
  EntityValueFilter,
  OriginDTO,
  TMMatchDTO,
  EntityAdaptationDTO,
  EntityAnnotationDTO,
  LookupTMRequest,
  AddTMEntryRequest,
  UpdateTMEntryRequest,
  AnnotateEntitiesRequest,
  EntityPatternRequest,
  AnnotateResult,
  ConceptDTO,
  TermDTO,
  TermSearchResult,
  TermbaseStats,
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

// Filter config editor (legacy — prefer SchemaForm)
export { FilterConfigEditor, SchemaConfigEditor } from "./components/filter";
export type { FormatSchema, FormatParamsValue } from "./components/filter/types";
