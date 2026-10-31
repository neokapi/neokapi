package screens

// MapSummaryDTO is everything the Map renders.
type MapSummaryDTO struct {
	// ProjectKey is the project's workspace key, empty for a tab the
	// workspace does not hold.
	ProjectKey string `json:"project_key,omitempty"`
	// Files counts the project's content files: the files its collections
	// declare, as the recipe resolves them.
	Files int `json:"files"`
	// Blocks counts the blocks the block cache holds for those files.
	Blocks int `json:"blocks"`
	// Rules counts the rules in force at the project's own point: the
	// concepts in the bound terms store, the word rules and pattern rules of
	// the voice profile, and the notes a person kept.
	Rules int `json:"rules"`
	// Findings counts the findings of the last checks run over the whole
	// project in this session. Nil when none has run.
	Findings *int `json:"findings"`
	// CheckedAt is when that run finished, RFC3339; empty when none has run.
	CheckedAt string `json:"checked_at,omitempty"`
	// ReadFiles counts the files the block cache holds blocks for, and ReadAt
	// is when the store sync last found the cache current with the files on
	// disk, RFC3339. ReadAt is empty until the first sync in this session.
	ReadFiles int    `json:"read_files"`
	ReadAt    string `json:"read_at,omitempty"`
	// Suggested counts the suggestions that advise and wait for a person, and
	// Conflicts the disagreements. Awaiting is the two together: the rows
	// under Suggestions.
	Suggested int `json:"suggested"`
	Conflicts int `json:"conflicts"`
	Awaiting  int `json:"awaiting"`
	// TargetLanguages are the languages the recipe declares, canonical
	// BCP-47; empty for a source-only project.
	TargetLanguages []string `json:"target_languages"`
	// FileRows has one row per content file, the most findings first, then by
	// path.
	FileRows []MapFileDTO `json:"file_rows"`
	// Suggestions are the items awaiting a decision: the disagreements, then
	// the suggestions, in the order the digest lists them.
	Suggestions []MapSuggestionDTO `json:"suggestions"`
}

// MapFileDTO is one content file on the Map.
type MapFileDTO struct {
	// Path is absolute, for the preview; Relative is project-relative and
	// slash-separated, for the row.
	Path       string `json:"path"`
	Relative   string `json:"relative"`
	Format     string `json:"format,omitempty"`
	Collection string `json:"collection,omitempty"`
	// Blocks counts the file's blocks in the block cache.
	Blocks int `json:"blocks"`
	// Findings counts the file's findings in the last checks run over the
	// whole project, zero when none has run.
	Findings int `json:"findings"`
}

// MapSuggestionDTO is one item awaiting a decision.
type MapSuggestionDTO struct {
	// Group is "conflict" for a disagreement, whose Item is its first side,
	// or "suggested". It and the item's id make the key the Decide view
	// selects the row by.
	Group string     `json:"group"`
	Item  DigestItem `json:"item"`
	// Conflict is the disagreement, for a conflict row.
	Conflict *DigestConflict `json:"conflict,omitempty"`
	// Blocks and Files count the places the evidence names: distinct blocks
	// (a unit, or a quote where no unit is recorded) and distinct files.
	Blocks int `json:"blocks"`
	Files  int `json:"files"`
}
