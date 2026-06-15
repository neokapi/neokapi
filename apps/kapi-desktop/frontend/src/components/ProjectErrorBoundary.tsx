import { Component, type ReactNode } from "react";
import { AlertTriangle, Plug, Loader2, RefreshCw } from "lucide-react";
import { Button, Badge } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { PluginIssue } from "../types/api";
import { api } from "../hooks/useApi";

interface ProjectErrorBoundaryProps {
  children: ReactNode;
  /** Unsatisfied plugin requirements for the active project, if any. */
  pluginIssues?: PluginIssue[];
  /** Navigate elsewhere in the app (e.g. to the plugin manager). */
  onNavigate?: (view: string) => void;
}

interface ProjectErrorBoundaryState {
  error: Error | null;
  installing: string | null;
}

/**
 * Catches render errors within a project view so a single bad project (for
 * example the OkapiMart sample opened without the okapi-bridge plugin, whose
 * okf_* formats are then missing — issue #4) shows a recoverable prompt instead
 * of crashing the whole webview. When the failure lines up with missing plugins
 * it offers a one-click install; otherwise it offers a generic recovery path.
 *
 * The boundary is remounted (via a `key` derived from the plugin-resolved state)
 * once plugins install, so a successful install re-renders the real view.
 */
export class ProjectErrorBoundary extends Component<
  ProjectErrorBoundaryProps,
  ProjectErrorBoundaryState
> {
  constructor(props: ProjectErrorBoundaryProps) {
    super(props);
    this.state = { error: null, installing: null };
  }

  static getDerivedStateFromError(error: Error): Partial<ProjectErrorBoundaryState> {
    return { error };
  }

  handleInstall = (plugin: string) => {
    this.setState({ installing: plugin });
    // Fire-and-forget: the backend emits plugins-changed, which re-checks the
    // project and remounts this boundary via its key once requirements are met.
    void api.installPlugin(plugin);
  };

  render() {
    if (!this.state.error) return this.props.children;

    const issues = this.props.pluginIssues ?? [];
    const missing = issues.filter((i) => i.type === "missing");

    return (
      <div className="flex h-full items-center justify-center p-8">
        <div className="max-w-md rounded-lg border border-amber-500/30 bg-amber-500/5 p-6">
          <div className="flex items-start gap-3">
            <AlertTriangle size={18} className="mt-0.5 shrink-0 text-amber-500" />
            <div className="flex-1">
              {missing.length > 0 ? (
                <>
                  <p className="text-sm font-medium">{t("Missing plugin")}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t(
                      "This project needs a plugin that isn't installed yet to read its content formats.",
                    )}
                  </p>
                  <ul className="mt-3 space-y-2">
                    {missing.map((issue) => (
                      <li key={issue.plugin} className="flex items-center justify-between gap-2">
                        <Badge variant="outline" className="text-[10px]">
                          {issue.plugin}
                        </Badge>
                        <Button
                          size="sm"
                          onClick={() => this.handleInstall(issue.plugin)}
                          disabled={this.state.installing === issue.plugin}
                        >
                          {this.state.installing === issue.plugin ? (
                            <Loader2 size={12} className="animate-spin" />
                          ) : (
                            <Plug size={12} />
                          )}
                          {t("Install {plugin}", { plugin: issue.plugin })}
                        </Button>
                      </li>
                    ))}
                  </ul>
                </>
              ) : (
                <>
                  <p className="text-sm font-medium">{t("Couldn't open this view")}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {this.state.error.message || t("An unexpected error occurred.")}
                  </p>
                  <div className="mt-3 flex gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => this.setState({ error: null })}
                    >
                      <RefreshCw size={12} />
                      {t("Retry")}
                    </Button>
                    {this.props.onNavigate && (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => this.props.onNavigate?.("app-settings")}
                      >
                        <Plug size={12} />
                        {t("Manage plugins")}
                      </Button>
                    )}
                  </div>
                </>
              )}
            </div>
          </div>
        </div>
      </div>
    );
  }
}
