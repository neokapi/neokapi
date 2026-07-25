// All UI primitives from the single source of truth: @neokapi/ui-primitives
export {
  // Utilities
  cn,
  useIsMobile,
  // Form layout primitives
  FormItem,
  FormLabel,
  FormDescription,
  FormMessage,
  FormControl,
  FormToggle,
  FormInputAction,
  FormFieldGroup,
  FormHelpText,
  // Code editor & tag input
  CodeInput,
  TagInput,
  // Primitives
  Alert,
  AlertTitle,
  AlertDescription,
  Avatar,
  AvatarImage,
  AvatarFallback,
  Button,
  buttonVariants,
  Badge,
  badgeVariants,
  Card,
  CardHeader,
  CardTitle,
  CardAction,
  CardDescription,
  CardContent,
  CardFooter,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  ChartLegend,
  ChartLegendContent,
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
  Combobox,
  ComboboxInput,
  ComboboxContent,
  ComboboxList,
  ComboboxItem,
  ComboboxEmpty,
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
  CommandSeparator,
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
  DialogClose,
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
  Input,
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupText,
  InputGroupInput,
  InputGroupTextarea,
  Label,
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverAnchor,
  ScrollArea,
  ScrollBar,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Markdown,
  Separator,
  Sheet,
  SheetTrigger,
  SheetContent,
  SheetHeader,
  SheetFooter,
  SheetTitle,
  SheetDescription,
  SheetClose,
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
  SidebarSeparator,
  SidebarTrigger,
  useSidebar,
  Skeleton,
  Switch,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Textarea,
  TooltipProvider,
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from "@neokapi/ui-primitives";
export type { ChartConfig, MarkdownProps } from "@neokapi/ui-primitives";

// Icons (Lucide)
export * from "./components/icons";

// Shared error display: parseAppError + ErrorNotice + JSON highlighting.
export * from "./errors";

// Honest render-cap footer for capped lists.
export { ListCapRow, type ListCapRowProps } from "./components/ListCapRow";

// Components
export { WorkspaceRail } from "./components/WorkspaceRail";
export { WorkspaceIcon } from "./components/WorkspaceIcon";
export { AccountMenu } from "./components/AccountMenu";
export { NotificationSettings } from "./components/NotificationSettings";
export type { DigestSettings } from "./components/NotificationSettings";
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
export type { ProjectDashboardProps } from "./components/ProjectDashboard";
export { LoopStatusRow } from "./components/LoopStatusRow";
export type {
  LoopActivitySummary,
  LoopBrandHealth,
  LoopRunStatus,
  LoopShipStatus,
  LoopStatusData,
  LoopStatusRowProps,
} from "./components/LoopStatusRow";
export { ProjectView } from "./components/ProjectView";
export { FormattedFileName, formatIcon } from "./components/FormattedFileName";
export { OpenInDesktop } from "./components/OpenInDesktop";
export { TranslationEditor } from "./components/TranslationEditor";
export type { TranslateView } from "./components/TranslationEditor";
export { ReviewSurface } from "./components/ReviewSurface";

// Governed review session (the dedicated project-level review surface).
export { ReviewSession } from "./components/review/ReviewSession";
export type { ReviewSessionProps } from "./components/review/ReviewSession";
export { ReviewQueueList } from "./components/review/ReviewQueueList";
export type { ReviewQueueListProps } from "./components/review/ReviewQueueList";
export { FocusedReviewer } from "./components/review/FocusedReviewer";
export type { FocusedReviewerProps, ReviewerOnBrand } from "./components/review/FocusedReviewer";
export { ReviewInbox } from "./components/review/ReviewInbox";
export type { ReviewInboxProps, ReviewInboxProject } from "./components/review/ReviewInbox";
export { MarkSourceTermDialog } from "./components/review/MarkSourceTermDialog";
export type { MarkSourceTermDialogProps } from "./components/review/MarkSourceTermDialog";
export { SuggestBrandRuleDialog } from "./components/review/SuggestBrandRuleDialog";
export type { SuggestBrandRuleDialogProps } from "./components/review/SuggestBrandRuleDialog";
export { ProposeSourceChangeDialog } from "./components/review/ProposeSourceChangeDialog";
export type { ProposeSourceChangeDialogProps } from "./components/review/ProposeSourceChangeDialog";
export { SourceProposalsDialog } from "./components/review/SourceProposalsDialog";
export type { SourceProposalsDialogProps } from "./components/review/SourceProposalsDialog";
export {
  entryKey,
  entryCheckStatus,
  isPendingReview,
  isEntryPassing,
  filterEntries,
  groupEntries,
  queueCounts,
  passingCount,
} from "./components/review/reviewQueue";
export type {
  ReviewEntry,
  ReviewGroup,
  ReviewGroupBy,
  ReviewCheckStatus,
  ReviewQueueFilter,
  ReviewQueueCounts,
} from "./components/review/reviewQueue";
export { normalizeServerBlock, normalizeServerBlocks } from "./components/editor/blockRuns";
export type { ServerBlockInfo } from "./components/editor/blockRuns";
export { PreProcessSurface } from "./components/PreProcessSurface";
export { TranslationDashboard } from "./components/TranslationDashboard";
export { ShipStateBadge, type ShipStateBadgeProps } from "./components/ShipStateBadge";
export { ProjectTypeBadge, type ProjectTypeBadgeProps } from "./components/ProjectTypeBadge";
export { OnBrandRateChip, type OnBrandRateChipProps } from "./components/OnBrandRateChip";
export {
  DeliveryPanel,
  type DeliveryPanelProps,
  type DeliveryConnectorStatus,
} from "./components/DeliveryPanel";
export { UnifiedTargetEditor } from "./components/UnifiedTargetEditor";
export type {
  UnifiedTargetEditorProps,
  UnifiedTargetEditorHandle,
  UnifiedSaveResult,
} from "./components/UnifiedTargetEditor";
export { toKapiBlock } from "./components/blockAdapter";
export { LanguageLabel, localeDisplayName } from "./components/LanguageLabel";
export { LocaleCompletionChart } from "./components/LocaleCompletionChart";
export { WordCountChart } from "./components/WordCountChart";
export { CollectionHeatmap } from "./components/CollectionHeatmap";
export { FileProgressTable, type FileProgressPaging } from "./components/FileProgressTable";
// Chart components re-exported from @neokapi/ui-primitives above
// content memory + terms browsers are the shared Apache-licensed suites from
// @neokapi/ui-primitives, fed by bowrain's REST adapter via the
// useMemoryBrowserAdapter / useTermsBrowserAdapter hooks below.
export { MemoryBrowser, TermsBrowser } from "@neokapi/ui-primitives";
export { InviteManager } from "./components/InviteManager";
export { ApiTokenManager } from "./components/ApiTokenManager";
export { RoleTemplateManager } from "./components/RoleTemplateManager";
export { ProjectMemberManager } from "./components/ProjectMemberManager";
export { WorkspaceLanguageSettings } from "./components/WorkspaceLanguageSettings";
export { WorkspaceModelSettings } from "./components/WorkspaceModelSettings";
export { AutomationsPage } from "./components/AutomationsPage";
export { AutomationRuleEditor } from "./components/AutomationRuleEditor";
export { AutomationRunsPage } from "./components/AutomationRunsPage";
// The convergence run view AND the fold that feeds it are the shared Apache
// pieces (@neokapi/status-views), consumed by both bowrain's Runs surface and
// kapi-desktop's Runner: one event protocol, one state machine, two surfaces.
// Re-exported here so bowrain's apps keep importing their UI from one place.
export { ConvergenceRunView } from "@neokapi/status-views";
export type { ConvergenceRunViewProps } from "@neokapi/status-views";
export { ConvergenceRunsList } from "./components/ConvergenceRunsList";
export type { ConvergenceRunsListProps } from "./components/ConvergenceRunsList";
export { ConvergenceRunContext } from "./components/ConvergenceRunContext";
export type { ConvergenceRunContextProps } from "./components/ConvergenceRunContext";
export { ConvergenceRunNowDialog } from "./components/ConvergenceRunNowDialog";
export type { ConvergenceRunNowDialogProps } from "./components/ConvergenceRunNowDialog";
export { reduceRun, applyEvent, emptyRunModel } from "@neokapi/status-views";
export type {
  ConvergenceRunModel,
  ConvergencePassView,
  ConvergenceLocaleRow,
  LocaleRowState,
} from "@neokapi/status-views";
export { AutomationHistory } from "./components/AutomationHistory";
export { NotificationCenter } from "./components/NotificationCenter";
export { ActivityIndicator, TaskIndicator } from "./components/ActivityTaskIndicators";
export type {
  ActivityIndicatorProps,
  TaskIndicatorProps,
} from "./components/ActivityTaskIndicators";
export { StreamBadge } from "./components/StreamBadge";
export type { StreamBadgeProps } from "./components/StreamBadge";
export { StreamTagBadge } from "./components/StreamTagBadge";
export type { StreamTagBadgeProps } from "./components/StreamTagBadge";
export { StreamTagList } from "./components/StreamTagList";
export type { StreamTagListProps } from "./components/StreamTagList";
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
export { ConnectorsPanel } from "./components/ConnectorsPanel";
export type { ConnectorsPanelProps } from "./components/ConnectorsPanel";
export { ProjectFormDialog } from "./components/ProjectFormDialog";
export type { ProjectFormDialogProps, ProjectFormData } from "./components/ProjectFormDialog";
export { AuditLogView } from "./components/AuditLogView";
export type { AuditLogViewProps } from "./components/AuditLogView";
export { BlockHistoryPanel } from "./components/BlockHistoryPanel";
export type { BlockHistoryPanelProps } from "./components/BlockHistoryPanel";
export { GovernanceSettings } from "./components/GovernanceSettings";
export { ActivityFeed } from "./components/ActivityFeed";
export type { ActivityFeedProps } from "./components/ActivityFeed";
export { TaskBoard } from "./components/TaskBoard";
export type { TaskBoardProps } from "./components/TaskBoard";
export { RecycleBinView } from "./components/RecycleBinView";
export type { RecycleBinViewProps } from "./components/RecycleBinView";
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
  ActivityFeedSkeleton,
  TaskBoardSkeleton,
} from "./components/skeletons";

