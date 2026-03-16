// Utilities
export { cn } from "./lib/utils";

// UI primitives (shadcn/ui)
export { SidebarProvider, SidebarInset, useSidebar } from "./components/ui/sidebar";
export { TooltipProvider } from "./components/ui/tooltip";
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
export { Alert, AlertTitle, AlertDescription } from "./components/ui/alert";
export {
  Combobox,
  ComboboxInput,
  ComboboxContent,
  ComboboxList,
  ComboboxItem,
  ComboboxEmpty,
} from "./components/ui/combobox";
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

// Icons (Lucide)
export * from "./components/icons";

// Components
export { WorkspaceRail } from "./components/WorkspaceRail";
export { WorkspaceIcon } from "./components/WorkspaceIcon";
export { AccountMenu } from "./components/AccountMenu";
export { TopBar } from "./components/TopBar";
export type { TopBarProps } from "./components/TopBar";
export { AppSidebar } from "./components/AppSidebar";
export type { AppSidebarProps, SidebarContext } from "./components/AppSidebar";
export { AppShell } from "./components/AppShell";
export type { AppShellProps } from "./components/AppShell";
export { CreateWorkspaceDialog } from "./components/CreateWorkspaceDialog";
export type { CreateWorkspaceDialogProps } from "./components/CreateWorkspaceDialog";
export { WorkspaceSwitcher } from "./components/WorkspaceSwitcher";
export { LocaleSelect, MultiLocaleSelect } from "./components/LocaleSelect";
export { ProjectDashboard } from "./components/ProjectDashboard";
export { ProjectView } from "./components/ProjectView";
export { OpenInDesktop } from "./components/OpenInDesktop";
export { TranslationEditor } from "./components/TranslationEditor";
export { TranslationDashboard } from "./components/TranslationDashboard";
export { LocaleCompletionChart } from "./components/LocaleCompletionChart";
export { WordCountChart } from "./components/WordCountChart";
export { CollectionHeatmap } from "./components/CollectionHeatmap";
export { FileProgressTable } from "./components/FileProgressTable";
export {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  type ChartConfig,
} from "./components/ui/chart";
export { TMExplorer } from "./components/tm/TMExplorer";
export { TermExplorer } from "./components/terms/TermExplorer";
export { InviteManager } from "./components/InviteManager";
export { ApiTokenManager } from "./components/ApiTokenManager";
export { WorkspaceLanguageSettings } from "./components/WorkspaceLanguageSettings";
export { AutomationsPage } from "./components/AutomationsPage";
export { AutomationRuleEditor } from "./components/AutomationRuleEditor";
export { AutomationHistory } from "./components/AutomationHistory";
export { NotificationCenter } from "./components/NotificationCenter";
export { StreamBadge } from "./components/StreamBadge";
export type { StreamBadgeProps } from "./components/StreamBadge";
export { StreamSelector } from "./components/StreamSelector";
export type { StreamSelectorProps } from "./components/StreamSelector";
export { CollectionTabs } from "./components/CollectionTabs";
export type { CollectionTabsProps } from "./components/CollectionTabs";
export { CreateCollectionDialog } from "./components/CreateCollectionDialog";
export type { CreateCollectionDialogProps } from "./components/CreateCollectionDialog";
export { FilterBar } from "./components/FilterBar";
export type {
  FilterBarProps,
  FilterToken,
  FilterField,
  FilterPreset,
} from "./components/FilterBar";
export { ConfirmDialog } from "./components/ConfirmDialog";
export type { ConfirmDialogProps } from "./components/ConfirmDialog";
export { ProjectFormDialog } from "./components/ProjectFormDialog";
export type { ProjectFormDialogProps, ProjectFormData } from "./components/ProjectFormDialog";
export { AuditLogView } from "./components/AuditLogView";
export type { AuditLogViewProps } from "./components/AuditLogView";
export { BinView } from "./components/BinView";
export type { BinViewProps } from "./components/BinView";
export { StreamCreateDialog } from "./components/StreamCreateDialog";
export type { StreamCreateDialogProps } from "./components/StreamCreateDialog";
export { StreamDiffView } from "./components/StreamDiffView";
export type { StreamDiffViewProps } from "./components/StreamDiffView";
export { StreamEditDialog } from "./components/StreamEditDialog";
export type { StreamEditDialogProps } from "./components/StreamEditDialog";
export { StreamMergeDialog } from "./components/StreamMergeDialog";
export type { StreamMergeDialogProps } from "./components/StreamMergeDialog";

// Skeletons
export {
  DashboardSkeleton,
  ProjectDetailSkeleton,
  EditorSkeleton,
  TablePageSkeleton,
  BrandProfilesSkeleton,
  SettingsSkeleton,
  ExplorerSkeleton,
  TranslationDashboardSkeleton,
} from "./components/skeletons";

// Editor components
export { HighlightedSource } from "./components/editor/HighlightedSource";
export { entityLabel } from "./components/editor/HighlightedSource";
export { EntityPopover } from "./components/editor/EntityPopover";
export { EntityMarkPopover } from "./components/editor/EntityMarkPopover";
export { SourceCellDisplay } from "./components/editor/SourceCellDisplay";
export { FormattedSourceDisplay } from "./components/editor/FormattedSourceDisplay";
export { TargetCellEditor } from "./components/editor/TargetCellEditor";
export { TagChipNode, $createTagChipNode, $isTagChipNode } from "./components/editor/TagChipNode";
export { TagChipComponent } from "./components/editor/TagChipComponent";
export { TagPalette } from "./components/editor/TagPalette";
export { TagValidationBar } from "./components/editor/TagValidationBar";
export { InlinePreview } from "./components/editor/InlinePreview";
export { DocumentPreview } from "./components/editor/DocumentPreview";
export type { PreviewContentMode } from "./components/editor/visual-editor-types";

