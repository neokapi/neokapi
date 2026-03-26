package agenticmcp

// WorkspaceMeta holds basic metadata for a testing workspace,
// read from the fleet repo's workspaces/*/plan.yaml files.
type WorkspaceMeta struct {
	Slug               string
	Phase              string // planned | approved | active | paused | completed
	ProjectName        string
	UpstreamRepo       string
	TargetLanguages    []string
	Mode               string // accelerated | real-time | paused
	ProjectCount       int
	UntranslatedBlocks map[string]int
	Health             string // healthy | degraded | stalled
}

// WorkspacePlan is the parsed plan.yaml for a workspace.
type WorkspacePlan struct {
	// Upstream identifies the source repo and the fork used for agentic testing.
	Upstream struct {
		Repo string `yaml:"repo"` // GitHub owner/name of the upstream project (e.g. "excalidraw/excalidraw")
		Fork string `yaml:"fork"` // GitHub owner/name of the testing fork (e.g. "neokapi/agentic-excalidraw")
	} `yaml:"upstream"`

	// Project metadata. Nested under "project:" in YAML for clarity,
	// but also supports flat fields for backward compatibility.
	Project struct {
		Name            string   `yaml:"name"`
		SourceLanguage  string   `yaml:"source_language"`
		TargetLanguages []string `yaml:"target_languages"`
	} `yaml:"project"`

	// Flat fields for backward compatibility with older plan.yaml files.
	ProjectName     string   `yaml:"name"`
	SourceLanguage  string   `yaml:"source_language"`
	TargetLanguages []string `yaml:"target_languages"`
	UpstreamRepo    string   `yaml:"upstream_repo"`
	Mode            string   `yaml:"mode"`

	// Content describes how to discover localizable content in the upstream repo.
	// Uses a natural language hint for agents and a glob pattern for automated discovery.
	Content struct {
		Hint              string `yaml:"hint"`                // Natural language guidance for agents
		Format            string `yaml:"format"`              // Expected format (json, markdown, etc.)
		SourceFilePattern string `yaml:"source_file_pattern"` // Glob for source locale file (e.g. "**/locales/en.json")
	} `yaml:"content"`

	ReleaseStrategy struct {
		Mode     string   `yaml:"mode"`
		StartTag string   `yaml:"start_tag"`
		EndTag   string   `yaml:"end_tag"`
		Pace     string   `yaml:"pace"`
		Tags     []string `yaml:"tags"`      // Explicit tag sequence to walk
		SkipTags []string `yaml:"skip_tags"` // Glob patterns for tags to skip
	} `yaml:"release_strategy"`

	AgentTeam map[string]string `yaml:"agent_team"`
}

// GetProjectName returns the project name, preferring the nested Project field.
func (p *WorkspacePlan) GetProjectName() string {
	if p.Project.Name != "" {
		return p.Project.Name
	}
	return p.ProjectName
}

// GetTargetLanguages returns target languages, preferring the nested Project field.
func (p *WorkspacePlan) GetTargetLanguages() []string {
	if len(p.Project.TargetLanguages) > 0 {
		return p.Project.TargetLanguages
	}
	return p.TargetLanguages
}

// GetUpstreamRepo returns the upstream repo, preferring the nested Upstream field.
func (p *WorkspacePlan) GetUpstreamRepo() string {
	if p.Upstream.Repo != "" {
		return p.Upstream.Repo
	}
	return p.UpstreamRepo
}

// ForkRepo returns the fork GitHub owner/name (e.g. "neokapi/agentic-excalidraw").
func (p *WorkspacePlan) ForkRepo() string {
	return p.Upstream.Fork
}