// Editor components
export { HighlightedSource } from "./components/editor/HighlightedSource";
export { entityLabel } from "./components/editor/HighlightedSource";
export { EntityPopover } from "./components/editor/EntityPopover";
export { EntityMarkPopover } from "./components/editor/EntityMarkPopover";
export { SourceCellDisplay } from "./components/editor/SourceCellDisplay";
export { FormattedSourceDisplay } from "./components/editor/FormattedSourceDisplay";
// Editor components — re-exported from shared @neokapi/ui-primitives
export { InlineCodeEditor as TargetCellEditor } from "@neokapi/ui-primitives";
export { TagChipNode, $createTagChipNode, $isTagChipNode } from "@neokapi/ui-primitives";
export { TagChipComponent } from "@neokapi/ui-primitives";
export { TagPalette } from "@neokapi/ui-primitives";
export { TagValidationBar } from "@neokapi/ui-primitives";
export { InlinePreview } from "@neokapi/ui-primitives";
export { InlineCodeLegend } from "@neokapi/ui-primitives";
export { DocumentPreview } from "./components/editor/DocumentPreview";
export type { PreviewContentMode } from "./components/editor/visual-editor-types";
export { ContextPanel } from "./components/editor/ContextPanel";
export { EditorSurfaceTabs } from "./components/editor/EditorSurfaceTabs";
export type { EditorSurface } from "./components/editor/EditorSurfaceTabs";
export { TableView } from "./components/editor/TableView";
export { CollapsedTargetCell, RowTagWarning } from "./components/editor/GridTargetRenderer";
export {
  getBlockStatus,
  getTargetStatus,
  getTargetText,
  withTargetEntry,
  withTargetStatus,
  statusLabel,
  statusDotClass,
  statusBorderClass,
  statusBadgeClass,
  statusConfig,
  memoryScoreClass,
  termStatusClass,
} from "./components/editor/blockStatus";
export type { BlockStatus } from "./components/editor/blockStatus";