// Editor utilities
export { parseCodedSegments, segmentsToCodedText, spanLabel } from "./components/editor/codedText";
export type { CodedSegment } from "./components/editor/codedText";
export {
  semanticCategory,
  semanticLabel,
  semanticTooltip,
  tagColors,
  tagNameFromData,
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
export { VocabularyRegistry, getDefaultRegistry } from "./vocabularies";
export type { SpanTypeInfo, ColorScheme, SpanConstraints } from "./vocabularies";

// Context
export { AuthProvider, useAuth } from "./context/AuthContext";
export { WorkspaceProvider, useWorkspace } from "./context/WorkspaceContext";
export { ApiProvider, useApi } from "./context/ApiContext";
export { ThemeProvider, useTheme } from "./context/ThemeContext";
export type { Theme } from "./context/ThemeContext";
export { BreadcrumbProvider, useBreadcrumb, useSetBreadcrumb } from "./context/BreadcrumbContext";
export { StreamProvider, useStream } from "./context/StreamContext";
export { StreamActionsProvider, useStreamActions } from "./context/StreamActionsContext";

// API
export type { ApiAdapter } from "./api/adapter";
export { RestApiAdapter } from "./api/rest-adapter";

// Collaboration
export { PresenceAvatars } from "./components/PresenceAvatars";
export type { PresenceAvatarsProps } from "./components/PresenceAvatars";
export { useCollaboration } from "./hooks/useCollaboration";
export type {
  CollabUser,
  CollabConnectionState,
  UseCollaborationOptions,
} from "./hooks/useCollaboration";

// Hooks
export { useProjectApi } from "./hooks/useProjectApi";
export { useEditorApi } from "./hooks/useEditorApi";
export { useTMApi } from "./hooks/useTMApi";
export { useTermsApi } from "./hooks/useTermsApi";
export { useProviderConfigs, useProviderApi } from "./hooks/useProviderApi";
export { useLocales } from "./hooks/useLocales";
export { useFormats } from "./hooks/useFormats";
export { useTools } from "./hooks/useTools";
export { useNotificationApi } from "./hooks/useNotificationApi";
export { useNotifications } from "./hooks/useNotifications";
export { useIsMobile } from "./hooks/useIsMobile";

// Types
export type {
  User,
  Workspace,
  Membership,
  ProjectInfo,
  ProjectItem,
  ConfigResponse,
  WebVersionInfo,
  SpanInfo,
  BlockInfo,
  UpdateBlockRequest,
  UpdateBlockTargetCodedRequest,
  AITranslateFileRequest,
  TranslationStats,
  WordCountResult,
  ProviderConfig,
  ProviderConfigWithKey,
  TMEntryInfo,
  TMSearchResult,
  TMUpdateRequest,
  TMMatchInfo,
  TermInfo,
  ConceptInfo,
  TermSearchResult,
  AddConceptRequest,
  UpdateConceptRequest,
  BlockTermMatch,
  TermEnforceResult,
  BlockNote,
  BlockHistoryEntry,
  QAIssue,
  FileQAResult,
  LocaleInfo,
  FormatInfo,
  ToolInfo,
  FlowNodePosition,
  FlowNodeInfo,
  FlowEdgeInfo,
  FlowDefinitionInfo,
  Invite,
  AcceptInviteResponse,
  ClaimProjectResponse,
  ApiToken,
  CreateApiTokenResponse,
  AutomationRule,
  AutomationCondition,
  AutomationAction,
  AutomationEvent,
  SaveAutomationRuleRequest,
  AutomationHistoryEntry,
  EntityInfo,
  NotificationInfo,
  StreamInfo,
  StreamVisibility,
  BlockChangeInfo,
  StreamDiffResult,
  StreamMergeResult,
  CreateStreamRequest,
  CollectionInfo,
  CollectionKind,
  CreateCollectionRequest,
  AuditEntry,
  AuditQuery,
  ArchivedProject,
  TranslationDashboardStats,
  LocaleTranslationStats,
  ItemTranslationStats,
  CollectionTranslationStats,
} from "./types/api";
export type { View, NavItem } from "./components/AppSidebar";

// Brand voice
export {
  BrandProfileCard,
  BrandScoreGauge,
  BrandFindingsList,
  BrandDimensionBreakdown,
  BrandExamplePair,
  BrandProfileEditor,
  BrandProfileList,
  BrandDashboard,
  BrandMCPGuide,
} from "./brand";
export type {
  VoiceProfile,
  ToneProfile,
  StyleRules,
  Pattern,
  VocabularyRules,
  TermRule,
  VoiceExample,
  LocaleOverride,
  ChannelOverride,
  Dimension,
  BrandSeverity,
  BrandVoiceFinding,
  DimensionScore,
  BrandComplianceScore,
  StoredScore,
  ScoreTrend,
  CreateVoiceProfileRequest,
  UpdateVoiceProfileRequest,
} from "./brand";

// Brand voice hooks
export {
  useBrandProfiles,
  useBrandProfile,
  useCreateBrandProfile,
  useUpdateBrandProfile,
  useDeleteBrandProfile,
  useBrandScores,
  useBrandTrends,
} from "./hooks/useBrandApi";

// Filter config editor
export { FilterConfigEditor } from "./components/filter";
export type {
  FilterSchema,
  FilterMeta,
  ParameterGroup,
  PropertySchema,
  WidgetType,
  ShowIfCondition,
  CodeFinderRulesValue,
  FilterParamsValue,
} from "./components/filter";
