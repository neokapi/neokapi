// Shared UI primitives — used by both framework (kapi-desktop) and platform (bowrain).
// This package contains only framework-safe components with zero platform dependencies.

// Utility
export { cn } from "./lib/utils";

// shadcn/ui primitives
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
export { Input } from "./components/ui/input";
export { Label } from "./components/ui/label";
export { Badge, badgeVariants } from "./components/ui/badge";
export { Separator } from "./components/ui/separator";
export { Tabs, TabsList, TabsTrigger, TabsContent } from "./components/ui/tabs";
export {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "./components/ui/select";
export { Switch } from "./components/ui/switch";
export { Collapsible, CollapsibleTrigger, CollapsibleContent } from "./components/ui/collapsible";
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
export { Skeleton } from "./components/ui/skeleton";
export { Textarea } from "./components/ui/textarea";
export { TooltipProvider } from "./components/ui/tooltip";

// Types
export type { SpanInfo } from "./types/span";

// Vocabulary registry
export { VocabularyRegistry, getDefaultRegistry } from "./vocabularies";
export type { SpanTypeInfo, ColorScheme, SpanConstraints } from "./vocabularies";

// Editor primitives — inline code rendering
export { TagChipComponent } from "./components/editor/TagChipComponent";
export { parseCodedSegments, segmentsToCodedText, spanLabel } from "./components/editor/codedText";
export type { CodedSegment } from "./components/editor/codedText";
export {
  tagColors,
  semanticLabel,
  semanticTooltip,
  semanticCategory,
  buildPairs,
  validateTags,
  codedTextToHtml,
} from "./components/editor/tagSemantics";
export type { TagColorScheme, TagValidationResult } from "./components/editor/tagSemantics";
export { resolveConstraints, isDeletable, isCloneable } from "./components/editor/tagConstraints";
export type { ResolvedConstraints } from "./components/editor/tagConstraints";

// Resource browser — TM and Termbase management
export {
  TMBrowser,
  TermbaseBrowser,
  TMLookupPanel,
  EntityAnnotationDialog,
  CodedTextDisplay,
  MatchScoreBar,
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
  TMSearchResult,
  TMStats,
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

// Filter config editor (dynamic forms from JSON schema)
export { FilterConfigEditor, SchemaConfigEditor } from "./components/filter";
export type {
  ComponentSchema,
  FormatSchema,
  FormatMeta,
  ToolMeta,
  ConditionExpr,
  ParameterGroup,
  PropertySchema,
  FormatParamsValue,
} from "./components/filter/types";