// Editor utilities — re-exported from shared @neokapi/ui-primitives
export { parseCodedSegments, segmentsToCodedText, spanLabel } from "@neokapi/ui-primitives";
export type { CodedSegment } from "@neokapi/ui-primitives";
export {
  semanticCategory,
  semanticLabel,
  semanticTooltip,
  tagColors,
  tagNameFromData,
  buildPairs,
  validateTags,
  codedTextToHtml,
} from "@neokapi/ui-primitives";
export type {
  SemanticCategory,
  TagColorScheme,
  SpanPairInfo,
  TagValidationResult,
  TagValidationIssue,
} from "@neokapi/ui-primitives";
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
export { AnalyticsProvider, useAnalytics } from "./context/AnalyticsContext";
export type { AnalyticsCaptureFn, AnalyticsContextValue } from "./context/AnalyticsContext";

// Product analytics event taxonomy (epic 018, workstream B)
export { AnalyticsEvents } from "./analytics-events";
export type { AnalyticsEventName } from "./analytics-events";
export { countBucket, creditBucket, percentBucket, sharePercentBucket } from "./analytics-buckets";

// API
export type { ApiAdapter } from "./api/adapter";
export { RestApiAdapter, type ApiTransport } from "./api/rest-adapter";

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
export { useProjectApi, useProjects } from "./hooks/useProjectApi";
export { useEditorApi } from "./hooks/useEditorApi";
export {
  useMemoryBrowserAdapter,
  useTermsBrowserAdapter,
} from "./hooks/useResourceBrowserAdapters";
export { useProviderConfigs, useProviderApi } from "./hooks/useProviderApi";
export { useLocales } from "./hooks/useLocales";
export { useFormats } from "./hooks/useFormats";
export { useTools } from "./hooks/useTools";
// useIsMobile re-exported from @neokapi/ui-primitives above

