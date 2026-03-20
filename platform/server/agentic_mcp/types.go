package agentic_mcp

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
	ProjectName     string   `yaml:"name"`
	SourceLanguage  string   `yaml:"source_language"`
	TargetLanguages []string `yaml:"target_languages"`
	UpstreamRepo    string   `yaml:"upstream_repo"`
	Mode            string   `yaml:"mode"`

	ReleaseStrategy struct {
		Mode     string `yaml:"mode"`
		StartTag string `yaml:"start_tag"`
		EndTag   string `yaml:"end_tag"`
		Pace     string `yaml:"pace"`
	} `yaml:"release_strategy"`

	AgentTeam map[string]string `yaml:"agent_team"`

	ContentPaths []ContentPath `yaml:"content_paths"`
}

// ContentPath describes a localizable content path in the upstream project.
type ContentPath struct {
	Path            string `yaml:"path"`
	Format          string `yaml:"format"`
	EstimatedBlocks int    `yaml:"estimated_blocks"`
}
