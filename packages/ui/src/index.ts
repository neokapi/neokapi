// Utilities
export { cn } from "./lib/utils";

// Glass UI (shadcn-glass-ui)
export { AnimatedBackgroundGlass } from "./components/ui/animated-background";
export { SidebarGlass, useSidebar } from "./components/ui/sidebar";
export { ModalGlass } from "./components/ui/dialog";

// UI primitives (shadcn/ui)
export { Button, type ButtonProps } from "./components/ui/button";
export { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter, GlassCard } from "./components/ui/card";
export type { GlassCardProps } from "./components/ui/card";
export { Input } from "./components/ui/input";
export { Label } from "./components/ui/label";
export { Badge, type BadgeProps } from "./components/ui/badge";
export { Separator } from "./components/ui/separator";
export { Tabs, TabsList, TabsTrigger, TabsContent, TabsGlass } from "./components/ui/tabs";
export { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "./components/ui/select";
export { Switch, type SwitchProps } from "./components/ui/switch";
export { Collapsible, CollapsibleTrigger, CollapsibleContent } from "./components/ui/collapsible";
export type { CollapsibleProps, CollapsibleTriggerProps, CollapsibleContentProps } from "./components/ui/collapsible";
export {
  Dialog, DialogTrigger, DialogContent, DialogHeader, DialogFooter,
  DialogTitle, DialogDescription, DialogClose,
} from "./components/ui/dialog";
export type { DialogProps, DialogContentProps } from "./components/ui/dialog";

// Icons (Lucide)
export * from "./components/icons";

// Components
export { WorkspaceRail } from "./components/WorkspaceRail";
export { WorkspaceIcon } from "./components/WorkspaceIcon";
export { AccountMenu } from "./components/AccountMenu";
export { TopBar } from "./components/TopBar";
export type { TopBarProps } from "./components/TopBar";
export { AppSidebar } from "./components/AppSidebar";
export type { AppSidebarProps } from "./components/AppSidebar";
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
export { TMExplorer } from "./components/tm/TMExplorer";
export { TermExplorer } from "./components/terms/TermExplorer";
export { InviteManager } from "./components/InviteManager";

// Editor components
export { SourceCellDisplay } from "./components/editor/SourceCellDisplay";
export { TargetCellEditor } from "./components/editor/TargetCellEditor";
export { TagChipNode, $createTagChipNode, $isTagChipNode } from "./components/editor/TagChipNode";
export { TagChipComponent } from "./components/editor/TagChipComponent";
export { TagPalette } from "./components/editor/TagPalette";
export { TagValidationBar } from "./components/editor/TagValidationBar";
export { InlinePreview } from "./components/editor/InlinePreview";

// Editor utilities
export { parseCodedSegments, segmentsToCodedText, spanLabel } from "./components/editor/codedText";
export type { CodedSegment } from "./components/editor/codedText";
export {
  semanticCategory, semanticLabel, semanticTooltip,
  tagColors, tagNameFromData, buildPairs, validateTags, codedTextToHtml,
} from "./components/editor/tagSemantics";
export type {
  SemanticCategory, TagColorScheme, SpanPairInfo,
  TagValidationResult, TagValidationIssue,
} from "./components/editor/tagSemantics";

// Context
export { AuthProvider, useAuth } from "./context/AuthContext";
export { WorkspaceProvider, useWorkspace } from "./context/WorkspaceContext";
export { ApiProvider, useApi } from "./context/ApiContext";
export { ThemeProvider, useTheme } from "./context/ThemeContext";
export type { Theme } from "./context/ThemeContext";
export { BreadcrumbProvider, useBreadcrumb, useSetBreadcrumb } from "./context/BreadcrumbContext";

// API
export type { ApiAdapter } from "./api/adapter";
export { RestApiAdapter } from "./api/rest-adapter";

// Collaboration
export { PresenceAvatars } from "./components/PresenceAvatars";
export type { PresenceAvatarsProps } from "./components/PresenceAvatars";
export { useCollaboration } from "./hooks/useCollaboration";
export type { CollabUser, CollabConnectionState, UseCollaborationOptions } from "./hooks/useCollaboration";

// Hooks
export { useProjectApi } from "./hooks/useProjectApi";
export { useEditorApi } from "./hooks/useEditorApi";
export { useTMApi } from "./hooks/useTMApi";
export { useTermsApi } from "./hooks/useTermsApi";
export { useProviderConfigs, useProviderApi } from "./hooks/useProviderApi";
export { useLocales } from "./hooks/useLocales";
export { useFormats } from "./hooks/useFormats";
export { useTools } from "./hooks/useTools";

// Types
export type {
  User, Workspace, Membership, ProjectInfo, ProjectItem, ConfigResponse,
  SpanInfo, BlockInfo, UpdateBlockRequest, UpdateBlockTargetCodedRequest,
  AITranslateFileRequest, TranslationStats, WordCountResult,
  ProviderConfig, ProviderConfigWithKey,
  TMEntryInfo, TMSearchResult, TMUpdateRequest, TMMatchInfo,
  TermInfo, ConceptInfo, TermSearchResult, AddConceptRequest, UpdateConceptRequest,
  BlockTermMatch, TermEnforceResult,
  LocaleInfo, FormatInfo, ToolInfo,
  FlowNodePosition, FlowNodeInfo, FlowEdgeInfo, FlowDefinitionInfo,
  Invite, AcceptInviteResponse, ClaimProjectResponse,
} from "./types/api";
export type { View, NavItem } from "./components/AppSidebar";

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