// Types
export type {
  User,
  Workspace,
  Membership,
  ProjectInfo,
  ProjectItem,
  SkippedFile,
  UploadFilesResult,
  ConfigResponse,
  ProviderTypeInfo,
  PublicPlatformConfig,
  PlatformModel,
  PlatformMaintenance,
  WebVersionInfo,
  SpanInfo,
  BlockInfo,
  TargetEntry,
  TargetInfo,
  TargetStatus,
  ReviewDemotion,
  ApprovePassingRequest,
  ApprovePassingResult,
  UpdateBlockRequest,
  UpdateBlockTargetCodedRequest,
  AITranslateFileRequest,
  TranslationStats,
  WordCountResult,
  ProviderConfig,
  ProviderConfigWithKey,
  MemoryEntryInfo,
  MemorySearchResult,
  MemoryUpdateRequest,
  MemoryMatchInfo,
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
  AutomationRun,
  AutomationStep,
  AutomationLogEntry,
  RunStatus,
  StepStatus,
  EntityInfo,
  NotificationInfo,
  StreamInfo,
  DashboardVisibility,
  StreamVisibility,
  BlockChangeInfo,
  StreamDiffResult,
  StreamMergeResult,
  CreateStreamRequest,
  StreamTag,
  StreamTagKind,
  CreateStreamTagRequest,
  CollectionInfo,
  CollectionKind,
  CreateCollectionRequest,
  AuditEntry,
  AuditQuery,
  AuditChainVerification,
  BlockWorkflowStatus,
  SoDMode,
  Group,
  GroupRoleBinding,
  DenyRule,
  DenyRuleInput,
  RestorePointOptions,
  ArchivedProject,
  TranslationDashboardStats,
  TranslationDashboardItemOpts,
  DashboardItemSort,
  ShipState,
  OnBrandBasis,
  LocaleTranslationStats,
  ItemTranslationStats,
  CollectionTranslationStats,
  ActivityInfo,
  ConvergenceRun,
  ConvergenceRunState,
  ConvergenceLocaleStanding,
  ConvergenceEstimate,
  ConvergenceSourceReadiness,
  ConvergenceEstimateLocale,
  ConvergenceEstimateTotals,
  ConvergenceEstimateCredits,
  ConvergenceRunScope,
  ConvergenceEvent,
  ConvergenceEventType,
  LoopRollup,
  LoopRollupRun,
  LoopRollupShip,
  LoopRollupShipProject,
  TaskInfo,
  TaskType,
  TaskStatus,
  TaskPriority,
  CreateTaskRequest,
  RoleTemplate,
  ProjectMembership,
  NotificationPreference,
  DigestSettingsDTO,
  BravoConversation,
  BravoMessage,
  BravoToolCall as BravoToolCallInfo,
  BravoConfig,
  BravoToolInfo,
  BravoToolInfo as BravoToolListItem,
  BravoUsageSummary,
  BravoSSEEventType,
  BravoSSEHandler,
  BravoSSEMessageStart,
  BravoSSEContentDelta,
  BravoSSEToolCallStart,
  BravoSSEToolCallEnd,
  BravoSSENeedsApproval,
  BravoSSEMessageEnd,
  BravoSSEError,
  BillingPlan,
  BillingStatus,
  BillingSubscription,
  CreditAllocation,
  BillingOverview,
  BillingPlanInfo,
  BillingPlansResponse,
  CreditLedgerEntry,
  BillingUsageBreakdown,
  ModelUsage,
  ModelUsageResponse,
  RunnerUsage,
  OnboardingStatus,
  SlugCheckResponse,
  EmailChangeRequestResponse,
  EmailChangeConfirmResponse,
  AccountSecurity,
  Passkey,
  PasskeyListResponse,
  PasskeyRegisterStartResponse,
  PasskeyRegisterFinishRequest,
  SlugReservation,
  ConnectorInfo,
  ConnectorSyncStatus,
  ConnectorContentItem,
  PostHogConnectorConfig,
  PostHogConnectorConfigRequest,
  PostHogLanguageSessions,
  PostHogCountryDemand,
  PostHogTrendPoint,
  PostHogLanguageDemandInfo,
  PostHogDemandSourceInfo,
  PostHogDemandResponse,
  InstallationRepo,
  BindInstallationRepoRequest,
  BindInstallationRepoResult,
  RepoDetection,
  RepoDetectOptions,
  RepoContentSignal,
  ModelSweepMeasurement,
  ModelSweepLocaleGroup,
  ModelRecommendationsResponse,
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
  BrandProfileWizard,
  StarterPackPicker,
  StarterPackCard,
  ToneSpectrumSelector,
  PersonalityTagPicker,
  BrandVoicePreview,
  PatternListEditor,
  VocabularyEditor,
  ExamplesEditor,
  CandidateRulesList,
  BlastRadiusSummary,
  DriftAlert,
  starterPacks,
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
  BrandCorrectionRequest,
  BrandCorrectionResult,
  BrandSeverity,
  BrandVoiceFinding,
  DimensionScore,
  BrandComplianceScore,
  StoredScore,
  ScoreTrend,
  CreateVoiceProfileRequest,
  UpdateVoiceProfileRequest,
  SuggestedRule,
  RuleDecisionStatus,
  CandidateRule,
  CollectionBlastRadius,
  BlastRadius,
  DriftResult,
  BrandTrend,
  BrandRollupEntry,
  BrandRollup,
  BrandRollupOptions,
  StarterPackMeta,
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
  useBrandCandidates,
  usePromoteBrandRule,
  useRejectBrandRule,
  useEvaluateBrandRule,
  useRecordBrandCorrection,
  useBrandDrift,
  useStarterPacks,
  useCreateFromStarter,
} from "./hooks/useBrandApi";

// Brand scan (AI brand onboarding — epic 016)
export {
  BrandScanInput,
  BrandScanProgress,
  BrandScanReview,
  BrandScanLiveTester,
  BrandScanLocalLaneCard,
  MAX_SCAN_URLS,
  MAX_SCAN_FILES,
} from "./brand-scan";
export type {
  BrandScanInputProps,
  BrandScanProgressProps,
  BrandScanReviewProps,
  BrandScanLiveTesterProps,
  BrandScanLocalLaneCardProps,
} from "./brand-scan";
export {
  useUploadBrandScanSources,
  useStartBrandScan,
  useBrandScanJob,
  useCheckBrandDraft,
  brandScanPollInterval,
} from "./hooks/useBrandScanApi";
export type {
  BrandScanRequest,
  BrandScanUpload,
  BrandScanUploadResult,
  BrandScanFieldEvidence,
  BrandScanEvidence,
  BrandScanTerm,
  BrandScanSource,
  BrandScanDraft,
  BrandScanStatus,
  BrandScanJob,
  BrandScanCheckResult,
} from "./types/api";

// Brand knowledge graph (AD-021) — concepts, graph, markets, change-sets
export {
  useConcepts,
  useConcept,
  useCreateConcept,
  useUpdateConcept,
  useDeleteConcept,
  useConceptStory,
  useConceptRelations,
  useAddConceptRelation,
  useDeleteConceptRelation,
  useConceptBlastRadius,
  useObservations,
  useAddObservation,
  useDeleteObservation,
  useConceptComments,
  useAddConceptComment,
  useResolveConceptComment,
  useDeleteConceptComment,
} from "./hooks/useConceptsApi";
export { useWorkspaceMembers, useUserDisplayNames } from "./hooks/useMembersApi";
export {
  useMarkets,
  useCreateMarket,
  useUpdateMarket,
  useDeleteMarket,
} from "./hooks/useMarketsApi";
export {
  useChangesets,
  useChangeset,
  useCreateChangeset,
  usePatchChangeset,
  useAppendChangesetOp,
  useRemoveChangesetOp,
  useSubmitChangeset,
  useApproveChangeset,
  useRejectChangeset,
  useMergeChangeset,
  useAbandonChangeset,
  useChangesetBlastRadius,
  useAddPilot,
  useRemovePilot,
} from "./hooks/useChangesetsApi";
// Brand hub (AD-021) — the unified Concepts/Voice/Experiments/Activity/Dashboard
// surface (shell + section views) built on the concept/graph/change-set hooks.
export {
  BrandHub,
  TermStatusBadge,
  ChangeSetStatusBadge,
  changeSetStatusLabel,
  RelationBadge,
  relationLabel,
  EmptyState as BrandHubEmptyState,
  formatDate as brandHubFormatDate,
  formatRelative as brandHubFormatRelative,
  // Concepts section — framework concept UI (R4) on @neokapi/concept-ui.
  ConceptsSection,
  ConceptStorySection,
  ConceptEditDialog,
  createRestConceptSource,
  GovernedEditError,
  isGovernedEditError,
  asGovernedEditError,
  ExperimentsView,
  ExperimentDetailView,
  ActivityView,
  BrandDashboardView,
  RollupMatrix,
  aggregateBrandRollup,
  rollupEntry,
} from "./brand-hub";
export type {
  BrandHubProps,
  ConceptsSectionProps,
  ConceptStorySectionProps,
  ConceptEditDialogProps,
  RestConceptSourceOptions,
  ExperimentsViewProps,
  ExperimentDetailViewProps,
  ActivityViewProps,
  BrandDashboardViewProps,
  RollupProjectInput,
} from "./brand-hub";

// Brand knowledge graph value exports (ordered constant arrays)
export { RELATION_TYPES, OBSERVATION_KINDS, CHANGE_SET_STATUSES } from "./types/brand-graph";
export type {
  TermStatus,
  TermSource,
  RelationType,
  Validity,
  Term,
  GraphConcept,
  ConceptRelation,
  ConceptStoryKind,
  ConceptStoryEntry,
  ConceptStory,
  ConceptRevision,
  ObservationKind,
  Observation,
  Comment,
  Market,
  OpType,
  VoiceRuleList,
  VoiceRule,
  ConceptCreatePayload,
  ConceptUpdatePayload,
  ConceptDeletePayload,
  TermAddPayload,
  TermUpdatePayload,
  TermRemovePayload,
  TermStatusPayload,
  RelationAddPayload,
  RelationRemovePayload,
  VoiceRuleAddPayload,
  VoiceRuleRemovePayload,
  ChangeSetOpPayload,
  ChangeSetOp,
  AddChangeSetOpRequest,
  ChangeSetStatus,
  ReviewVerdict,
  ChangeSet,
  ChangeSetReview,
  Pilot,
  ChangeSetDetail,
  BlockSample,
  LocaleImpact,
  CollectionImpact,
  ProjectImpact,
  ChangeSetImpact,
  LocaleUsage,
  CollectionUsage,
  ProjectUsage,
  ConceptUsage,
  OpConflict,
  MergeEvent,
  MergeResult,
  AddConceptRelationRequest,
  AddObservationRequest,
  AddCommentRequest,
  MarketRequest,
  CreateChangeSetRequest,
  UpdateChangeSetRequest,
  ReviewRequest,
  StartPilotRequest,
  ListConceptsParams,
  RelationScope,
} from "./types/brand-graph";

// Bravo (@bravo agent) — assistant-ui powered components
export {
  BravoSidebar,
  BravoAssistantThread,
  BravoToolCallRenderer,
  BravoFallbackToolUI,
  useBravoRuntime,
  useBravoThreadListAdapter,
  BravoPanelTrigger,
  BravoConversationList,
  BravoConfigPanel,
  BravoUsageDashboard,
  BravoModeSelector,
  BravoColdStart,
} from "./components/bravo";
export type {
  BravoSidebarProps,
  BravoRuntimeOptions,
  BravoThreadListOptions,
  BravoPanelTriggerProps,
  BravoConversationListProps,
  BravoConfigPanelProps,
  BravoUsageDashboardProps,
  BravoMode,
  BravoModeSelectorProps,
} from "./components/bravo";
export {
  BravoProvider,
  useBravo,
  useBravoAssistantRuntime,
  useBravoAssistantThreadList,
} from "./context/BravoContext";
export { useBravoApi } from "./hooks/useBravoApi";
export { BravoStepUpCard } from "./components/bravo/BravoStepUpCard";

// Billing
export {
  SubscriptionBadge,
  UsageBar,
  CreditCounter,
  PlanCard,
  PlanComparisonTable,
  UpgradePrompt,
  CreditLedger,
  ModelUsageTable,
} from "./components/billing";
export type {
  SubscriptionBadgeProps,
  UsageBarProps,
  CreditCounterProps,
  PlanCardProps,
  PlanFeature,
  PlanComparisonTableProps,
  ComparisonFeature,
  UpgradePromptProps,
  CreditLedgerProps,
  ModelUsageTableProps,
} from "./components/billing";

// CodeFinder editor (from primitives)
export {
  CodeFinderEditor,
  type CodeFinderEditorProps,
  type CodeFinderRulesValue,
} from "@neokapi/ui-primitives";

// Schema-driven form (from primitives)
export {
  SchemaForm,
  type SchemaFormProps,
  type ComponentSchema,
  type PropertySchema,
  type ParameterGroup,
  type ConditionExpr,
  type LayoutHints,
  type FormatMeta,
  type ToolMeta,
  type ToolDoc,
  type ToolDocParam,
} from "@neokapi/ui-primitives";

// Filter and tool config editors (legacy aliases — prefer SchemaForm)
export { FilterConfigEditor, SchemaConfigEditor } from "./components/filter";
export type { FilterSchema, FilterParamsValue } from "./components/filter";

// Pulse public dashboard components (Bowrain AD-017)
export {
  CompletionRing,
  RisingStarBadge,
  PulseHeader,
  LanguageProgressGrid,
  ContributorBoard,
  PulseProjectCard,
  TrendAreaChart,
  TermExplorerPublic,
  PulseFilterBar,
  PulseOverview,
  PulseSettings,
  ActivityHeatmap,
} from "./components/pulse";
export type { PulseSettingsProps } from "./components/pulse";

// Typed registry of data-testid values for the bowrain UI. React components
// import names from here; Playwright specs import the same so renaming a
// testid is a single-line change. See test-ids.ts for rationale and the
// convention. Issue #425 Phase 5.
export { TEST_IDS, flattenTestIds } from "./test-ids";
